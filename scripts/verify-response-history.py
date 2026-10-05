"""Live replay of hosted Responses history, including a change of model.

Only synthetic QA text is sent. Keys and private desktop history are never
written to evidence. HTTP 200 alone does not count as a completed response.
"""
import copy
import json
import urllib.error
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
BASE = ROOT / 'runtime/web-search-history-qa'
BASE.mkdir(exist_ok=True)
KEY = next(line.split('=', 1)[1].strip().strip('\"\'') for line in (ROOT / '.env').read_text().splitlines() if line.startswith('PROXY_API_KEY='))
OPENER = urllib.request.build_opener(urllib.request.ProxyHandler({}))
ORIGINAL = json.loads((BASE / 'request.json').read_text())
RECORDS = []


def ask(channel, body, name):
    request = urllib.request.Request('http://127.0.0.1:2048/' + channel + '/v1/responses', data=json.dumps(body).encode(), headers={'Content-Type': 'application/json', 'Authorization': 'Bearer ' + KEY})
    record = {'case': name, 'channel': channel, 'model': body['model'], 'stream': body.get('stream', False), 'uses_previous_id': bool(body.get('previous_response_id')), 'input_types': [item.get('type', 'message') for item in body['input']]}
    RECORDS.append(record)
    try:
        with OPENER.open(request, timeout=180) as response:
            record['http_status'] = response.status
            record['channel_header'] = response.headers.get('X-AIStudio-Channel')
            raw = response.read().decode()
        if body.get('stream'):
            events = [json.loads(line[6:]) for line in raw.splitlines() if line.startswith('data: ') and line[6:] != '[DONE]']
            record['event_types'] = [e.get('type') for e in events]
            errors = [e for e in events if e.get('type') in ['response.failed', 'error', 'response.incomplete']]
            assert not errors, json.dumps(errors)
            completed = [e['response'] for e in events if e.get('type') == 'response.completed']
            assert len(completed) == 1, 'Missing completion event'
            result = completed[0]
        else:
            result = json.loads(raw)
        record['completed'] = result.get('status') == 'completed'
        text = result.get('output_text', '') or ''.join(p.get('text', '') for item in result.get('output', []) for p in item.get('content', []))
        record['marker_returned'] = 'WEB_HISTORY_CONTEXT_17' in text
        record['response_id'] = result['id']
        record['provider_model'] = result.get('provider_model')
        record['passed'] = record['completed'] and record['marker_returned'] and record['channel_header'] == channel
        assert record['passed'], record
        return result
    except urllib.error.HTTPError as error:
        record.update(http_status=error.code, error=error.read().decode(), passed=False)
        raise
    finally:
        (BASE / 'after-fix.json').write_text(json.dumps(RECORDS, indent=2))
        print(json.dumps(record), flush=True)


for channel in ['playground', 'build']:
    for stream in [False, True]:
        body = copy.deepcopy(ORIGINAL)
        body['stream'] = stream
        # No search tool is enabled: the old record must remain usable context.
        result = ask(channel, body, 'old-search-history')
        if channel == 'playground':
            switched = {'model': 'gemini-3.1-pro-preview', 'stream': stream, 'input': [{'role': 'user', 'content': 'Return the marker from the earlier search query only.'}]}
            if stream:
                switched['previous_response_id'] = result['id']
            else:
                switched['input'] = body['input'] + result['output'] + switched['input']
            ask(channel, switched, 'model-switch-' + ('stored' if stream else 'explicit'))
