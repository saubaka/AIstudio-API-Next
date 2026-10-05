package chromeauth

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestSelectedGoogleAccountIdentityAndModels(t *testing.T) {
	accounts := []googleAccountIdentity{
		{email: "first@example.test", id: "1", valid: true, verified: true},
		{email: "second@example.test", id: "2", valid: true, verified: true},
		{email: "third@example.test", id: "3", valid: true, verified: true},
	}
	for _, binary := range []bool{false, true} {
		fixture := identityFixture(binary, accounts[0], accounts[1:]...)
		if got, err := listAccountsEmailAt(fixture, 2); err != nil || got != accounts[2].email {
			t.Fatalf("selected identity: %s %v", got, err)
		}
		if _, err := listAccountsEmailAt(fixture, 3); err == nil {
			t.Fatal("missing selected account fell back to primary")
		}
	}
	state := fixtureState()
	_ = state.SetGoogleAuthUser("2")
	client := &http.Client{Transport: identityTransport(func(r *http.Request) (*http.Response, error) {
		body, contentType := "", "text/html"
		switch {
		case r.URL.Host == "aistudio.google.com":
			if r.URL.Path != "/u/2/prompts/new_chat" {
				t.Fatal("verification page selected default account")
			}
			body = "<html></html>"
		case r.URL.Host == "accounts.google.com":
			body = string(identityFixture(true, accounts[0], accounts[1:]...))
		case strings.HasSuffix(r.URL.Path, "ListModels"):
			if r.Header.Get("X-Goog-AuthUser") != "2" {
				t.Fatal("model catalog selected default account")
			}
			body = `[[["models/fixture",null,null,"Fixture Model"]]]`
			contentType = "application/json+protobuf"
		default:
			t.Fatalf("unexpected request %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}
	result, err := verifySession(context.Background(), client, &state, http.Header{"User-Agent": {"fixture"}, "X-Goog-Authuser": {"0"}})
	if err != nil || result.Email != accounts[2].email {
		t.Fatalf("verification used wrong account: %+v %v", result, err)
	}
}
