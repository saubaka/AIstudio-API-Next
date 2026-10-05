package aistudio

import (
	"encoding/json"
	"testing"
)

func TestPlaygroundCacheUsagePreservesAuthoritativeValues(t *testing.T) {
	for _, raw := range []string{`[1000,20,1050,800,null,null,null,0,null,30]`, `[1000,20,1050,null,null,null,null,0,null,30]`} {
		usage, complete, err := decodeUsage(json.RawMessage(raw), json.RawMessage(raw))
		if err != nil || !complete || usage.InputTokens != 1000 || usage.TotalTokens != 1050 {
			t.Fatal("usage changed", err)
		}
	}
	usage, _, err := decodeUsage(json.RawMessage(`[1000,20,1020,800]`), nil)
	if err != nil || usage.CachedTokens != 800 || !usage.CacheTokensKnown {
		t.Fatal("cache count lost", err)
	}
	missing, _, err := decodeUsage(json.RawMessage(`[1000,20,1020]`), nil)
	if err != nil || missing.CacheTokensKnown || missing.CachedTokens != 0 {
		t.Fatal("missing cache metadata was invented")
	}
}

func TestBuildCacheUsagePreservesAuthoritativeValues(t *testing.T) {
	decoder := &BuildStreamDecoder{}
	events, err := decoder.Decode([]byte(`{"candidates":[{"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":1000,"candidatesTokenCount":20,"totalTokenCount":1020,"cachedContentTokenCount":800}}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.Kind == EventUsage {
			if event.Usage.CachedTokens != 800 || !event.Usage.CacheTokensKnown || event.Usage.InputTokens != 1000 {
				t.Fatal("build cache count dropped")
			}
			return
		}
	}
	t.Fatal("missing usage event")
}
