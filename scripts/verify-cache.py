"""Bounded live implicit-cache probe; writes only usage and synthetic fixture IDs."""
import json, pathlib, urllib.error, urllib.request, time

root = pathlib.Path(__file__).resolve().parents[1]
key = next(line.split('=', 1)[1].strip().strip('\"\'') for line in (root / '.env').read_text().splitlines() if line.startswith('PROXY_API_KEY='))
prefix = '\n'.join('Fixture record %04d: stable public cache acceptance data, preserve sequence and return the requested marker without summarizing these records.' % i for i in range(600))
records = []
for channel in ['playground', 'build']:
    for attempt in range(4):
        body = {'model': 'gemini-3.8-flash', 'instructions': 'This is a synthetic cache acceptance test. Reply CACHE_QA_OK only. Do not use tools.\n' + prefix, 'input': 'Reply CACHE_QA_OK only.', 'max_output_tokens': 256}
        request = urllib.request.Request('http://127.0.0.1:2048/' + channel + '/v1/responses', data=json.dumps(body).encode(), headers={'Authorization': 'Bearer ' + key, 'Content-Type': 'application/json'})
        try:
            response = json.load(urllib.request.urlopen(request, timeout=90))
            record = {'channel': channel, 'attempt': attempt + 1, 'id': response.get('id'), 'status': response.get('status'), 'usage': response.get('usage'), 'marker': 'CACHE_QA_OK' in json.dumps(response.get('output'))}
        except urllib.error.HTTPError as error:
            record = {'channel': channel, 'attempt': attempt + 1, 'http_status': error.code, 'error': json.loads(error.read())}
        records.append(record)
        print(json.dumps(record), flush=True)
        if record.get('usage', {}).get('input_tokens_details', {}).get('cached_tokens', 0) > 0 or 'error' in record:
            break
        time.sleep(2)
path = root / 'runtime/account-agent-cache-qa/cache-live.json'
path.write_text(json.dumps({'synthetic_prefix_chars': len(prefix), 'records': records}, indent=2) + '\n')
