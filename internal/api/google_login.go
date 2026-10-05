package api

import (
	"context"
	"net/http"

	"github.com/Mag1cFall/AIStudio2API/internal/browserlogin"
)

type GoogleLoginOptions struct {
	Browsers              []browserlogin.Browser `json:"browsers"`
	URL                   string                 `json:"url"`
	ChromeImportSupported bool                   `json:"chrome_import_supported"`
}

// This only parses supplied text; it never starts a browser or verifies upstream.
func (s *server) handleInspectGooglePaste(w http.ResponseWriter, r *http.Request) {
	var input browserlogin.CompleteInput
	if err := decodeJSON(r, &input); err != nil {
		writeAdminError(w, http.StatusBadRequest, "invalid_request", "粘贴内容格式无效")
		return
	}
	check, err := browserlogin.InspectPaste(input.Data)
	if err != nil {
		writeAdminUpstreamError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, check)
}

// Kept separate so existing AdminService implementations remain compatible.
type GoogleLoginService interface {
	GoogleLoginOptions(context.Context) (GoogleLoginOptions, error)
	StartGoogleLogin(context.Context, browserlogin.StartInput) (browserlogin.Session, error)
	CompleteGoogleLogin(context.Context, string, browserlogin.CompleteInput) (AdminAccount, error)
	CancelGoogleLogin(context.Context, string) error
}

func (s *server) googleLogin(w http.ResponseWriter) GoogleLoginService {
	service, ok := s.config.Admin.(GoogleLoginService)
	if !ok {
		writeAdminError(w, http.StatusNotImplemented, "unsupported", "当前运行时不支持本机浏览器登录")
		return nil
	}
	return service
}

func (s *server) handleGoogleLoginOptions(w http.ResponseWriter, r *http.Request) {
	service := s.googleLogin(w)
	if service == nil {
		return
	}
	options, err := service.GoogleLoginOptions(r.Context())
	if err != nil {
		writeAdminUpstreamError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, options)
}

func (s *server) handleStartGoogleLogin(w http.ResponseWriter, r *http.Request) {
	service := s.googleLogin(w)
	if service == nil {
		return
	}
	var input browserlogin.StartInput
	if err := decodeJSON(r, &input); err != nil {
		writeAdminError(w, http.StatusBadRequest, "invalid_request", "登录参数格式无效")
		return
	}
	session, err := service.StartGoogleLogin(r.Context(), input)
	if err != nil {
		writeAdminUpstreamError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, session)
}

func (s *server) handleCompleteGoogleLogin(w http.ResponseWriter, r *http.Request) {
	service := s.googleLogin(w)
	if service == nil {
		return
	}
	var input browserlogin.CompleteInput
	if err := decodeJSON(r, &input); err != nil {
		writeAdminError(w, http.StatusBadRequest, "invalid_request", "会话数据格式无效")
		return
	}
	account, err := service.CompleteGoogleLogin(r.Context(), r.PathValue("session"), input)
	if err != nil {
		writeAdminUpstreamError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]AdminAccount{"account": account})
}

func (s *server) handleCancelGoogleLogin(w http.ResponseWriter, r *http.Request) {
	service := s.googleLogin(w)
	if service == nil {
		return
	}
	if err := service.CancelGoogleLogin(r.Context(), r.PathValue("session")); err != nil {
		writeAdminUpstreamError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
