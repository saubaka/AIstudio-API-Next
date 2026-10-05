package chromeauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Mag1cFall/AIStudio2API/internal/aistudio"
	"google.golang.org/protobuf/encoding/protowire"
)

func identityFixture(binary bool, primary googleAccountIdentity, others ...googleAccountIdentity) []byte {
	accounts := append([]googleAccountIdentity{primary}, others...)
	if binary {
		var result []byte
		for _, account := range accounts {
			var record []byte
			for _, field := range []struct {
				number protowire.Number
				value  string
			}{{3, account.email}, {10, account.id}} {
				record = protowire.AppendTag(record, field.number, protowire.BytesType)
				record = protowire.AppendString(record, field.value)
			}
			for _, field := range []struct {
				number protowire.Number
				value  bool
			}{{9, account.valid}, {14, account.signedOut}, {15, account.verified}} {
				record = protowire.AppendTag(record, field.number, protowire.VarintType)
				var value uint64
				if field.value {
					value = 1
				}
				record = protowire.AppendVarint(record, value)
			}
			result = protowire.AppendTag(result, 1, protowire.BytesType)
			result = protowire.AppendBytes(result, record)
		}
		return []byte(base64.StdEncoding.EncodeToString(result))
	}
	var rows [][]any
	for _, account := range accounts {
		row := make([]any, 16)
		row[3], row[10] = account.email, account.id
		for _, field := range []struct {
			index int
			value bool
		}{{9, account.valid}, {14, account.signedOut}, {15, account.verified}} {
			value := 0
			if field.value {
				value = 1
			}
			row[field.index] = value
		}
		rows = append(rows, row)
	}
	data, _ := json.Marshal([]any{"gaia.l.a", rows})
	return data
}

func TestListAccountsPrimaryIdentity(t *testing.T) {
	primary := googleAccountIdentity{email: "Primary@Example.Test", id: "fixture-1", valid: true, verified: true}
	other := googleAccountIdentity{email: "other@example.test", id: "fixture-2", valid: true, verified: true}
	for _, binary := range []bool{false, true} {
		if email, err := listAccountsEmail(identityFixture(binary, primary, other)); err != nil || email != "primary@example.test" {
			t.Fatalf("binary=%v email=%q err=%v", binary, email, err)
		}
		for _, mutate := range []func(*googleAccountIdentity){
			func(a *googleAccountIdentity) { a.valid = false },
			func(a *googleAccountIdentity) { a.signedOut = true },
			func(a *googleAccountIdentity) { a.verified = false },
			func(a *googleAccountIdentity) { a.email = "" },
			func(a *googleAccountIdentity) { a.id = "" },
			func(a *googleAccountIdentity) { a.email = "Name <primary@example.test>" },
		} {
			invalid := primary
			mutate(&invalid)
			if email, err := listAccountsEmail(identityFixture(binary, invalid, other)); err == nil || email != "" {
				t.Fatalf("invalid primary selected another account: binary=%v", binary)
			}
		}
	}
	for _, data := range []string{"", "unrelated@example.test", "<html>primary@example.test</html>", "[]", `["gaia.l.a",[]]`, "Cg==", "////"} {
		if _, err := listAccountsEmail([]byte(data)); err == nil {
			t.Fatal("malformed account response accepted")
		}
	}
	data := identityFixture(false, primary)
	if email, err := listAccountsEmail(append([]byte(")]}'\n"), data...)); err != nil || email != "primary@example.test" {
		t.Fatal("XSSI response rejected")
	}
	quoted, _ := json.Marshal(string(identityFixture(true, primary)))
	if _, err := listAccountsEmail(quoted); err != nil {
		t.Fatal("quoted binary response rejected")
	}
}

type identityTransport func(*http.Request) (*http.Response, error)

func (transport identityTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return transport(r)
}

func fixtureState() aistudio.StorageState {
	state := aistudio.StorageState{}
	for _, name := range []string{"SAPISID", "__Secure-1PAPISID", "__Secure-3PAPISID", "SID"} {
		state.Cookies = append(state.Cookies, aistudio.StateCookie{Name: name, Value: "fixture-only", Domain: ".google.com", Path: "/", Expires: -1, Secure: true})
	}
	return state
}

// Covers both Google bootstrap formats and the missing-metadata fallback.
func TestVerifySessionPageMetadataFormats(t *testing.T) {
	for _, metadataPage := range []string{
		"",
		`<script>window.WIZ_global_data={"FdrFJe":"123456789","oPEP7c":"Primary\u0040Example.Test"}</script>`,
		`<script id="random-bootstrap-id" type="application/json">{"qwAQke":"MakerSuiteHttp","WIu0Nc":"public-fixture-key","FdrFJe":"123456789","oPEP7c":"Primary\u0040Example.Test"}</script>`,
	} {
		hasMetadata := metadataPage != ""
		identityRequests := 0
		client := &http.Client{Transport: identityTransport(func(r *http.Request) (*http.Response, error) {
			body, contentType := "", "text/html"
			responseHeaders := make(http.Header)
			switch r.URL.String() {
			case aiStudioChatURL:
				body = `<html>AI Studio prompt contains unrelated@example.test</html>`
				if hasMetadata {
					body += metadataPage
				} else {
					responseHeaders.Set("Set-Cookie", "SID=; Domain=.google.com; Path=/; Max-Age=0")
				}
			case aistudio.MakerSuiteRPCBase + "ListModels":
				if r.Header.Get("Authorization") == "" || r.Header.Get("X-Goog-Authuser") != "0" {
					t.Fatal("model request did not verify default account")
				}
				body, contentType = `[[["models/fixture",null,null,"Fixture Model"]]]`, aistudio.JSONProtobufContentType
			case googleAccountsURL:
				identityRequests++
				if r.Method != "POST" || !strings.Contains(r.Header.Get("Cookie"), "SID=fixture-only") || r.Header.Get("Origin") != "https://www.google.com" || r.Header.Get("Authorization") != "" {
					t.Fatal("incorrect identity request")
				}
				body = string(identityFixture(true, googleAccountIdentity{email: "primary@example.test", id: "fixture-1", valid: true, verified: true}))
			default:
				t.Fatalf("unexpected request: %s", r.URL)
			}
			responseHeaders.Set("Content-Type", contentType)
			return &http.Response{StatusCode: 200, Header: responseHeaders, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
		})}
		state := fixtureState()
		result, err := verifySession(context.Background(), client, &state, http.Header{"User-Agent": {"fixture"}, "X-Goog-Authuser": {"0"}})
		if err != nil || result.Email != "primary@example.test" || result.ModelCount != 1 {
			t.Fatalf("verification=%+v err=%v", result, err)
		}
		if len(result.Models) != result.ModelCount || result.Models[0].ID != "fixture" {
			t.Fatal("verification did not return the verified catalog for account scheduling")
		}
		if (identityRequests == 0) != hasMetadata {
			t.Fatal("identity fallback called incorrectly")
		}
	}
}

func TestIdentityRejectsRedirectsAndBadResponses(t *testing.T) {
	for _, code := range []int{200, 302, 401, 500} {
		requests := 0
		client := &http.Client{Transport: identityTransport(func(r *http.Request) (*http.Response, error) {
			requests++
			return &http.Response{StatusCode: code, Header: http.Header{"Location": {"https://other.example/"}}, Body: io.NopCloser(strings.NewReader("invalid response")), Request: r}, nil
		})}
		state := fixtureState()
		if _, err := verifyGoogleIdentity(context.Background(), client, &state, "fixture"); err == nil || requests != 1 {
			t.Fatalf("bad response accepted or redirect followed: code=%d requests=%d", code, requests)
		}
	}
}

func TestIdentityEmptyBinaryResponseUsesStandardJSON(t *testing.T) {
	requests := 0
	client := &http.Client{Transport: identityTransport(func(r *http.Request) (*http.Response, error) {
		requests++
		body := ""
		if requests == 1 && r.URL.String() != googleAccountsURL {
			t.Fatal("wrong initial endpoint")
		}
		if requests == 2 {
			if r.URL.String() != googleAccountsJSONURL {
				t.Fatal("wrong fallback endpoint")
			}
			body = string(identityFixture(false, googleAccountIdentity{email: "primary@example.test", id: "fixture-1", valid: true, verified: true}))
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}
	state := fixtureState()
	email, err := verifyGoogleIdentity(context.Background(), client, &state, "fixture")
	if err != nil || email != "primary@example.test" || requests != 2 {
		t.Fatalf("identity fallback failed: requests=%d err=%v", requests, err)
	}
}
