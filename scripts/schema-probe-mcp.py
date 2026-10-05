"""Disposable local MCP tool for Codex $defs/$ref compatibility acceptance."""
import json
import sys
from pathlib import Path

schema = {
    "$defs": {
        "Point": {
            "type": "object",
            "properties": {
                "line": {"type": "integer"},
                "label": {"$ref": "#/$defs/Label"},
            },
            "required": ["line", "label"],
            "additionalProperties": False,
        },
        "Label": {"type": "string", "enum": ["SCHEMA_REF_OK_17"]},
    },
    "type": "object",
    "properties": {"points": {"type": "array", "items": {"$ref": "#/$defs/Point"}}},
    "required": ["points"],
    "additionalProperties": False,
}
typed_probe = len(sys.argv) > 2 and sys.argv[2] == "typed-enum"
marker = "ENUM_TYPED_OK_22" if typed_probe else "SCHEMA_REF_OK_17"
expected = {"points": [{"line": 17, "label": marker}]}
if typed_probe:
    schema["$defs"]["Label"]["enum"] = [marker]
    schema["$defs"]["Point"]["properties"]["line"]["const"] = 22
    schema["$defs"]["Point"]["properties"]["enabled"] = {"type": "boolean", "const": True}
    schema["$defs"]["Point"]["required"].append("enabled")
    schema["properties"].update({
        "allowAsync": {"type": "boolean", "enum": [False]},
        "allowAsyncTrue": {"type": "boolean", "enum": [True]},
        "attempts": {"type": "integer", "enum": [17, 22]},
        "ratio": {"type": "number", "enum": [0.5, 1.25]},
    })
    schema["required"] += ["allowAsync", "allowAsyncTrue", "attempts", "ratio"]
    expected = {"points": [{"line": 22, "label": marker, "enabled": True}], "allowAsync": False, "allowAsyncTrue": True, "attempts": 22, "ratio": 1.25}

for line in sys.stdin:
    request = json.loads(line)
    if "id" not in request:
        continue
    method = request.get("method")
    if method == "initialize":
        result = {
            "protocolVersion": request["params"]["protocolVersion"],
            "capabilities": {"tools": {}},
            "serverInfo": {"name": "schema-reference-probe", "version": "1.0.0"},
        }
    elif method == "tools/list":
        result = {"tools": [{
            "name": "record_reference_probe",
            "description": "Records a local compatibility marker. Call with these exact JSON arguments: " + json.dumps(expected),
            "inputSchema": schema,
            "annotations": {"readOnlyHint": False, "destructiveHint": False, "openWorldHint": False, "idempotentHint": True},
        }]}
    elif method == "tools/call":
        params = request.get("params", {})
        arguments = params.get("arguments", {})
        passed = params.get("name") == "record_reference_probe" and arguments == expected
        if typed_probe:
            passed = passed and arguments.get("allowAsync") is False and arguments.get("allowAsyncTrue") is True and arguments.get("points", [{}])[0].get("enabled") is True
        evidence = {"tool": params.get("name"), "arguments": arguments, "passed": passed}
        Path(sys.argv[1]).write_text(json.dumps(evidence, indent=2))
        result = {
            "content": [{"type": "text", "text": marker if passed else "Schema arguments did not match"}],
            "isError": not passed,
        }
    elif method == "ping":
        result = {}
    else:
        print(json.dumps({"jsonrpc": "2.0", "id": request["id"], "error": {"code": -32601, "message": "Unsupported method"}}), flush=True)
        continue
    print(json.dumps({"jsonrpc": "2.0", "id": request["id"], "result": result}), flush=True)
