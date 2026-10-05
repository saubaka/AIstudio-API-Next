package aistudio

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestSchemaDefinitionsEncodeLikeInlineParameters(t *testing.T) {
	withRefs := json.RawMessage(`{
		"$defs":{
			"Point":{"type":"object","properties":{"line":{"type":"integer"},"label":{"$ref":"#/$defs/Label"}},"required":["line","label"],"additionalProperties":false},
			"Label":{"type":"string","enum":["SCHEMA_REF_OK_17"]},
			"Unused":{"$ref":"#/$defs/Unused"}
		},
		"type":"object","properties":{
			"points":{"type":"array","items":{"$ref":"#/$defs/Point"}},
			"maybe":{"anyOf":[{"$ref":"#/$defs/Label"},{"type":"null"}]},
			"$defs":{"type":"string"}
		},"required":["points","maybe"],"additionalProperties":false
	}`)
	inline := json.RawMessage(`{"type":"object","properties":{
		"points":{"type":"array","items":{"type":"object","properties":{"line":{"type":"integer"},"label":{"type":"string","enum":["SCHEMA_REF_OK_17"]}},"required":["line","label"],"additionalProperties":false}},
		"maybe":{"anyOf":[{"type":"string","enum":["SCHEMA_REF_OK_17"]},{"type":"null"}]},
		"$defs":{"type":"string"}
	},"required":["points","maybe"],"additionalProperties":false}`)
	want, err := encodeJSONSchema(inline)
	if err != nil {
		t.Fatal(err)
	}
	got, err := encodeJSONSchema(withRefs)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("reference expansion changed wire structure:\ngot %#v\nwant %#v", got, want)
	}
}

func TestSchemaReferencePointersAndSiblingConstraints(t *testing.T) {
	raw := json.RawMessage(`{"definitions":{"a/b~c":{"type":"string","minLength":2}},"type":"object","properties":{"value":{"$ref":"#/definitions/a~1b~0c","description":"local description","maxLength":9},"reused":{"$ref":"#/definitions/a%7E1b%7E0c"}},"default":{"$ref":"literal user data","$defs":{"literal":true}}}`)
	expanded, err := expandLocalSchemaReferences(raw)
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Properties map[string]struct {
			Type        string           `json:"type"`
			Description string           `json:"description"`
			MinLength   int              `json:"minLength"`
			AllOf       []map[string]int `json:"allOf"`
		} `json:"properties"`
		Default map[string]json.RawMessage `json:"default"`
	}
	if err := json.Unmarshal(expanded, &schema); err != nil {
		t.Fatal(err)
	}
	value := schema.Properties["value"]
	if value.Type != "string" || value.MinLength != 2 || value.Description != "local description" || len(value.AllOf) != 1 || value.AllOf[0]["maxLength"] != 9 {
		t.Fatalf("reference or sibling constraint lost: %s", expanded)
	}
	if schema.Properties["reused"].Type != "string" || schema.Default["$ref"] == nil || schema.Default["$defs"] == nil {
		t.Fatalf("reused reference or literal data changed: %s", expanded)
	}
	if _, err := encodeJSONSchema(raw); err != nil {
		t.Fatal(err)
	}
}

func TestSchemaReferenceInvalidInputsFailClearly(t *testing.T) {
	for _, test := range []struct{ name, raw, message string }{
		{"missing", `{"$ref":"#/$defs/Missing"}`, "不存在"},
		{"external", `{"$ref":"https://example.invalid/schema.json"}`, "本地引用"},
		{"self", `{"$ref":"#"}`, "循环引用"},
		{"mutual", `{"$defs":{"a":{"$ref":"#/$defs/b"},"b":{"$ref":"#/$defs/a"}},"$ref":"#/$defs/a"}`, "循环引用"},
		{"bad escape", `{"$ref":"#/bad~2"}`, "无效 JSON Pointer"},
		{"not string", `{"$ref":2}`, "字符串"},
		{"not schema", `{"$defs":{"x":42},"$ref":"#/$defs/x"}`, "JSON object"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := encodeJSONSchema(json.RawMessage(test.raw)); err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("wanted %s error, got %v", test.message, err)
			}
		})
	}
	deep := `{"type":"string"}`
	for i := 0; i < 70; i++ {
		deep = `{"type":"array","items":` + deep + `}`
	}
	if _, err := encodeJSONSchema(json.RawMessage(deep)); err == nil || !strings.Contains(err.Error(), "上限") {
		t.Fatalf("deep schema must be bounded, got %v", err)
	}
}
