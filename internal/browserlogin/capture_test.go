package browserlogin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Mag1cFall/AIStudio2API/internal/aistudio"
	"github.com/gorilla/websocket"
)

const testCookie = "SAPISID=fixture-a; __Secure-1PAPISID=fixture-b; __Secure-3PAPISID=fixture-c; SID=fixture-sid"

func TestSessionDataFormatsAndScope(t *testing.T) {
	state, err := ParseSessionData("Cookie: " + testCookie)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := aistudio.NewSigner().Sign(state); err != nil {
		t.Fatal(err)
	}
	if !state.Cookies[0].HTTPOnly || state.Cookies[0].Expires != -1 {
		t.Fatal("header import must include HttpOnly session cookies")
	}
	state.Cookies = append(state.Cookies, aistudio.StateCookie{Name: "other", Value: "private", Domain: "example.com", Path: "/", Expires: -1})
	state.Origins = []aistudio.StorageOrigin{{Origin: "https://example.com"}, {Origin: "https://aistudio.google.com"}}
	_ = state.SetAuthExtension(aistudio.AuthExtension{OAuth: &aistudio.ChromeOAuthMaterial{RefreshToken: "must-not-import"}})
	encoded, _ := json.Marshal(state)
	parsed, err := ParseSessionData(string(encoded))
	if err != nil || len(parsed.Cookies) != 4 || len(parsed.Origins) != 1 {
		t.Fatalf("scope filtering: %v", err)
	}
	if _, exists, _ := parsed.AuthExtension(); exists {
		t.Fatal("untrusted OAuth extensions must not survive import")
	}
	encoded, _ = json.Marshal(state.Cookies)
	if parsed, err := ParseSessionData(string(encoded)); err != nil || len(parsed.Cookies) != 4 {
		t.Fatal("cookie-array import failed", err)
	}
	for _, invalid := range []string{"", "Cookie: SID=x\r\nHost: evil.test", "SID=x; invalid", `{"cookies":[{"name":"SID","domain":"google.com.evil.test","value":"x","path":"/"}]}`, strings.Repeat("x", 512*1024+1)} {
		if _, err := ParseSessionData(invalid); err == nil {
			t.Fatal("accepted invalid session payload")
		}
	}
}

func TestLoginSessionLifecycle(t *testing.T) {
	manager := New(t.TempDir())
	defer manager.Close()
	ctx := context.Background()
	if _, err := manager.Start(ctx, StartInput{Mode: "open", BrowserID: "arbitrary-command"}); err == nil {
		t.Fatal("unknown browser accepted")
	}
	if _, err := manager.Start(ctx, StartInput{Mode: "unknown"}); err == nil {
		t.Fatal("unknown mode accepted")
	}
	session, err := manager.Start(ctx, StartInput{Mode: "link", AccountID: "target@example.test"})
	if err != nil || session.Opened || session.Capture != "manual" || session.URL != LoginURL {
		t.Fatal("link mode invalid", err)
	}
	if _, err := manager.Capture(ctx, session.ID, CompleteInput{}); err == nil {
		t.Fatal("missing payload accepted")
	}
	if _, err := manager.Capture(ctx, session.ID, CompleteInput{Data: "SID=only"}); err == nil {
		t.Fatal("partial auth accepted")
	}
	result, err := manager.Capture(ctx, session.ID, CompleteInput{Data: testCookie})
	if err != nil || result.Input.AccountID != "target@example.test" {
		t.Fatal("capture input was lost", err)
	}
	manager.Cancel(session.ID)
	manager.Cancel(session.ID)
	if _, err := manager.Capture(ctx, session.ID, CompleteInput{Data: testCookie}); err == nil {
		t.Fatal("cancelled session accepted")
	}
	expired, err := manager.Start(ctx, StartInput{Mode: "link"})
	if err != nil {
		t.Fatal(err)
	}
	manager.sessions[expired.ID].ExpiresAt = time.Now().Add(-time.Second)
	if _, err := manager.Capture(ctx, expired.ID, CompleteInput{Data: testCookie}); err == nil {
		t.Fatal("expired login accepted")
	}
	if _, exists := manager.sessions[expired.ID]; exists {
		t.Fatal("expired login was not cleaned up")
	}
	for range 4 {
		if _, err := manager.Start(ctx, StartInput{Mode: "link"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := manager.Start(ctx, StartInput{Mode: "link"}); err == nil {
		t.Fatal("unbounded sessions accepted")
	}
}

func TestExpiredBrowserExportCannotBecomeSessionCookie(t *testing.T) {
	for _, expiry := range []string{`"expires":0`, `"expirationDate":1`} {
		data := `[{"name":"SAPISID","value":"fixture-a","domain":".google.com","path":"/","sameSite":"no_restriction",` + expiry + `}]`
		state, err := ParseSessionData(data)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := state.CookieValue("SAPISID", "https://aistudio.google.com/", time.Now()); ok {
			t.Fatal("expired cookie became live")
		}
	}
}

func TestDedicatedBrowserCapture(t *testing.T) {
	state, _ := ParseSessionData(testCookie)
	state.Cookies = append(state.Cookies, aistudio.StateCookie{Name: "other", Value: "not-google", Domain: "other.test", Path: "/"})
	upgrader := websocket.Upgrader{}
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/json/list" {
			_ = json.NewEncoder(w).Encode([]map[string]string{{"url": "https://aistudio.google.com/prompts/new_chat", "webSocketDebuggerUrl": "ws" + strings.TrimPrefix(server.URL, "http") + "/devtools/page/fixture"}})
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		var request struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		if conn.ReadJSON(&request) != nil {
			return
		}
		var result any
		if request.Method == "Storage.getCookies" {
			result = map[string]any{"cookies": state.Cookies}
		} else {
			result = map[string]any{"result": map[string]string{"value": "fixture@example.test"}}
		}
		_ = conn.WriteJSON(map[string]any{"method": "ignored-event"})
		_ = conn.WriteJSON(map[string]any{"id": request.ID, "result": result})
	}))
	defer server.Close()
	directory := t.TempDir()
	port := strings.Split(strings.TrimPrefix(server.URL, "http://"), ":")[1]
	if err := os.WriteFile(filepath.Join(directory, "DevToolsActivePort"), []byte(port+"\n/devtools/browser/fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	got, email, err := captureChrome(ctx, directory, make(chan struct{}))
	if err != nil || email != "fixture@example.test" || len(got.Cookies) != 4 {
		t.Fatalf("capture=%s cookies=%d err=%v", email, len(got.Cookies), err)
	}
	if err := cdp(ctx, "ws://external.example/devtools/browser/x", "Storage.getCookies", nil, nil); err == nil {
		t.Fatal("non-loopback endpoint accepted")
	}
	_ = os.WriteFile(filepath.Join(directory, "DevToolsActivePort"), []byte("not-a-port\n//external.example"), 0600)
	if _, _, err := endpoint(ctx, directory, make(chan struct{})); err == nil {
		t.Fatal("unsafe endpoint file accepted")
	}
}
