"""Verify cached-input counters in the installed Codex client and native session."""
import json, os, pathlib, subprocess

root = pathlib.Path(__file__).resolve().parents[1]
base = root / 'runtime/account-agent-cache-qa/native-cache'
base.mkdir(parents=True, exist_ok=True)
key = next(line.split('=', 1)[1].strip().strip('\"\'') for line in (root / '.env').read_text().splitlines() if line.startswith('PROXY_API_KEY='))
catalog = pathlib.Path.home() / '.codex/models-aistudio.json'
prefix = '\n'.join('Stable synthetic Codex cache record %04d: no personal data, no tools, no files to modify; respond with the requested marker.' % i for i in range(600))
prompt = 'This is a cache-usage acceptance test. Do not call tools or change files. The following records are context only.\n' + prefix + '\nReply NATIVE_CACHE_OK only.'
records = []
for channel in ['playground', 'build']:
    home = base / ('home-' + channel)
    work = base / ('work-' + channel)
    home.mkdir(exist_ok=True); work.mkdir(exist_ok=True)
    config = 'model = "gemini-3.8-flash"\nmodel_catalog_json = ' + json.dumps(str(catalog)) + '\nmodel_provider = "aistudio"\n[model_providers.aistudio]\nname = "AIStudio native cache QA"\nbase_url = "http://127.0.0.1:2048/' + channel + '/v1"\nwire_api = "responses"\nenv_key = "AISTUDIO_QA_KEY"\nrequest_max_retries = 0\nstream_max_retries = 0\n'
    (home / 'config.toml').write_text(config)
    for attempt in range(4):
        result = subprocess.run(['codex', 'exec', '--skip-git-repo-check', '-C', str(work), '-s', 'read-only', '--json', prompt], env=dict(os.environ, CODEX_HOME=str(home), AISTUDIO_QA_KEY=key), capture_output=True, text=True, timeout=120)
        events = [json.loads(line) for line in result.stdout.splitlines() if line.startswith('{')]
        usage = next((e.get('usage') for e in events if e.get('type') == 'turn.completed'), None)
        record = {'channel': channel, 'attempt': attempt + 1, 'exit_code': result.returncode, 'usage': usage, 'marker': 'NATIVE_CACHE_OK' in result.stdout}
        if result.returncode != 0:record['errors'] = [e for e in events if e.get('type') in ['error','turn.failed']]
        (base / (channel + '-attempt-' + str(attempt + 1) + '.jsonl')).write_text(result.stdout.replace(key, '<redacted>'))
        records.append(record);print(json.dumps(record), flush=True)
        if result.returncode != 0 or (usage and usage.get('cached_input_tokens', 0) > 0):break
    token_events = []
    for path in home.glob('sessions/**/*.jsonl'):
        for line in path.read_text().splitlines():
            row = json.loads(line)
            payload = row.get('payload', {})
            if row.get('type') == 'event_msg' and payload.get('type') == 'token_count':
                info = payload.get('info') or {}
                token_events.append({'total_token_usage': info.get('total_token_usage'), 'last_token_usage': info.get('last_token_usage')})
    (base / (channel + '-session-token-events.json')).write_text(json.dumps(token_events, indent=2) + '\n')
evidence = {'cli_version': subprocess.check_output(['codex','--version'],text=True).strip(), 'catalog': str(catalog), 'records': records}
(base / 'acceptance.json').write_text(json.dumps(evidence, indent=2) + '\n')
if not all(any(r['channel'] == channel and r['exit_code'] == 0 and r.get('usage', {}).get('cached_input_tokens', 0) > 0 for r in records) for channel in ['playground','build']):raise SystemExit('No positive native cache counter for one or more channels')
