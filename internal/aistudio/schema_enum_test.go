package aistudio

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestTypedEnumWireKeepsJSONTypes(t *testing.T) {
	for _, test := range []struct {
		name, schema string
		code         int64
		values       []string
	}{
		{"allowAsync false", `{"type":"boolean","enum":[false]}`, 4, []string{"false"}},
		{"allowAsync true", `{"type":"boolean","enum":[true]}`, 4, []string{"true"}},
		{"both booleans", `{"type":"boolean","enum":[false,true]}`, 4, []string{"false", "true"}},
		{"integer", `{"type":"integer","enum":[17,22]}`, 3, []string{"17", "22"}},
		{"number", `{"type":"number","enum":[1.25,-0.5,1e3]}`, 2, []string{"1.25", "-0.5", "1e3"}},
		{"large integer", `{"type":"integer","enum":[9007199254740993]}`, 3, []string{"9007199254740993"}},
		{"inferred boolean", `{"enum":[false,true]}`, 4, []string{"false", "true"}},
		{"inferred number", `{"enum":[1.25,2]}`, 2, []string{"1.25", "2"}},
		{"boolean const", `{"type":"boolean","const":false}`, 4, []string{"false"}},
		{"inferred boolean const", `{"const":true}`, 4, []string{"true"}},
		{"integer const", `{"type":"integer","const":22}`, 3, []string{"22"}},
		{"number const", `{"const":1.25}`, 2, []string{"1.25"}},
		{"string unchanged", `{"type":"string","enum":["false","22"]}`, 1, []string{"false", "22"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			wire, err := encodeJSONSchema(json.RawMessage(test.schema))
			if err != nil {
				t.Fatal(err)
			}
			if len(wire) < 5 || wire[0] != test.code || !reflect.DeepEqual(wire[4], test.values) {
				t.Fatalf("type or enum changed: %#v", wire)
			}
			// Structured output projection must preserve the same scalar type.
			projection := buildResponseSchema(wire)
			if projection["type"] == "STRING" && test.code != 1 {
				t.Fatal("non-string type converted to STRING")
			}
		})
	}
}

func TestFunction22AllowAsyncWithNestedDefinitions(t *testing.T) {
	tools := Tools{}
	for i := 0; i < 22; i++ {
		tools.Functions = append(tools.Functions, FunctionDeclaration{Name: "placeholder", Parameters: json.RawMessage(`{"type":"object"}`)})
	}
	tools.Functions = append(tools.Functions, FunctionDeclaration{Name: "probe", Parameters: json.RawMessage(`{
		"$defs":{"Job":{"type":"object","properties":{"enabled":{"const":true},"priority":{"type":"integer","enum":[17,22]}}}},
		"type":"object","properties":{"allowAsync":{"type":"boolean","enum":[false]},"jobs":{"type":"array","items":{"$ref":"#/$defs/Job"}}}
	}`)})
	if _, _, err := encodeRequestedTools(tools); err != nil {
		t.Fatal(err)
	}
}

func TestInvalidEnumStructureDoesNotDisappear(t *testing.T) {
	for _, raw := range []string{`{"enum":null}`, `{"enum":[]}`, `{"enum":true}`, `{"enum":[{}]}`, `{"enum":[[]]}`} {
		if _, err := encodeJSONSchema(json.RawMessage(raw)); err == nil || !strings.Contains(err.Error(), "enum") {
			t.Fatalf("invalid enum silently accepted: %s, %v", raw, err)
		}
	}
}
