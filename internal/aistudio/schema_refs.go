package aistudio

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// Playground's positional schema has no definition table. Resolve references
// against the original document before encoding any child schema. Walk schema
// positions only: examples, defaults and property names are user data.
func expandLocalSchemaReferences(raw json.RawMessage) (json.RawMessage, error) {
	resolver := schemaReferenceResolver{root: raw, active: make(map[string]bool)}
	return resolver.expand(raw, 0)
}

type schemaReferenceResolver struct {
	root   json.RawMessage
	active map[string]bool
	nodes  int
}

func (r *schemaReferenceResolver) expand(raw json.RawMessage, depth int) (json.RawMessage, error) {
	r.nodes++
	if depth > 64 || r.nodes > 10000 {
		return nil, fmt.Errorf("schema 引用展开超过深度或节点上限")
	}
	var schema map[string]json.RawMessage
	if err := json.Unmarshal(raw, &schema); err != nil || schema == nil {
		return nil, fmt.Errorf("schema 必须是 JSON object")
	}
	if value, exists := schema["$ref"]; exists {
		var ref string
		if err := json.Unmarshal(value, &ref); err != nil {
			return nil, fmt.Errorf("schema.$ref 必须是字符串")
		}
		target, pointer, err := r.lookup(ref)
		if err != nil {
			return nil, err
		}
		if r.active[pointer] {
			return nil, fmt.Errorf("schema.$ref %q 包含循环引用，Playground 暂不支持递归工具参数", ref)
		}
		r.active[pointer] = true
		expanded, err := r.expand(target, depth+1)
		delete(r.active, pointer)
		if err != nil {
			return nil, err
		}
		delete(schema, "$ref")
		delete(schema, "$defs")
		delete(schema, "definitions")
		if len(schema) == 0 {
			return expanded, nil
		}
		// Siblings constrain the referenced schema too. Keep both constraints,
		// rather than overwriting an enum/type/property in the referenced target.
		siblings, _ := json.Marshal(schema)
		siblings, err = r.expand(siblings, depth+1)
		if err != nil {
			return nil, err
		}
		var result map[string]json.RawMessage
		_ = json.Unmarshal(expanded, &result)
		var constraints map[string]json.RawMessage
		_ = json.Unmarshal(siblings, &constraints)
		for _, name := range []string{"description", "title", "default", "examples", "example", "$comment"} {
			if value, ok := constraints[name]; ok {
				result[name] = value
				delete(constraints, name)
			}
		}
		if len(constraints) > 0 {
			var allOf []json.RawMessage
			if value, ok := result["allOf"]; ok {
				if err := json.Unmarshal(value, &allOf); err != nil {
					return nil, fmt.Errorf("schema.allOf 必须是 JSON object 数组")
				}
			}
			constraint, _ := json.Marshal(constraints)
			allOf = append(allOf, constraint)
			result["allOf"], _ = json.Marshal(allOf)
		}
		return json.Marshal(result)
	}
	delete(schema, "$defs")
	delete(schema, "definitions")
	for _, name := range []string{"properties", "patternProperties", "dependentSchemas"} {
		if value, ok := schema[name]; ok {
			var properties map[string]json.RawMessage
			if err := json.Unmarshal(value, &properties); err != nil || properties == nil {
				return nil, fmt.Errorf("schema.%s 必须是 JSON object", name)
			}
			names := make([]string, 0, len(properties))
			for property := range properties {
				names = append(names, property)
			}
			sort.Strings(names)
			for _, property := range names {
				expanded, err := r.expand(properties[property], depth+1)
				if err != nil {
					return nil, fmt.Errorf("schema.%s.%s: %w", name, property, err)
				}
				properties[property] = expanded
			}
			schema[name], _ = json.Marshal(properties)
		}
	}
	for _, name := range []string{"items", "not", "additionalProperties", "propertyNames", "contains", "if", "then", "else", "unevaluatedProperties", "unevaluatedItems"} {
		if value, ok := schema[name]; ok && len(value) > 0 && value[0] == '{' {
			expanded, err := r.expand(value, depth+1)
			if err != nil {
				return nil, fmt.Errorf("schema.%s: %w", name, err)
			}
			schema[name] = expanded
		}
	}
	for _, name := range []string{"anyOf", "oneOf", "allOf", "prefixItems"} {
		if value, ok := schema[name]; ok {
			var variants []json.RawMessage
			if err := json.Unmarshal(value, &variants); err != nil {
				return nil, fmt.Errorf("schema.%s 必须是 JSON object 数组", name)
			}
			for index, variant := range variants {
				expanded, err := r.expand(variant, depth+1)
				if err != nil {
					return nil, fmt.Errorf("schema.%s[%d]: %w", name, index, err)
				}
				variants[index] = expanded
			}
			schema[name], _ = json.Marshal(variants)
		}
	}
	return json.Marshal(schema)
}

func (r *schemaReferenceResolver) lookup(ref string) (json.RawMessage, string, error) {
	if !strings.HasPrefix(ref, "#") {
		return nil, "", fmt.Errorf("schema.$ref %q 只支持当前 schema 内的本地引用", ref)
	}
	pointer, err := url.PathUnescape(strings.TrimPrefix(ref, "#"))
	if err != nil || (pointer != "" && !strings.HasPrefix(pointer, "/")) {
		return nil, "", fmt.Errorf("schema.$ref %q 不是有效的本地 JSON Pointer", ref)
	}
	current := r.root
	if pointer == "" {
		return current, pointer, nil
	}
	for _, segment := range strings.Split(pointer[1:], "/") {
		for i := 0; i < len(segment); i++ {
			if segment[i] == '~' {
				if i+1 >= len(segment) || (segment[i+1] != '0' && segment[i+1] != '1') {
					return nil, "", fmt.Errorf("schema.$ref %q 包含无效 JSON Pointer 转义", ref)
				}
				i++
			}
		}
		segment = strings.ReplaceAll(strings.ReplaceAll(segment, "~1", "/"), "~0", "~")
		var object map[string]json.RawMessage
		if json.Unmarshal(current, &object) == nil && object != nil {
			var exists bool
			current, exists = object[segment]
			if !exists {
				return nil, "", fmt.Errorf("schema.$ref %q 指向不存在的定义", ref)
			}
			continue
		}
		var array []json.RawMessage
		index, err := strconv.Atoi(segment)
		if json.Unmarshal(current, &array) != nil || err != nil || index < 0 || index >= len(array) || strconv.Itoa(index) != segment {
			return nil, "", fmt.Errorf("schema.$ref %q 指向不存在的定义", ref)
		}
		current = array[index]
	}
	return current, pointer, nil
}
