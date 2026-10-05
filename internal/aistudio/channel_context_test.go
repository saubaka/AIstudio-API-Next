package aistudio

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPinnedChannelCannotUseOtherQuotaOrSpecialCapability(t *testing.T) {
	model := Model{ID: "shared", Methods: []string{"generateContent"}, Capabilities: map[string]bool{"chat_model": true}}
	account := &Account{ID: "fixture", Config: DefaultAccountConfig("fixture"), State: AccountReady, Models: []Model{model}, buildModels: []Model{model}}
	pool := NewAccountPool([]*Account{account}, 1)
	pool.SetUpstreamChannels([]Channel{ChannelPlayground, ChannelBuild})
	// Build is cooling, but the same account's Playground quota is still free.
	account.runtime.Cooldowns["build:shared"] = CooldownState{Until: time.Now().Add(time.Hour)}
	for _, channel := range []Channel{ChannelPlayground, ChannelBuild} {
		ctx := ContextWithChannel(context.Background(), channel)
		selection, err := PinChannelSelection(ctx, AccountSelection{ModelID: "shared", Method: "generateContent", Channel: ChannelBuild})
		if err != nil || selection.Channel != channel {
			t.Fatal("channel pin lost to native Build preference")
		}
		groups, err := pool.ClassifyCandidates(ctx, selection, nil)
		if err != nil || !groups.Eligible {
			t.Fatal(err)
		}
		if channel == ChannelBuild && (len(groups.StandbyReady) != 0 || groups.EarliestCooldown.IsZero()) {
			t.Fatal("Build used Playground quota while cooling")
		}
		if channel == ChannelPlayground && len(groups.StandbyReady) != 1 {
			t.Fatal("Build cooldown blocked Playground")
		}
	}
	for _, selection := range []AccountSelection{
		{ModelID: "shared", Method: "countTokens"},
		{ModelID: "shared", Method: "generateContent", ResourceID: "drive-file"},
		{ModelID: "shared", Method: "generateContent", PlaygroundOnly: true},
	} {
		if _, err := PinChannelSelection(ContextWithChannel(context.Background(), ChannelBuild), selection); !errors.Is(err, ErrInvalidArgument) {
			t.Fatal("Build silently accepted Playground-only request")
		}
	}
	pool.SetUpstreamChannels([]Channel{ChannelPlayground})
	selection, _ := PinChannelSelection(ContextWithChannel(context.Background(), ChannelBuild), AccountSelection{ModelID: "shared", Method: "generateContent"})
	groups, err := pool.ClassifyCandidates(context.Background(), selection, nil)
	if err != nil || groups.Eligible {
		t.Fatal("disabled Build fell back to Playground")
	}
}

func TestChannelCatalogDoesNotMutateSharedSnapshot(t *testing.T) {
	models := []Model{{ID: "shared", Methods: []string{"generateContent", "countTokens"}, Channels: []string{"playground", "build"}}}
	filtered := ModelsForChannel(models, ChannelBuild)
	if len(filtered) != 1 || len(filtered[0].Methods) != 1 || len(filtered[0].Channels) != 1 {
		t.Fatal("invalid Build catalog")
	}
	if len(models[0].Methods) != 2 || len(models[0].Channels) != 2 {
		t.Fatal("filter mutated shared snapshot")
	}
}
