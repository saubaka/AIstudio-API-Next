package app

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Mag1cFall/AIStudio2API/internal/aistudio"
	"github.com/Mag1cFall/AIStudio2API/internal/chromeauth"
)

// A successful verification during startup must leave a schedulable bootstrap
// model, including when the account had no cached catalog before verification.
func TestVerifyAccountDuringStartupKeepsBootstrapCandidate(t *testing.T) {
	for _, cached := range []bool{false, true} {
		t.Run(map[bool]string{false: "empty_cache", true: "synced_cache"}[cached], func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			dir := t.TempDir()
			model := aistudio.Model{ID: "gemini-flash-latest", Methods: []string{"generateContent"}, Capabilities: map[string]bool{"chat_model": true}}
			account := &aistudio.Account{
				ID: "fixture@example.test", Config: aistudio.DefaultAccountConfig("fixture"),
				State: aistudio.AccountReady, StoragePath: filepath.Join(dir, "state.json"),
				RuntimePath: filepath.Join(dir, "runtime.json"),
			}
			if cached {
				account.Models = []aistudio.Model{model}
			}
			pool := aistudio.NewAccountPool([]*aistudio.Account{account}, 1)
			requests := newRequestRegistry(ctx)
			service := &trackedService{lifecycle: ctx, pool: pool, requests: requests}
			service.state.Store(serviceLaunching)
			admin := &runtimeAdmin{
				pool: pool, service: service, requests: requests,
				verifySession: func(context.Context, *aistudio.StorageState, string) (chromeauth.Verification, error) {
					return chromeauth.Verification{Email: account.ID, ModelCount: 1, Models: []aistudio.Model{model}}, nil
				},
			}
			if _, err := admin.VerifyAccount(ctx, account.ID); err != nil {
				t.Fatal(err)
			}
			models, err := pool.BootstrapModels(account.ID)
			if err != nil || len(models) != 1 || models[0] != model.ID {
				t.Fatalf("verification removed bootstrap catalog: models=%v err=%v", models, err)
			}
			groups, err := pool.ClassifyCandidates(ctx, aistudio.AccountSelection{ModelID: model.ID, Method: "generateContent"}, nil)
			if err != nil || !groups.Eligible || len(groups.StandbyReady) != 1 {
				t.Fatalf("startup cannot prewarm verified account: groups=%+v err=%v", groups, err)
			}
		})
	}
}
