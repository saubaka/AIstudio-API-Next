package aistudio

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBrowserTransportPreservesExplicitSessionCookieOnWire(t *testing.T) {
	const cookie = "SID=fixture-sid; SAPISID=fixture-api; __Secure-3PSID=fixture-secure"
	var receivedCookie, receivedBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedCookie = r.Header.Get("Cookie")
		body, _ := io.ReadAll(r.Body)
		receivedBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	client, err := NewProxyHTTPClient("")
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()
	request, _ := http.NewRequest(http.MethodPost, server.URL, strings.NewReader(" "))
	request.Header.Set("Cookie", cookie)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if receivedCookie != cookie || receivedBody != " " {
		t.Fatal("browser transport changed the explicit Cookie header or request body")
	}
}
