package chromeauth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"github.com/Mag1cFall/AIStudio2API/internal/aistudio"
	"golang.org/x/net/html"
)

const aiStudioChatURL = "https://aistudio.google.com/prompts/new_chat"

// Verification 保存 AI Studio 登录页与模型目录验收结果
type Verification struct {
	ModelCount int
	Models     []aistudio.Model
	Email      string
}

// Verify 验证账号可访问 WAA 页面并读取实时模型目录
func Verify(ctx context.Context, state *aistudio.StorageState, proxy string) (Verification, error) {
	if state == nil {
		return Verification{}, fmt.Errorf("storage state 为空")
	}
	if _, err := aistudio.NewSigner().Sign(*state); err != nil {
		return Verification{}, err
	}
	client, err := aistudio.NewProxyHTTPClient(proxy)
	if err != nil {
		return Verification{}, err
	}
	defer client.CloseIdleConnections()
	headers, err := aistudio.DiscoverPublicHeaders(ctx, client)
	if err != nil {
		return Verification{}, err
	}
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return verifySession(ctx, client, state, headers)
}

func verifySession(ctx context.Context, client *http.Client, state *aistudio.StorageState, headers http.Header) (Verification, error) {
	// A page response can rotate cookies even when account metadata is absent.
	// Resolve the imported session's identity before relying on those updates.
	identityState := *state
	identityState.Cookies = append([]aistudio.StateCookie(nil), state.Cookies...)
	var email string
	if err := verifyChatPage(ctx, client, state, headers.Get("User-Agent"), &email); err != nil {
		return Verification{}, err
	}
	if email == "" {
		var err error
		email, err = verifyGoogleIdentity(ctx, client, &identityState, headers.Get("User-Agent"))
		if err != nil {
			return Verification{}, err
		}
		state.Cookies = identityState.Cookies
	}
	models, err := verifyModels(ctx, client, state, headers)
	if err != nil {
		return Verification{}, err
	}
	return Verification{ModelCount: len(models), Models: models, Email: email}, nil
}

func verifyChatPage(ctx context.Context, client *http.Client, state *aistudio.StorageState, userAgent string, email *string) error {
	target := aiStudioChatURL
	if selected := state.GoogleAuthUser(); selected != "0" {
		target = "https://aistudio.google.com/u/" + selected + "/prompts/new_chat"
	}
	cookie, err := state.CookieHeader(target, time.Now())
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	request.Header.Set("Cookie", cookie)
	request.Header.Set("User-Agent", userAgent)
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("访问 AI Studio 登录页: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("AI Studio 登录页返回 HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if err != nil {
		return fmt.Errorf("读取 AI Studio 登录页失败")
	}
	*email = verifiedPageEmail(string(body))
	return mergeResponseCookies(state, response, target)
}

var pageMetadataPattern = regexp.MustCompile(`^\s*(?:window\.)?WIZ_global_data\s*=\s*`)

// verifiedPageEmail reads account metadata, never arbitrary prompt or page text.
func verifiedPageEmail(body string) string {
	tokens := html.NewTokenizer(strings.NewReader(body))
	inScript := false
	jsonMetadata := false
	for {
		switch tokens.Next() {
		case html.ErrorToken:
			return ""
		case html.StartTagToken:
			token := tokens.Token()
			inScript = token.Data == "script"
			jsonMetadata = false
			if inScript {
				for _, attr := range token.Attr {
					if attr.Key == "type" && strings.EqualFold(strings.TrimSpace(attr.Val), "application/json") {
						jsonMetadata = true
					}
				}
			}
		case html.EndTagToken:
			inScript = false
		case html.TextToken:
			if !inScript {
				continue
			}
			data := tokens.Text()
			if !jsonMetadata {
				prefix := pageMetadataPattern.FindIndex(data)
				if prefix == nil {
					continue
				}
				data = data[prefix[1]:]
			}
			var metadata struct {
				Email     string `json:"oPEP7c"`
				App       string `json:"qwAQke"`
				APIKey    string `json:"WIu0Nc"`
				SessionID string `json:"FdrFJe"`
			}
			if json.NewDecoder(bytes.NewReader(data)).Decode(&metadata) == nil {
				// New AI Studio releases put bootstrap metadata in a JSON script
				// with a random ID, rather than assigning WIZ_global_data. Require
				// its application and protocol fields, not arbitrary JSON email.
				if jsonMetadata && (!strings.HasPrefix(metadata.App, "MakerSuite") || metadata.APIKey == "" || metadata.SessionID == "") {
					continue
				}
				// oPEP7c is the signed-in email. FdrFJe is a numeric session ID.
				return normalizedEmail(metadata.Email)
			}
		}
	}
}

func normalizedEmail(value string) string {
	email := strings.ToLower(value)
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email {
		return ""
	}
	return email
}

func verifyModels(ctx context.Context, client *http.Client, state *aistudio.StorageState, headers http.Header) ([]aistudio.Model, error) {
	url := aistudio.MakerSuiteRPCBase + "ListModels"
	cookie, err := state.CookieHeader(url, time.Now())
	if err != nil {
		return nil, err
	}
	authorization, err := aistudio.NewSigner().Authorization(*state)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader([]byte("[]")))
	if err != nil {
		return nil, err
	}
	request.Header = headers.Clone()
	request.Header.Set("X-Goog-AuthUser", state.GoogleAuthUser())
	request.Header.Set("Accept", "*/*")
	request.Header.Set("Authorization", authorization)
	request.Header.Set("Content-Type", aistudio.JSONProtobufContentType)
	request.Header.Set("Cookie", cookie)
	request.Header.Set("Origin", "https://aistudio.google.com")
	request.Header.Set("Referer", "https://aistudio.google.com/")
	request.Header.Set("Sec-Fetch-Dest", "empty")
	request.Header.Set("Sec-Fetch-Mode", "cors")
	request.Header.Set("Sec-Fetch-Site", "same-site")
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("读取 AI Studio 模型目录: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, response.Body)
		return nil, fmt.Errorf("AI Studio ListModels 返回 HTTP %d", response.StatusCode)
	}
	if !strings.HasPrefix(strings.ToLower(response.Header.Get("Content-Type")), aistudio.JSONProtobufContentType) {
		return nil, fmt.Errorf("AI Studio ListModels 返回未识别的 Content-Type %q", response.Header.Get("Content-Type"))
	}
	models, err := aistudio.ParseModels(response.Body)
	if err != nil {
		return nil, err
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("AI Studio ListModels 返回空目录")
	}
	if err := mergeResponseCookies(state, response, url); err != nil {
		return nil, err
	}
	return models, nil
}

func mergeResponseCookies(state *aistudio.StorageState, response *http.Response, sourceURL string) error {
	setCookies := response.Header.Values("Set-Cookie")
	if len(setCookies) == 0 {
		return nil
	}
	return state.MergeSetCookieHeaders(setCookies, sourceURL, time.Now())
}
