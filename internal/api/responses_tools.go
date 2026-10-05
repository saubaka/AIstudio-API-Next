package api

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Mag1cFall/AIStudio2API/internal/aistudio"
)

// Stable names keep namespaces reversible even when two namespaces expose the
// same leaf name. Only the wire name changes; clients see their original tool.
func responseWireName(namespace, name string) string {
	if namespace == "" {
		return name
	}
	digest := sha256.Sum256([]byte(namespace + "\x00" + name))
	return fmt.Sprintf("ns_%x", digest[:12])
}

func responseToolDefinitions(tools []responsesTool) []responsesTool {
	var result []responsesTool
	for _, tool := range tools {
		if tool.Type == "namespace" {
			for _, inner := range tool.Tools {
				inner.Namespace = tool.Name
				result = append(result, inner)
			}
		} else {
			result = append(result, tool)
		}
	}
	return result
}

func responseToolForCall(tools []responsesTool, name string) (responsesTool, bool) {
	for _, tool := range responseToolDefinitions(tools) {
		leaf := tool.Name
		if tool.Type == "tool_search" {
			leaf = "tool_search"
		}
		if responseWireName(tool.Namespace, leaf) == name {
			return tool, true
		}
	}
	return responsesTool{}, false
}

func responseCustomInput(call aistudio.FunctionCall) string {
	var value struct {
		Input string `json:"input"`
	}
	if json.Unmarshal(call.Arguments, &value) == nil {
		return value.Input
	}
	return string(call.Arguments)
}

func responseToolCallItem(call aistudio.FunctionCall, tools []responsesTool) map[string]any {
	item := map[string]any{"id": "fc_" + call.ID, "type": "function_call", "status": "completed", "call_id": call.ID, "name": call.Name, "arguments": string(call.Arguments)}
	if tool, ok := responseToolForCall(tools, call.Name); ok {
		item["name"] = tool.Name
		if tool.Namespace != "" {
			item["namespace"] = tool.Namespace
		}
		switch tool.Type {
		case "custom":
			item["type"] = "custom_tool_call"
			item["input"] = responseCustomInput(call)
			delete(item, "arguments")
		case "tool_search":
			item["type"] = "tool_search_call"
			item["execution"] = "client"
			delete(item, "name")
			var arguments any
			_ = json.Unmarshal(call.Arguments, &arguments)
			item["arguments"] = arguments
		}
	}
	return item
}

// Expand dynamically discovered client tools and translate only tool history.
// Raw freeform input is preserved in an input string, never executed here.
func prepareResponsesTools(request *responsesRequest) error {
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(request.Input, &items); err != nil {
		return nil
	} // string input
	for _, item := range items {
		var kind string
		_ = json.Unmarshal(item["type"], &kind)
		if kind == "tool_search_output" {
			var loaded []responsesTool
			if err := json.Unmarshal(item["tools"], &loaded); err != nil {
				return fmt.Errorf("tool_search_output.tools: %w", err)
			}
			request.Tools = append(request.Tools, loaded...)
		}
	}
	// Identical rediscovered tools are merged; distinct duplicate definitions are
	// rejected by normal declaration validation rather than silently overwritten.
	var tools []responsesTool
	seen := map[string]string{}
	for _, tool := range request.Tools {
		raw, _ := json.Marshal(tool)
		identity := tool.Type + ":" + tool.Name
		if seen[identity] == string(raw) {
			continue
		}
		seen[identity] = string(raw)
		tools = append(tools, tool)
	}
	request.Tools = tools
	for _, item := range items {
		var kind, name, namespace string
		_ = json.Unmarshal(item["type"], &kind)
		_ = json.Unmarshal(item["name"], &name)
		_ = json.Unmarshal(item["namespace"], &namespace)
		switch kind {
		case "custom_tool_call":
			var input string
			if err := json.Unmarshal(item["input"], &input); err != nil {
				return fmt.Errorf("custom_tool_call.input must be text")
			}
			arguments, _ := json.Marshal(map[string]string{"input": input})
			item["arguments"], _ = json.Marshal(string(arguments))
			item["type"] = json.RawMessage(`"function_call"`)
		case "tool_search_call":
			name = "tool_search"
			item["type"] = json.RawMessage(`"function_call"`)
			var arguments string
			if json.Unmarshal(item["arguments"], &arguments) != nil {
				arguments = string(item["arguments"])
			}
			item["arguments"], _ = json.Marshal(arguments)
		case "custom_tool_call_output":
			item["type"] = json.RawMessage(`"function_call_output"`)
		case "tool_search_output":
			item["type"] = json.RawMessage(`"function_call_output"`)
			// Tool schemas are metadata, not function-result attachments. Gemini
			// interprets nested object keys named $ref as multimedia part refs.
			// Carry the exact discovered catalog as text in the history instead;
			// actual declarations have already been loaded above.
			item["output"], _ = json.Marshal(map[string]string{"tools": string(item["tools"])})
		}
		if kind == "function_call" || kind == "custom_tool_call" || kind == "tool_search_call" {
			wire := responseWireName(namespace, name)
			item["name"], _ = json.Marshal(wire)
			delete(item, "namespace")
		}
	}
	request.Input, _ = json.Marshal(items)
	return nil
}

func responseCustomParameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"input":{"type":"string","description":"Exact raw input for the custom tool. Preserve all newlines and syntax."}},"required":["input"],"additionalProperties":false}`)
}

func responseToolDescription(tool responsesTool) string {
	description := tool.Description
	if tool.Namespace != "" {
		description = "Namespace " + tool.Namespace + ", tool " + tool.Name + ". " + description
	}
	if tool.Type == "custom" {
		description += "\nUse the input field for the tool's exact freeform input."
		if len(tool.Format) > 0 && string(tool.Format) != "null" {
			description += "\nInput format specification: " + strings.TrimSpace(string(tool.Format))
		}
	}
	if tool.Name == "apply_patch" {
		description += "\nUse this dedicated tool to add, update or delete source files. Send the patch directly; do not wrap it in a shell command or an EOF heredoc. Read the target file first and preserve unrelated changes."
		description += "\nThe input STRING must start with *** Begin Patch and end with *** End Patch, without Markdown fences. Add file: *** Add File: path followed by +content lines. Update file: *** Update File: path followed by a standalone @@, unchanged context prefixed with a space, -old lines and +new lines. Do not use unified-diff line numbers (such as @@ -1,3 +1,3 @@). Example input string:\n*** Begin Patch\n*** Update File: example.txt\n@@\n-old value\n+new value\n*** End Patch"
	}
	return description
}

func responseAgentGuidance(tools []responsesTool) string {
	for _, tool := range responseToolDefinitions(tools) {
		if tool.Name == "apply_patch" {
			return "Client file workflow: follow applicable AGENTS.md instructions. Discover paths and read existing files with the client's read/search/shell tools (prefer scoped rg and sed). For source-file edits, call the dedicated apply_patch tool with its exact patch input; do not replace it with shell EOF/heredoc, cat >, or a Python file-write command. Use shell tools for commands and verification. Preserve unrelated work. User instructions about generated files or specific write methods take precedence."
		}
	}
	return ""
}
