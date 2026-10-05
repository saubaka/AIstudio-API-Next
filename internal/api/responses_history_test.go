package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Mag1cFall/AIStudio2API/internal/aistudio"
)

func TestWebSearchHistoryAfterModelSwitch(t *testing.T) {
	for _, action := range []string{
		`{"type":"search","query":"HISTORY_QUERY_91","queries":["second query"],"sources":[{"type":"url","url":"https://example.com/source"}]}`,
		`{"type":"open_page","url":"https://example.com/HISTORY_QUERY_91"}`,
		`{"type":"find_in_page","url":"https://example.com/source","pattern":"HISTORY_QUERY_91"}`,
		`null`,
	} {
		for _, status := range []string{"completed", "in_progress", "searching", "failed", "incomplete"} {
			for _, enabled := range []bool{false, true} {
				item := map[string]any{"id": "ws_old_model", "type": "web_search_call", "status": status, "action": json.RawMessage(action), "results": []any{map[string]any{"title": "HISTORY_RESULT_91", "$ref": "literal source metadata"}}}
				raw, _ := json.Marshal([]any{
					map[string]any{"type": "reasoning", "encrypted_content": "foreign_old_signature"}, item,
					map[string]any{"type": "function_call", "call_id": "local_call", "name": "read", "arguments": "{}"},
					map[string]any{"type": "function_call_output", "call_id": "local_call", "output": "LOCAL_RESULT"},
					map[string]any{"type": "message", "role": "assistant", "content": []any{map[string]string{"type": "output_text", "text": "Previous answer"}}},
					map[string]string{"role": "user", "content": "Continue on a different model"},
				})
				request := responsesRequest{Model: "different-model", Input: raw}
				if enabled {
					request.Tools = []responsesTool{{Type: "web_search"}}
				}
				gen, _, err := request.toGenerateRequest("new")
				if err != nil {
					t.Fatalf("action=%s status=%s enabled=%t: %v", action, status, enabled, err)
				}
				if len(gen.Contents) != 5 || gen.Contents[0].Role != aistudio.RoleAssistant {
					t.Fatalf("history order/role changed: %#v", gen.Contents)
				}
				history := gen.Contents[0].Parts[0]
				if !strings.Contains(history.Text, "HISTORY_RESULT_91") || !strings.Contains(history.Text, status) || !strings.Contains(history.Text, "literal source metadata") || history.FunctionCall != nil || history.FunctionResult != nil {
					t.Fatalf("hosted history was dropped or replayed: %#v", history)
				}
				if action != "null" && !strings.Contains(history.Text, "HISTORY_QUERY_91") {
					t.Fatal("search action metadata lost")
				}
				if gen.Contents[1].Parts[0].FunctionCall.ThoughtSignature != "" {
					t.Fatal("signature before hosted history leaked into a later local call")
				}
				if !enabled && len(gen.Tools.Google) != 0 {
					t.Fatal("historical call re-enabled web search")
				}
			}
		}
	}
}

func TestHostedOutputCanBeReplayedAsInput(t *testing.T) {
	png, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	result := generationResult{events: []aistudio.Event{
		{Kind: aistudio.EventGrounding, Grounding: &aistudio.GroundingMetadata{WebSearchQueries: []string{"ROUNDTRIP_QUERY"}, Chunks: []aistudio.GroundingChunk{{URI: "https://example.com/roundtrip"}}}},
		{Kind: aistudio.EventExecutableCode, ExecutableCode: &aistudio.ExecutableCode{Code: "print(17)"}},
		{Kind: aistudio.EventCodeExecutionResult, CodeExecutionResult: &aistudio.CodeExecutionResult{Outcome: "OUTCOME_OK", Output: "17"}},
	}, media: []aistudio.Media{{MIME: "image/png", Data: png}}}
	response, err := buildResponsesObject("old", 1, responsesRequest{Tools: []responsesTool{{Type: "web_search"}, {Type: "code_interpreter"}}}, result)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(response["output"])
	contents, _, err := responsesContents(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(contents) != 3 || !strings.Contains(contents[1].Parts[0].Text, "ROUNDTRIP_QUERY") || !strings.Contains(contents[0].Parts[0].Text, "print(17)") || !strings.Contains(contents[0].Parts[0].Text, `"logs":"17"`) {
		t.Fatalf("search/code history missing: %#v", contents)
	}
	image := contents[2].Parts[len(contents[2].Parts)-1].InlineData
	if image == nil || image.MIME != "image/png" || !bytes.Equal(image.Data, png) {
		t.Fatal("generated image history lost its pixels")
	}
	if _, _, err := responsesContents(json.RawMessage(`[{"type":"unrecognized_new_tool"}]`)); err == nil {
		t.Fatal("unknown items must not be silently dropped")
	}
	if _, _, err := responsesContents(json.RawMessage(`[{"type":"image_generation_call","result":"not base64"}]`)); err == nil {
		t.Fatal("invalid image history must fail clearly")
	}
}

type hostedHistoryFixture struct {
	channelFixture
	requests []aistudio.GenerateRequest
}

func (f *hostedHistoryFixture) Generate(_ context.Context, request aistudio.GenerateRequest) (<-chan aistudio.Event, error) {
	f.requests = append(f.requests, request)
	events := make(chan aistudio.Event, 3)
	events <- aistudio.Event{Kind: aistudio.EventGrounding, Grounding: &aistudio.GroundingMetadata{WebSearchQueries: []string{"MODEL_SWITCH_SEARCH_17"}}}
	events <- aistudio.Event{Kind: aistudio.EventText, Text: "SEARCH_HISTORY_OK"}
	events <- aistudio.Event{Kind: aistudio.EventFinish, FinishReason: "stop"}
	close(events)
	return events, nil
}

func TestResponsesHistoryReplayRoutesAndPreviousID(t *testing.T) {
	for _, prefix := range []string{"", "/playground", "/build"} {
		for _, stream := range []bool{false, true} {
			for _, stored := range []bool{false, true} {
				fixture := &hostedHistoryFixture{}
				handler := NewHandler(fixture, Config{})
				send := func(body any) *httptest.ResponseRecorder {
					raw, _ := json.Marshal(body)
					w := httptest.NewRecorder()
					handler.ServeHTTP(w, httptest.NewRequest("POST", prefix+"/v1/responses", bytes.NewReader(raw)))
					if w.Code != 200 || strings.Contains(w.Body.String(), "response.failed") {
						t.Fatalf("prefix=%s stream=%t stored=%t: %d %s", prefix, stream, stored, w.Code, w.Body.String())
					}
					return w
				}
				initial := send(map[string]any{"model": "model-before", "input": "search", "tools": []any{map[string]string{"type": "web_search"}}})
				var previous map[string]json.RawMessage
				if err := json.Unmarshal(initial.Body.Bytes(), &previous); err != nil {
					t.Fatal(err)
				}
				body := map[string]any{"model": "model-after", "stream": stream, "input": []any{map[string]string{"role": "user", "content": "continue"}}}
				if stored {
					body["previous_response_id"] = previous["id"]
				} else {
					var items []any
					_ = json.Unmarshal(previous["output"], &items)
					body["input"] = append(items, map[string]string{"role": "user", "content": "continue"})
				}
				last := send(body)
				if stream && !strings.Contains(last.Body.String(), `"type":"response.completed"`) {
					t.Fatal("stream did not complete")
				}
				request := fixture.requests[1]
				contents, _ := json.Marshal(request.Contents)
				if request.Model != "model-after" || !strings.Contains(string(contents), "MODEL_SWITCH_SEARCH_17") || len(request.Tools.Google) != 0 {
					t.Fatalf("switched history or disabled tool state lost: %s", contents)
				}
			}
		}
	}
}
