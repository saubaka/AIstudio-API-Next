"""Live reproduction and both-channel typed-enum/function-history acceptance."""
import json
import os
import urllib.request
from pathlib import Path

root = Path(__file__).resolve().parents[1]
base = root / "runtime" / "enum-qa"
base.mkdir(parents=True, exist_ok=True)
key = next(line.split("=", 1)[1].strip().strip("\"'") for line in (root / ".env").read_text().splitlines() if line.startswith("PROXY_API_KEY="))
schema = {
    "$defs": {"Job": {
        "type": "object",
        "properties": {"enabled": {"type": "boolean", "const": True}, "priority": {"type": "integer", "const": 17}},
        "required": ["enabled", "priority"], "additionalProperties": False,
    }},
    "type": "object",
    "properties": {
        "allowAsync": {"type": "boolean", "enum": [False]},
        "allowAsyncTrue": {"type": "boolean", "enum": [True]},
        "attempts": {"type": "integer", "enum": [17, 22]},
        "ratio": {"type": "number", "enum": [0.5, 1.25]},
        "jobs": {"type": "array", "items": {"$ref": "#/$defs/Job"}},
        "marker": {"type": "string", "enum": ["ENUM_TYPED_OK_22"]},
    },
    "required": ["allowAsync", "allowAsyncTrue", "attempts", "ratio", "jobs", "marker"],
    "additionalProperties": False,
}
expected = {"allowAsync": False, "allowAsyncTrue": True, "attempts": 22, "ratio": 1.25, "jobs": [{"enabled": True, "priority": 17}], "marker": "ENUM_TYPED_OK_22"}
tools = [{"type": "function", "name": "unused_" + str(i), "description": "Do not call this test placeholder.", "parameters": {"type": "object", "properties": {}}} for i in range(22)]
tools.append({"type": "function", "name": "typed_enum_probe", "parameters": schema, "description": "Record the typed enum compatibility marker.", "strict": True})
initial = {"model": "gemini-3.8-flash", "tools": tools, "input": [{"role": "user", "content": "Call typed_enum_probe with exactly these JSON arguments, then wait for the result: " + json.dumps(expected) + ". Do not call the other tools."}]}
(base / "reproduction-request.json").write_text(json.dumps(initial, indent=2))

def ask(channel, payload):
    req = urllib.request.Request("http://127.0.0.1:2048/" + channel + "/v1/responses", data=json.dumps(payload).encode(), headers={"Authorization": "Bearer " + key, "Content-Type": "application/json"})
    with urllib.request.urlopen(req, timeout=150) as response:
        raw = response.read().decode()
        header = response.headers.get("X-AIStudio-Channel")
        if not payload.get("stream"):
            return json.loads(raw), [], header
        events = []
        for line in raw.splitlines():
            if line.startswith("data: "):
                try:
                    events.append(json.loads(line[6:]))
                except ValueError:
                    pass
        completed = next((e for e in events if e.get("type") == "response.completed"), None)
        if completed is None:
            failed = next((e for e in events if e.get("type") == "response.failed"), {})
            return failed.get("response", {}), [e.get("type") for e in events], header
        return completed["response"], [e.get("type") for e in events], header

if os.environ.get("QA_CAPTURE_FAILURE") == "1":
    initial["stream"] = True
    response, events, _ = ask("playground", initial)
    error = response.get("error", {})
    assert "function declaration 22" in error.get("message", "") and "allowAsync" in error.get("message", "") and "schema.enum" in error.get("message", ""), error
    record = {"status": response.get("status"), "error": error, "event_types": events}
    (base / "before-fix.json").write_text(json.dumps(record, ensure_ascii=False, indent=2))
    print(json.dumps(record, ensure_ascii=False))
    raise SystemExit(0)

records = []
for channel in ["playground", "build"]:
    for stream in [True, False]:
        payload = json.loads(json.dumps(initial))
        payload["stream"] = stream
        response, events, header = ask(channel, payload)
        assert header == channel, {"expected_channel": channel, "actual_channel": header}
        assert response.get("status") == "completed", response.get("error")
        call = next(x for x in response["output"] if x["type"] == "function_call")
        arguments = json.loads(call["arguments"])
        assert call["name"] == "typed_enum_probe" and arguments == expected, arguments
        assert type(arguments["allowAsync"]) is bool and arguments["allowAsync"] is False
        assert type(arguments["allowAsyncTrue"]) is bool and arguments["allowAsyncTrue"] is True
        assert type(arguments["attempts"]) in [int, float] and not isinstance(arguments["attempts"], bool)
        assert type(arguments["ratio"]) in [int, float] and not isinstance(arguments["ratio"], bool)
        assert type(arguments["jobs"][0]["enabled"]) is bool
        payload["input"] += response["output"] + [{"type": "function_call_output", "call_id": call["call_id"], "output": "ENUM_TYPED_OK_22"}, {"role": "user", "content": "Return the actual tool result ENUM_TYPED_OK_22 only. Do not call any tools."}]
        final, final_events, final_header = ask(channel, payload)
        assert final_header == channel, {"expected_channel": channel, "actual_channel": final_header}
        text = "".join(p.get("text", "") for x in final.get("output", []) for p in x.get("content", []))
        assert final.get("status") == "completed" and "ENUM_TYPED_OK_22" in text, final.get("error")
        record = {"channel": channel, "model": payload["model"], "stream": stream, "channel_header": header, "tool_count": len(tools), "actual_arguments": arguments, "types_preserved": True, "result_return": text, "event_types": events, "final_event_types": final_events, "passed": True}
        records.append(record)
        (base / "after-fix.json").write_text(json.dumps(records, indent=2))
        print(json.dumps(record), flush=True)
print("PASSED: both channels, streaming and nonstreaming, typed enums and tool results")
