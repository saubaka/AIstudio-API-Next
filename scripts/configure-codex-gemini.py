"""Add Gemini tool metadata to the installed Codex catalog, preserving native models.

No credentials are read or copied. Use --install to configure the user's local
catalog, or --output PATH for an isolated acceptance catalog.
"""
import argparse, copy, datetime, json, os, pathlib, re, sqlite3, subprocess, tempfile

parser=argparse.ArgumentParser()
parser.add_argument('--output',type=pathlib.Path)
parser.add_argument('--install',action='store_true')
parser.add_argument('--ccswitch',action='store_true',help='Preserve the custom catalog when CC Switch activates local AIStudio providers')
parser.add_argument('--model',action='append',default=[])
args=parser.parse_args()
if not args.output and not args.install:parser.error('Specify --output or --install')
with tempfile.TemporaryDirectory(prefix='aistudio-codex-catalog-') as temporary:
    raw=subprocess.check_output(['codex','debug','models'],env=dict(os.environ,CODEX_HOME=temporary),text=True)
catalog=json.loads(raw)
template=next(m for m in catalog['models'] if m['slug']=='gpt-5.5')
ids=args.model or ['gemini-3.8-flash','gemini-3.1-pro-preview','gemini-omni-flash-preview','gemini-flash-latest','gemini-pro-latest','gemini-2.5-flash','gemini-2.5-pro']
target=args.output or pathlib.Path.home()/'.codex/models-aistudio.json'
managed_path=pathlib.Path.home()/'.codex/cc-switch-model-catalog.json'
managed_models={}
if args.install and managed_path.exists():
    managed_models={row['slug']:row for row in json.loads(managed_path.read_text()).get('models',[])}
if args.install:
    config=pathlib.Path.home()/'.codex/config.toml'
    text=config.read_text() if config.exists() else ''
    top=text.split('\n[')[0]
    match=re.search(r'^model_catalog_json\s*=\s*"([^"]+)"',top,re.M)
    if match:
        prior_path=pathlib.Path(match.group(1)).expanduser()
        if not prior_path.is_absolute():prior_path=config.parent/prior_path
        prior=json.loads(prior_path.read_text())
        known={m['slug'] for m in prior['models']}
        prior['models'].extend(m for m in catalog['models'] if m['slug'] not in known)
        catalog=prior
for model in ids:
    if not re.fullmatch(r'gemini-[a-zA-Z0-9._-]+',model):raise SystemExit('Invalid Gemini model identifier')
    row=next((m for m in catalog['models'] if m['slug']==model),None)
    if row is None:
        row=copy.deepcopy(template);catalog['models'].append(row)
    row.update(slug=model,display_name=model,description='Gemini through local AIStudio2API',apply_patch_tool_type='freeform',shell_type='unified_exec',use_responses_lite=False,prefer_websockets=False,model_messages=None)
    # Keep the user's existing Gemini context limits rather than inheriting
    # the GPT template's smaller context window.
    for field in ['context_window','max_context_window','effective_context_window_percent','supports_experimental_context']:
        if field in managed_models.get(model,{}):row[field]=managed_models[model][field]
    row['base_instructions']=re.sub(r'You are Codex, an agent based on [^.]+\.', 'You are Codex using a Gemini model through AIStudio2API.',row.get('base_instructions',''),count=1)
target=target.expanduser().resolve();target.parent.mkdir(parents=True,exist_ok=True)
target.write_text(json.dumps(catalog,ensure_ascii=False,indent=2)+'\n')
if args.install:
    stamp=datetime.datetime.now().strftime('%Y%m%d-%H%M%S')
    if config.exists():
        backup=config.with_name('config.toml.before-aistudio-catalog-'+stamp)
        backup.write_bytes(config.read_bytes());backup.chmod(0o600)
    line='model_catalog_json = '+json.dumps(str(target),ensure_ascii=False)
    if match:text=re.sub(r'^model_catalog_json\s*=.*$',lambda _:line,text,count=1,flags=re.M)
    else:text=line+'\n'+text
    config.write_text(text);config.chmod(0o600)
updated=[]
if args.ccswitch:
    database=pathlib.Path.home()/'.cc-switch/cc-switch.db'
    if not database.exists():raise SystemExit('CC Switch database not found')
    connection=sqlite3.connect(database)
    candidates=[]
    for provider_id, raw in connection.execute("SELECT id,settings_config FROM providers WHERE app_type='codex'"):
        settings=json.loads(raw);provider_text=settings.get('config','')
        if not re.search(r'base_url\s*=\s*"http://(?:127\.0\.0\.1|localhost):2048/(?:playground|build)/v1/?"',provider_text):continue
        line='model_catalog_json = '+json.dumps(str(target),ensure_ascii=False)
        if re.search(r'^model_catalog_json\s*=',provider_text,re.M):provider_text=re.sub(r'^model_catalog_json\s*=.*$',lambda _:line,provider_text,count=1,flags=re.M)
        else:provider_text=line+'\n'+provider_text
        settings['config']=provider_text
        candidates.append((provider_id,raw,json.dumps(settings,ensure_ascii=False)))
    if candidates:
        backup=database.parent/'backups'/('before-aistudio-catalog-'+datetime.datetime.now().strftime('%Y%m%d-%H%M%S')+'.db')
        with sqlite3.connect(backup) as destination:connection.backup(destination)
        backup.chmod(0o600)
        with connection:
            for provider_id,original,replacement in candidates:
                cursor=connection.execute("UPDATE providers SET settings_config=? WHERE id=? AND app_type='codex' AND settings_config=?",(replacement,provider_id,original))
                if cursor.rowcount!=1:raise RuntimeError('Provider changed while applying catalog fix')
                updated.append(provider_id)
    connection.close()
print(json.dumps({'catalog':str(target),'gemini_models':ids,'native_models_preserved':True,'installed':args.install,'ccswitch_providers_updated':len(updated)}))
