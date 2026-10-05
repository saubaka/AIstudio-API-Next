package api

import (
	"encoding/json"
	"github.com/Mag1cFall/AIStudio2API/internal/aistudio"
	"strings"
	"testing"
)

func TestCustomNamespaceAndSearchRoundtrip(t *testing.T) {
	request := responsesRequest{Model: "test", Tools: []responsesTool{
		{Type: "custom", Name: "apply_patch", Format: json.RawMessage(`{"type":"grammar","syntax":"lark","definition":"patch"}`)},
		{Type: "namespace", Name: "one", Tools: []responsesTool{{Type: "function", Name: "same", Strict: ptrBool(true)}}},
		{Type: "namespace", Name: "two", Tools: []responsesTool{{Type: "function", Name: "same"}}},
		{Type: "tool_search", Execution: "client", Parameters: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}}}`)},
	}, Input: json.RawMessage(`[{"type":"custom_tool_call","call_id":"c1","name":"apply_patch","input":"*** Begin Patch\n*** End Patch"},{"type":"custom_tool_call_output","call_id":"c1","output":"success"},{"type":"function_call","call_id":"c2","namespace":"one","name":"same","arguments":"{}"},{"type":"function_call_output","call_id":"c2","output":"OK"}]`)}
	gen, _, err := request.toGenerateRequest("test")
	if err != nil {
		t.Fatal(err)
	}
	if len(gen.Tools.Functions) != 4 || gen.Tools.Functions[1].Name == gen.Tools.Functions[2].Name {
		t.Fatal("namespace collision or lost tool")
	}
	call := *gen.Contents[0].Parts[0].FunctionCall
	item := responseFunctionCall(call, request.Tools)
	if item["type"] != "custom_tool_call" || item["input"] != "*** Begin Patch\n*** End Patch" {
		t.Fatalf("custom input lost: %v", item)
	}
	ns := responseFunctionCall(*gen.Contents[2].Parts[0].FunctionCall, request.Tools)
	if ns["name"] != "same" || ns["namespace"] != "one" {
		t.Fatalf("namespace lost: %v", ns)
	}
	if err := prepareResponsesTools(&request); err != nil {
		t.Fatal(err)
	}
	first := string(request.Input)
	if err := prepareResponsesTools(&request); err != nil {
		t.Fatal(err)
	}
	if string(request.Input) != first {
		t.Fatal("history mapping is not idempotent")
	}
}
func ptrBool(v bool) *bool { return &v }
func TestClientSearchLoadsTools(t *testing.T) {
	request := responsesRequest{Model: "test", Tools: []responsesTool{{Type: "tool_search", Execution: "client"}}, Input: json.RawMessage(`[{"type":"tool_search_call","execution":"client","call_id":"s1","arguments":{"query":"files"}},{"type":"tool_search_output","call_id":"s1","execution":"client","tools":[{"type":"namespace","name":"files","tools":[{"type":"function","name":"read","parameters":{"type":"object","properties":{}}}]}]},{"role":"user","content":"read the file"}]`)}
	if err := prepareResponsesTools(&request); err != nil {
		t.Fatal(err)
	}
	gen, _, err := request.toGenerateRequest("test")
	if err != nil {
		t.Fatal(err)
	}
	if len(gen.Tools.Functions) != 2 {
		t.Fatal("discovered tool not loaded")
	}
	item := responseFunctionCall(aistudio.FunctionCall{ID: "s2", Name: "tool_search", Arguments: json.RawMessage(`{"query":"files"}`)}, request.Tools)
	if item["type"] != "tool_search_call" || item["execution"] != "client" {
		t.Fatal(item)
	}
	if !strings.Contains(gen.Contents[0].Parts[0].FunctionCall.Name, "tool_search") {
		t.Fatal("search history lost")
	}
}

func TestClientSearchSchemaReferencesRemainLiteralHistory(t *testing.T) {
	request := responsesRequest{Tools: []responsesTool{{Type: "tool_search", Execution: "client"}}, Input: json.RawMessage(`[
		{"type":"tool_search_call","execution":"client","call_id":"s1","arguments":{"query":"probe"}},
		{"type":"tool_search_output","call_id":"s1","execution":"client","tools":[{"type":"namespace","name":"probe","tools":[{"type":"function","name":"record","parameters":{"$defs":{"Label":{"type":"string"}},"type":"object","properties":{"label":{"$ref":"#/$defs/Label"}}}}]}]}
	]`)}
	gen, _, err := request.toGenerateRequest("test")
	if err != nil {
		t.Fatal(err)
	}
	if len(gen.Tools.Functions) != 2 || !strings.Contains(string(gen.Tools.Functions[1].Parameters), `"$defs"`) {
		t.Fatal("discovered declaration lost its schema")
	}
	var result map[string]any
	if err := json.Unmarshal(gen.Contents[1].Parts[0].FunctionResult.Content, &result); err != nil {
		t.Fatal(err)
	}
	metadata, ok := result["tools"].(string)
	if !ok || !strings.Contains(metadata, `"$ref":"#/$defs/Label"`) || !json.Valid([]byte(metadata)) {
		t.Fatalf("schema metadata must be preserved as text: %#v", result)
	}
	first := string(request.Input)
	if err := prepareResponsesTools(&request); err != nil {
		t.Fatal(err)
	}
	second := string(request.Input)
	if err := prepareResponsesTools(&request); err != nil || string(request.Input) != second {
		t.Fatal("discovery history normalization must be idempotent", first, err)
	}
}
