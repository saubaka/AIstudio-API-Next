package browserlogin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Mag1cFall/AIStudio2API/internal/aistudio"
	"github.com/gorilla/websocket"
)

var cookieName = regexp.MustCompile(`^[A-Za-z0-9_!#$%&'*+.^` + "`" + `|~-]{1,128}$`)

// ParseSessionData accepts a Cookie request header, a cookie array, or storage state.
// Non-Google cookies and origins are discarded; browser OAuth extensions are never imported.
func ParseSessionData(data string) (aistudio.StorageState, error) {
	data = strings.TrimSpace(data)
	selected := "0"
	if len(data) > 512*1024 {
		return aistudio.StorageState{}, invalid("会话数据过大")
	}
	if strings.HasPrefix(data, "curl ") || strings.HasPrefix(data, "curl\n") || strings.HasPrefix(data, "curl\t") {
		var err error
		data, selected, err = sessionFromCurl(data)
		if err != nil {
			return aistudio.StorageState{}, err
		}
	}
	state := aistudio.StorageState{Origins: []aistudio.StorageOrigin{}}
	if strings.HasPrefix(data, "{") {
		if err := json.Unmarshal([]byte(data), &state); err != nil {
			return state, invalid("会话 JSON 格式无效")
		}
	} else if strings.HasPrefix(data, "[") {
		if err := json.Unmarshal([]byte(data), &state.Cookies); err != nil {
			return state, invalid("Cookie JSON 格式无效")
		}
	} else {
		if strings.HasPrefix(strings.ToLower(data), "cookie:") {
			data = strings.TrimSpace(data[7:])
		}
		if strings.ContainsAny(data, "\r\n") {
			return state, invalid("请粘贴 Cookie 请求头的值，或浏览器“复制为 cURL”的完整内容，不要粘贴全部标头")
		}
		for _, part := range strings.Split(data, ";") {
			if strings.TrimSpace(part) == "" {
				continue
			}
			name, value, ok := strings.Cut(strings.TrimSpace(part), "=")
			if !ok || !cookieName.MatchString(name) {
				return state, invalid("Cookie 格式无效，请复制完整 Cookie 请求头")
			}
			state.Cookies = append(state.Cookies, aistudio.StateCookie{Name: name, Value: value, Domain: ".google.com", Path: "/", Expires: -1, HTTPOnly: true, Secure: true, SameSite: "None"})
		}
	}
	// Common browser exports use expirationDate and lowercase SameSite names.
	if strings.HasPrefix(data, "{") || strings.HasPrefix(data, "[") {
		// Only retain the account index from our own session export. Identity
		// and OAuth material are still stripped and reverified with Google.
		selected = state.GoogleAuthUser()
		var raw []json.RawMessage
		if strings.HasPrefix(data, "{") {
			var object struct {
				Cookies []json.RawMessage `json:"cookies"`
			}
			_ = json.Unmarshal([]byte(data), &object)
			raw = object.Cookies
		} else {
			_ = json.Unmarshal([]byte(data), &raw)
		}
		for index, value := range raw {
			if index >= len(state.Cookies) {
				break
			}
			var fields struct {
				ExpirationDate *float64 `json:"expirationDate"`
				Expires        *float64 `json:"expires"`
			}
			_ = json.Unmarshal(value, &fields)
			if fields.Expires == nil {
				if fields.ExpirationDate != nil {
					state.Cookies[index].Expires = *fields.ExpirationDate
				} else {
					state.Cookies[index].Expires = -1
				}
			}
			switch strings.ToLower(state.Cookies[index].SameSite) {
			case "none", "no_restriction":
				state.Cookies[index].SameSite = "None"
			case "lax":
				state.Cookies[index].SameSite = "Lax"
			case "strict":
				state.Cookies[index].SameSite = "Strict"
			case "unspecified":
				state.Cookies[index].SameSite = ""
			}
		}
	}
	filtered, err := filterState(state)
	if err != nil {
		return filtered, err
	}
	if selected != "0" {
		if err := filtered.SetGoogleAuthUser(selected); err != nil {
			return filtered, invalid(err.Error())
		}
	}
	return filtered, nil
}

func googleDomain(domain string) bool {
	domain = strings.ToLower(strings.TrimPrefix(domain, "."))
	return domain == "google.com" || strings.HasSuffix(domain, ".google.com")
}

func filterState(source aistudio.StorageState) (aistudio.StorageState, error) {
	state := aistudio.StorageState{Cookies: []aistudio.StateCookie{}, Origins: []aistudio.StorageOrigin{}}
	for _, cookie := range source.Cookies {
		if !googleDomain(cookie.Domain) {
			continue
		}
		if !cookieName.MatchString(cookie.Name) || strings.ContainsAny(cookie.Value, "\r\n\x00;") {
			return state, invalid("Cookie 字段无效")
		}
		if cookie.Path == "" {
			cookie.Path = "/"
		}
		state.Cookies = append(state.Cookies, cookie)
	}
	for _, origin := range source.Origins {
		parsed, err := url.Parse(origin.Origin)
		if err == nil && parsed.Scheme == "https" && parsed.User == nil && googleDomain(parsed.Hostname()) {
			state.Origins = append(state.Origins, origin)
		}
	}
	if err := state.Validate(); err != nil {
		return state, invalid("会话字段无效，请重新导出 Cookie JSON")
	}
	if len(state.Cookies) == 0 {
		return state, invalid("没有可用于 Google 登录的 Cookie")
	}
	return state, nil
}

func endpoint(ctx context.Context, directory string, done <-chan struct{}) (string, string, error) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		data, err := os.ReadFile(filepath.Join(directory, "DevToolsActivePort"))
		if err == nil {
			lines := strings.Split(strings.TrimSpace(string(data)), "\n")
			if len(lines) == 2 {
				port, err := strconv.Atoi(lines[0])
				if err == nil && port > 0 && port <= 65535 && strings.HasPrefix(lines[1], "/devtools/browser/") && !strings.ContainsAny(lines[1], "?#\r\n") {
					base := fmt.Sprintf("127.0.0.1:%d", port)
					return "http://" + base, "ws://" + base + lines[1], nil
				}
			}
			return "", "", invalid("本机浏览器登录窗口的连接信息无效")
		}
		select {
		case <-ctx.Done():
			return "", "", invalid("等待本机浏览器超时，请重试或使用登录链接")
		case <-done:
			return "", "", invalid("登录窗口已关闭，请重新打开登录")
		case <-ticker.C:
		}
	}
}

func cdp(ctx context.Context, endpoint, method string, params any, result any) error {
	if params == nil {
		params = map[string]any{}
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme != "ws" || parsed.Hostname() != "127.0.0.1" {
		return invalid("登录窗口连接地址无效")
	}
	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	conn, _, err := dialer.DialContext(ctx, endpoint, nil)
	if err != nil {
		return invalid("无法连接登录窗口，请确认窗口仍打开")
	}
	defer conn.Close()
	deadline := time.Now().Add(5 * time.Second)
	if end, ok := ctx.Deadline(); ok && end.Before(deadline) {
		deadline = end
	}
	_ = conn.SetReadDeadline(deadline)
	_ = conn.SetWriteDeadline(deadline)
	conn.SetReadLimit(4 * 1024 * 1024)
	if err := conn.WriteJSON(map[string]any{"id": 1, "method": method, "params": params}); err != nil {
		return invalid("登录窗口连接已中断")
	}
	for {
		var message struct {
			ID     int             `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  json.RawMessage `json:"error"`
		}
		if err := conn.ReadJSON(&message); err != nil {
			return invalid("读取登录窗口失败，请确认已在 AI Studio 完成登录")
		}
		if message.ID != 1 {
			continue
		}
		if len(message.Error) > 0 {
			return invalid("浏览器未能导出会话，请使用手动导入")
		}
		if result != nil && json.Unmarshal(message.Result, result) != nil {
			return invalid("浏览器返回无效会话")
		}
		return nil
	}
}

func captureChrome(ctx context.Context, directory string, done <-chan struct{}) (aistudio.StorageState, string, error) {
	base, ws, err := endpoint(ctx, directory, done)
	if err != nil {
		return aistudio.StorageState{}, "", err
	}
	var result struct {
		Cookies []aistudio.StateCookie `json:"cookies"`
	}
	if err := cdp(ctx, ws, "Storage.getCookies", map[string]any{}, &result); err != nil {
		return aistudio.StorageState{}, "", err
	}
	state, err := filterState(aistudio.StorageState{Cookies: result.Cookies})
	if err != nil {
		return state, "", invalid("请先在登录窗口完成 Google 登录并进入 AI Studio，再点击完成登录")
	}
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, base+"/json/list", nil)
	client := &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: 5 * time.Second}
	defer client.CloseIdleConnections()
	response, err := client.Do(request)
	if err != nil {
		return state, "", invalid("无法读取登录窗口")
	}
	defer response.Body.Close()
	var pages []struct {
		URL       string `json:"url"`
		WebSocket string `json:"webSocketDebuggerUrl"`
	}
	if response.StatusCode != 200 || json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&pages) != nil {
		return state, "", invalid("无法读取登录页面")
	}
	for _, page := range pages {
		parsed, err := url.Parse(page.URL)
		if err != nil || parsed.Scheme != "https" || parsed.Hostname() != "aistudio.google.com" {
			continue
		}
		var evaluated struct {
			Result struct {
				Value string `json:"value"`
			} `json:"result"`
		}
		expression := `(() => { const metadata = window.WIZ_global_data?.oPEP7c; if (typeof metadata === 'string' && metadata.includes('@')) return metadata; return [...document.querySelectorAll('[aria-label]')].map(e=>e.getAttribute('aria-label')).join('\n').match(/[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}/i)?.[0] || ''; })()`
		if err := cdp(ctx, page.WebSocket, "Runtime.evaluate", map[string]any{"expression": expression, "returnByValue": true}, &evaluated); err != nil {
			return state, "", err
		}
		return state, strings.ToLower(strings.TrimSpace(evaluated.Result.Value)), nil
	}
	return state, "", invalid("请在登录窗口进入 AI Studio 页面后再完成登录")
}
