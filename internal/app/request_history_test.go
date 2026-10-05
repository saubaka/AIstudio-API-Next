package app

import (
	"context"
	"encoding/json"
	"github.com/Mag1cFall/AIStudio2API/internal/api"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRequestHistorySurvivesRestartAndFullWindowSnapshot(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	path := filepath.Join(t.TempDir(), "request-history.jsonl")
	first := newRequestRegistry(ctx)
	if err := first.loadHistory(path); err != nil {
		t.Fatal(err)
	}
	first.recordLog(api.AdminLog{Event: "request.finished", Request: &api.RequestLog{ID: "old-request", State: "completed", Usage: &api.RequestLogUsage{InputTokens: 1000, CachedTokens: 800, CacheTokensKnown: true}}})
	for range 250 {
		first.recordLog(api.AdminLog{Message: "progress"})
	}
	second := newRequestRegistry(ctx)
	if err := second.loadHistory(path); err != nil {
		t.Fatal(err)
	}
	subscriber := newEventSubscriber(ctx)
	events := second.activateSubscriber(subscriber, nil, nil)
	select {
	case event := <-events:
		entry, ok := event.Data.(api.AdminLog)
		if !ok || entry.Request == nil || entry.Request.ID != "old-request" || entry.Request.Usage.CachedTokens != 800 {
			t.Fatal("new window snapshot dropped old request")
		}
	case <-time.After(time.Second):
		t.Fatal("snapshot timed out")
	}
	if err := second.clearLogs(); err != nil {
		t.Fatal(err)
	}
	third := newRequestRegistry(ctx)
	if err := third.loadHistory(path); err != nil || len(third.logs) != 0 {
		t.Fatal("cleared history reappeared")
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatal("history file permissions too broad")
	}
}

func TestOldServiceRequestLogMigration(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dir := t.TempDir()
	_ = os.Mkdir(filepath.Join(dir, "logs"), 0700)
	data, _ := json.Marshal(map[string]any{"time": time.Now(), "level": "INFO", "msg": "request finished", "event": "request.finished", "request": map[string]string{"id": "legacy"}})
	_ = os.WriteFile(filepath.Join(dir, "logs/service.log"), append(data, '\n'), 0600)
	r := newRequestRegistry(ctx)
	if err := r.loadHistory(filepath.Join(dir, "request-history.jsonl")); err != nil || len(r.logs) != 1 || r.logs[0].Request.ID != "legacy" {
		t.Fatal("legacy history missing", err)
	}
}
