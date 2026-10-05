package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Mag1cFall/AIStudio2API/internal/aistudio"
)

type channelFixture struct {
	channel aistudio.Channel
	calls   int
}

func (*channelFixture) Models(context.Context) ([]aistudio.Model, error) {
	return []aistudio.Model{
		{ID: "shared", Methods: []string{"generateContent", "countTokens"}, Channels: []string{"playground", "build"}},
		{ID: "studio-only", Methods: []string{"generateContent"}, Channels: []string{"playground"}},
		{ID: "build-only", Methods: []string{"generateContent"}, Channels: []string{"build"}},
	}, nil
}
func (*channelFixture) CountTokens(context.Context, aistudio.TokenCountRequest) (aistudio.TokenCount, error) {
	return aistudio.TokenCount{}, nil
}
func (f *channelFixture) Generate(ctx context.Context, _ aistudio.GenerateRequest) (<-chan aistudio.Event, error) {
	f.channel = aistudio.ChannelFromContext(ctx)
	f.calls++
	events := make(chan aistudio.Event, 2)
	events <- aistudio.Event{Kind: aistudio.EventText, Text: "OK"}
	events <- aistudio.Event{Kind: aistudio.EventFinish, FinishReason: "stop"}
	close(events)
	return events, nil
}

func TestDedicatedChannelProtocolsAndAuth(t *testing.T) {
	for _, channel := range []string{"playground", "build"} {
		for _, stream := range []bool{false, true} {
			for _, protocol := range []struct{ path, body string }{
				{"/v1/chat/completions", `{"model":"shared","messages":[{"role":"user","content":"hi"}]}`},
				{"/v1/responses", `{"model":"shared","input":"hi"}`},
				{"/v1/messages", `{"model":"shared","max_tokens":128,"messages":[{"role":"user","content":"hi"}]}`},
				{"/v1beta/models/shared:generateContent", `{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`},
			} {
				t.Run(channel+protocol.path+map[bool]string{true: "/stream", false: "/json"}[stream], func(t *testing.T) {
					f := &channelFixture{}
					handler := NewHandler(f, Config{APIKey: "test-key"})
					path, body := protocol.path, protocol.body
					if stream {
						if strings.Contains(path, "generateContent") {
							path = strings.Replace(path, ":generateContent", ":streamGenerateContent", 1)
						} else {
							body = strings.TrimSuffix(body, "}") + `,"stream":true}`
						}
					}
					for _, authorized := range []bool{false, true} {
						r := httptest.NewRequest("POST", "/"+channel+path, strings.NewReader(body))
						r.Header.Set("Content-Type", "application/json")
						if authorized {
							r.Header.Set("Authorization", "Bearer test-key")
						}
						w := httptest.NewRecorder()
						handler.ServeHTTP(w, r)
						if !authorized {
							if w.Code != 401 || f.calls != 0 {
								t.Fatal("channel route bypassed authentication")
							}
							continue
						}
						if w.Code != 200 || f.channel != aistudio.Channel(channel) || !strings.Contains(w.Body.String(), "OK") || w.Header().Get("X-AIStudio-Channel") != channel {
							t.Fatalf("code=%d channel=%s body=%s", w.Code, f.channel, w.Body.String())
						}
						if stream && !strings.Contains(w.Header().Get("Content-Type"), "text/event-stream") {
							t.Fatal("stream route lost SSE")
						}
					}
				})
			}
		}
	}
}

func TestDedicatedCatalogAndUnsupportedBuildRoutes(t *testing.T) {
	f := &channelFixture{}
	handler := NewHandler(f, Config{})
	for _, channel := range []string{"playground", "build"} {
		for _, path := range []string{"/v1/models", "/v1beta/models"} {
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, httptest.NewRequest("GET", "/"+channel+path, nil))
			if w.Code != 200 || strings.Contains(w.Body.String(), map[string]string{"playground": "build-only", "build": "studio-only"}[channel]) {
				t.Fatalf("catalog leaked other channel: %s", w.Body.String())
			}
			var object map[string]any
			if json.Unmarshal(w.Body.Bytes(), &object) != nil {
				t.Fatal("invalid model JSON")
			}
			if channel == "build" && strings.Contains(w.Body.String(), "countTokens") {
				t.Fatal("Build advertised Playground-only method")
			}
		}
	}
	for _, path := range []string{"/v1/videos", "/v1/audio/transcriptions", "/v1/messages/count_tokens", "/v1beta/interactions", "/v1beta/models/shared:countTokens"} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("POST", "/build"+path, strings.NewReader("{}")))
		if w.Code != 501 || f.calls != 0 {
			t.Fatalf("unsupported Build route escaped: path=%s code=%d", path, w.Code)
		}
	}
	for _, path := range []string{"/v1/models", "/playground/v1/models", "/build/v1/models"} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 {
			t.Fatal("catalog unavailable")
		}
	}
}
