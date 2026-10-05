package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"runtime"
	"strings"
	"time"

	"github.com/Mag1cFall/AIStudio2API/internal/aistudio"
	"github.com/Mag1cFall/AIStudio2API/internal/api"
	"github.com/Mag1cFall/AIStudio2API/internal/browserlogin"
	"github.com/Mag1cFall/AIStudio2API/internal/chromeauth"
	"github.com/Mag1cFall/AIStudio2API/internal/config"
)

var _ api.GoogleLoginService = (*runtimeManager)(nil)

func (manager *runtimeManager) GoogleLoginOptions(context.Context) (api.GoogleLoginOptions, error) {
	return api.GoogleLoginOptions{Browsers: browserlogin.Browsers(), URL: browserlogin.LoginURL, ChromeImportSupported: runtime.GOOS == "windows"}, nil
}

func (manager *runtimeManager) StartGoogleLogin(ctx context.Context, input browserlogin.StartInput) (browserlogin.Session, error) {
	if manager.googleLogins == nil {
		return browserlogin.Session{}, fmt.Errorf("本机登录服务未初始化")
	}
	if err := config.ValidateProxy(strings.TrimSpace(input.Proxy)); err != nil {
		return browserlogin.Session{}, invalidAccount(err)
	}
	manager.mu.RLock()
	cfg := manager.current.config
	if input.AccountID != "" {
		accounts, err := manager.current.admin.Accounts(ctx)
		if err != nil {
			manager.mu.RUnlock()
			return browserlogin.Session{}, err
		}
		found := false
		for _, account := range accounts {
			if account.ID == input.AccountID {
				found = true
				break
			}
		}
		if !found {
			manager.mu.RUnlock()
			return browserlogin.Session{}, &adminOperationError{http.StatusNotFound, "account_not_found", "账户不存在"}
		}
	}
	manager.mu.RUnlock()
	input.Proxy = strings.TrimSpace(input.Proxy)
	input.BrowserProxy = input.Proxy
	if input.BrowserProxy == "" {
		input.BrowserProxy = cfg.Proxy
	}
	return manager.googleLogins.Start(ctx, input)
}

func (manager *runtimeManager) CompleteGoogleLogin(ctx context.Context, id string, input browserlogin.CompleteInput) (api.AdminAccount, error) {
	manager.googleMu.Lock()
	defer manager.googleMu.Unlock()
	if manager.googleLogins == nil {
		return api.AdminAccount{}, fmt.Errorf("本机登录服务未初始化")
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	result, err := manager.googleLogins.Capture(ctx, id, input)
	if err != nil {
		return api.AdminAccount{}, err
	}
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	importer, ok := manager.current.admin.(interface {
		ImportGoogleSession(context.Context, browserlogin.Result) (api.AdminAccount, error)
	})
	if !ok {
		return api.AdminAccount{}, fmt.Errorf("当前运行时不支持导入 Google 会话")
	}
	account, err := importer.ImportGoogleSession(ctx, result)
	if err != nil {
		return api.AdminAccount{}, err
	}
	manager.googleLogins.Cancel(id)
	return account, nil
}

func (manager *runtimeManager) CancelGoogleLogin(_ context.Context, id string) error {
	manager.googleMu.Lock()
	defer manager.googleMu.Unlock()
	if manager.googleLogins != nil {
		manager.googleLogins.Cancel(id)
	}
	return nil
}

// ImportGoogleSession verifies Google's live model endpoint before persisting credentials.
func (admin *runtimeAdmin) ImportGoogleSession(ctx context.Context, result browserlogin.Result) (api.AdminAccount, error) {
	verification, err := admin.verifySession(ctx, &result.State, admin.effectiveProxy(result.Input.Proxy))
	if err != nil {
		return api.AdminAccount{}, invalidAccount(fmt.Errorf("会话未通过 AI Studio 验证，请确认已登录并可访问模型：%w", err))
	}
	email, err := googleSessionEmail(result, verification)
	if err != nil {
		return api.AdminAccount{}, err
	}
	if err := result.State.SetAuthExtension(aistudio.AuthExtension{Source: aistudio.AuthSource{Browser: "local_browser", Email: email, AuthUser: result.State.GoogleAuthUser()}}); err != nil {
		return api.AdminAccount{}, err
	}
	if result.Input.AccountID != "" {
		return admin.updateGoogleSession(ctx, email, result.State)
	}
	// Re-importing an already verified identity refreshes its credentials using
	// the account lease. Keep its settings, resources and directory intact.
	for _, existing := range admin.pool.Status() {
		if existing.ID == email {
			return admin.updateGoogleSession(ctx, email, result.State)
		}
	}
	accountConfig := aistudio.DefaultAccountConfig(email)
	accountConfig.Proxy = strings.TrimSpace(result.Input.Proxy)
	if result.Input.Locale != "" {
		accountConfig.Locale = result.Input.Locale
	}
	if result.Input.Timezone != "" {
		accountConfig.Timezone = result.Input.Timezone
	}
	if err := accountConfig.Validate(); err != nil {
		return api.AdminAccount{}, invalidAccount(err)
	}
	account, err := admin.addAccount(ctx, accountConfig, result.State, "")
	if err != nil {
		return api.AdminAccount{}, err
	}
	admin.requests.log("auth", "INFO", "本机浏览器账户已接入 | 账户="+email)
	return account, nil
}

func googleSessionEmail(result browserlogin.Result, verification chromeauth.Verification) (string, error) {
	email := strings.ToLower(strings.TrimSpace(verification.Email))
	captured := strings.ToLower(strings.TrimSpace(result.Email))
	if email != "" && captured != "" && email != captured {
		return "", invalidAccount(fmt.Errorf("浏览器显示的账户与导入会话不一致，请只保留目标 Google 账户后重试"))
	}
	if email == "" {
		email = captured
	}
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email {
		return "", invalidAccount(fmt.Errorf("无法识别会话所属 Google 邮箱，请刷新已登录的 AI Studio 页面，重新复制该页面请求的 cURL 后接入"))
	}
	if result.Input.AccountID != "" && strings.ToLower(result.Input.AccountID) != email {
		return "", invalidAccount(fmt.Errorf("登录邮箱与要更新的账户不一致"))
	}
	return email, nil
}

func (admin *runtimeAdmin) updateGoogleSession(ctx context.Context, id string, state aistudio.StorageState) (api.AdminAccount, error) {
	lease, err := admin.pool.AcquireAccount(ctx, id)
	if err != nil {
		return api.AdminAccount{}, accountOperationError(err)
	}
	account := lease.Account()
	if err := admin.workers.Reset(id); err != nil {
		return api.AdminAccount{}, errors.Join(err, lease.Release())
	}
	if err := lease.SaveStorageState(state); err != nil {
		return api.AdminAccount{}, errors.Join(err, lease.Release())
	}
	if err := admin.service.changeModels(func() error {
		return errors.Join(admin.pool.MarkReady(id), admin.pool.ResetModelAccess(id), admin.pool.SetCatalog(id, account.BenefitTier, nil))
	}); err != nil {
		return api.AdminAccount{}, errors.Join(err, lease.Release())
	}
	if err := lease.Release(); err != nil {
		return api.AdminAccount{}, err
	}
	if admin.service.State() == "RUNNING" {
		admin.syncAccountModelCatalog(ctx, account)
	}
	admin.requests.log("auth", "INFO", "本机浏览器会话已更新 | 账户="+id)
	return admin.account(id)
}
