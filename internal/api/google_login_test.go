package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Mag1cFall/AIStudio2API/internal/browserlogin"
)

type loginAdminFixture struct {
	AdminService
	starts        int
	cancellations int
}

func (f *loginAdminFixture) GoogleLoginOptions(context.Context) (GoogleLoginOptions, error) {
	return GoogleLoginOptions{URL: browserlogin.LoginURL}, nil
}
func (f *loginAdminFixture) StartGoogleLogin(_ context.Context, input browserlogin.StartInput) (browserlogin.Session, error) {
	f.starts++
	return browserlogin.Session{ID: "fixture", URL: browserlogin.LoginURL, Capture: "manual"}, nil
}
func (f *loginAdminFixture) CompleteGoogleLogin(_ context.Context, id string, input browserlogin.CompleteInput) (AdminAccount, error) {
	return AdminAccount{ID: "fixture@example.test"}, nil
}
func (f *loginAdminFixture) CancelGoogleLogin(context.Context, string) error {
	f.cancellations++
	return nil
}

func TestGoogleLoginRoutesStayBehindControlPlane(t *testing.T) {
	fixture := &loginAdminFixture{}
	s := &server{config: Config{Admin: fixture}}
	mux := http.NewServeMux()
	s.registerAdmin(mux)
	auth := newAdminAuth(Config{AdminAuthEnabled: true, AdminUsername: "operator", AdminPassword: "fixture-password"})
	handler := auth.handler(mux)
	if got := authExchange(handler, "POST", "/api/auth/google/start", `{"mode":"link"}`); got.Code != 401 || fixture.starts != 0 {
		t.Fatal("Google login must require console authentication")
	}
	if got := authExchange(handler, "POST", "/api/auth/google/inspect", `{"data":"SID=fixture-secret"}`); got.Code != 401 {
		t.Fatal("inspection must require console authentication")
	}
	login := authExchange(handler, "POST", "/api/auth/login", `{"username":"operator","password":"fixture-password"}`)
	cookie := login.Result().Cookies()[0]
	r := httptest.NewRequest("POST", "http://localhost/api/auth/google/start", strings.NewReader(`{"mode":"link"}`))
	r.RemoteAddr = "127.0.0.1:5000"
	r.Header.Set("Origin", "https://external.example")
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 403 || fixture.starts != 0 {
		t.Fatal("cross-origin login launch accepted")
	}
	r = httptest.NewRequest("POST", "http://localhost/api/auth/google/inspect", strings.NewReader(`{"data":"SID=fixture-secret"}`))
	r.RemoteAddr = "127.0.0.1:5000"
	r.Header.Set("Origin", "https://external.example")
	r.AddCookie(cookie)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("cross-origin inspection accepted")
	}
	if got := authExchange(handler, "POST", "/api/auth/google/inspect", `{"data":"SID=fixture-secret"}`, cookie); got.Code != 200 || strings.Contains(got.Body.String(), "fixture-secret") || got.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("inspection failed or revealed cookies")
	}
	if got := authExchange(handler, "GET", "/api/auth/google/options", "", cookie); got.Code != 200 || got.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("options failed")
	}
	if got := authExchange(handler, "POST", "/api/auth/google/start", `{"mode":"link"}`, cookie); got.Code != 201 || fixture.starts != 1 {
		t.Fatal("launch failed")
	}
	if got := authExchange(handler, "POST", "/api/auth/google/fixture/complete", `{"data":"fixture-secret-cookie"}`, cookie); got.Code != 200 || strings.Contains(got.Body.String(), "fixture-secret-cookie") {
		t.Fatal("completion leaked session data")
	}
	if got := authExchange(handler, "DELETE", "/api/auth/google/fixture", "", cookie); got.Code != 204 || fixture.cancellations != 1 {
		t.Fatal("cancel failed")
	}
	for _, path := range []string{"/api/accounts", "/api/accounts/fixture@example.test/login"} {
		if got := authExchange(handler, "POST", path, `{}`, cookie); got.Code != 410 {
			t.Fatal("legacy Camoufox login route must be disabled")
		}
	}
}
