package aistudio

import (
	"context"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

type selectedAccountPreparer struct{ received http.Header }

func (p *selectedAccountPreparer) Prepare(_ context.Context, r ProtectedRequest) (PreparedProtectedRequest, error) {
	return PreparedProtectedRequest{Body: r.Body, Headers: http.Header{"X-Goog-Authuser": {"0"}}}, nil
}
func (p *selectedAccountPreparer) BrowserStorageState(context.Context) (StorageState, error) {
	// Native browser export only includes cookies, without authuser metadata.
	return selectedAccountFixture(), nil
}
func (p *selectedAccountPreparer) SendProtected(_ context.Context, r ProtectedRequest) (*RPCResponse, error) {
	p.received = r.Headers
	return &RPCResponse{StatusCode: 200, Body: io.NopCloser(strings.NewReader("[]"))}, nil
}
func selectedAccountFixture() StorageState {
	state := StorageState{}
	for _, name := range []string{"SAPISID", "__Secure-1PAPISID", "__Secure-3PAPISID"} {
		state.Cookies = append(state.Cookies, StateCookie{Name: name, Value: "fixture", Domain: ".google.com", Path: "/", Secure: true, Expires: -1})
	}
	return state
}

func TestProtectedChannelsKeepSelectedGoogleAccount(t *testing.T) {
	for _, channel := range []Channel{ChannelPlayground, ChannelBuild} {
		t.Run(string(channel), func(t *testing.T) {
			state := selectedAccountFixture()
			if err := state.SetGoogleAuthUser("2"); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "state.json")
			if err := WriteStorageState(path, state); err != nil {
				t.Fatal(err)
			}
			account := &Account{ID: "fixture", Config: DefaultAccountConfig("fixture"), State: AccountReady, StoragePath: path}
			pool := NewAccountPool([]*Account{account}, 1)
			lease, err := pool.AcquireAccount(context.Background(), account.ID)
			if err != nil {
				t.Fatal(err)
			}
			defer lease.Release()
			lease.channel = channel
			worker := &selectedAccountPreparer{}
			transport := &WorkerProtectedTransport{transport: &MakerSuiteHTTPTransport{signer: NewSigner()}, workers: ProtectedPreparerProviderFunc(func(context.Context, string, string) (ProtectedPreparer, error) { return worker, nil })}
			ctx := ContextWithAccountLease(context.Background(), lease)
			response, err := transport.doBrowserPrepared(ctx, "fixture", 5, AccountSelection{Channel: channel}, RPCRequest{Body: []byte("[]")})
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if worker.received.Get("X-Goog-AuthUser") != "2" {
				t.Fatal("protected request lost selected Google account")
			}
		})
	}
}
