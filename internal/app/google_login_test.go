package app

import (
	"bytes"
	"context"
	"github.com/Mag1cFall/AIStudio2API/internal/aistudio"
	"github.com/Mag1cFall/AIStudio2API/internal/browserlogin"
	"github.com/Mag1cFall/AIStudio2API/internal/chromeauth"
	"os"
	"path/filepath"
	"testing"
)

func TestRepeatVerifiedGoogleImportRefreshesExistingAccount(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	state, err := browserlogin.ParseSessionData("SAPISID=fixture-a; __Secure-1PAPISID=fixture-b; __Secure-3PAPISID=fixture-c")
	if err != nil {
		t.Fatal(err)
	}
	id := "repeat@example.test"
	_ = state.SetAuthExtension(aistudio.AuthExtension{Source: aistudio.AuthSource{Email: id}})
	store := aistudio.NewAccountStore(t.TempDir())
	cfg := aistudio.DefaultAccountConfig(id)
	account, lease, err := store.Create(cfg, state)
	if err != nil {
		t.Fatal(err)
	}
	if err := lease.Release(); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(account.ConfigPath)
	sentinel := filepath.Join(account.Directory, "preserve-me.txt")
	_ = os.WriteFile(sentinel, []byte("preserve"), 0600)
	pool := aistudio.NewAccountPool([]*aistudio.Account{account}, 1)
	requests := newRequestRegistry(ctx)
	service := &trackedService{lifecycle: ctx, pool: pool, requests: requests}
	workers := &accountWorkerManager{accounts: map[string]*accountWorker{id: {id: id, label: id}}}
	admin := &runtimeAdmin{pool: pool, store: store, workers: workers, service: service, requests: requests, verifySession: func(context.Context, *aistudio.StorageState, string) (chromeauth.Verification, error) {
		return chromeauth.Verification{Email: id, ModelCount: 1}, nil
	}}
	_ = state.SetGoogleAuthUser("2")
	state.Cookies[0].Value = "rotated"
	result, err := admin.ImportGoogleSession(ctx, browserlogin.Result{State: state})
	if err != nil || result.ID != id || len(pool.Status()) != 1 {
		t.Fatal("repeat import did not refresh account", err)
	}
	loaded, err := aistudio.LoadStorageState(account.StoragePath)
	if err != nil || loaded.GoogleAuthUser() != "2" || loaded.Cookies[0].Value != "rotated" {
		t.Fatal("new credentials were not stored", err)
	}
	after, _ := os.ReadFile(account.ConfigPath)
	if !bytes.Equal(before, after) {
		t.Fatal("refresh replaced existing settings")
	}
	if data, err := os.ReadFile(sentinel); err != nil || string(data) != "preserve" {
		t.Fatal("refresh replaced account directory")
	}
}

func TestGoogleSessionIdentityCannotOverwriteDifferentAccount(t *testing.T) {
	cases := []struct {
		browser, server, target string
		valid                   bool
	}{
		{"user@example.test", "user@example.test", "user@example.test", true},
		{"USER@EXAMPLE.TEST", "user@example.test", "", true},
		{"", "user@example.test", "", true},
		{"user@example.test", "", "", true},
		{"other@example.test", "user@example.test", "", false},
		{"user@example.test", "user@example.test", "different@example.test", false},
		{"", "", "", false},
		{"not-email", "", "", false},
	}
	for _, c := range cases {
		result := browserlogin.Result{Email: c.browser, Input: browserlogin.StartInput{AccountID: c.target}}
		email, err := googleSessionEmail(result, chromeauth.Verification{Email: c.server, ModelCount: 1})
		if (err == nil) != c.valid {
			t.Fatalf("identity validation failed: browser=%q server=%q target=%q", c.browser, c.server, c.target)
		}
		if c.valid && email != "user@example.test" {
			t.Fatal("identity was not normalized")
		}
	}
}
