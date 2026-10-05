import json,os,re,subprocess,threading,urllib.request,urllib.error,time,sys,datetime
from pathlib import Path
from http.server import ThreadingHTTPServer,BaseHTTPRequestHandler
root=Path(__file__).resolve().parents[1]
base=root/"runtime"/os.environ.get('QA_SUITE','codex-qa')
base.mkdir(parents=True,exist_ok=True)
key=''
for line in (root/'.env').read_text().splitlines():
 if line.startswith('PROXY_API_KEY='):key=line.split('=',1)[1].strip().strip('\"\'')
channel=os.environ.get('QA_CHANNEL','playground')
records=[]
client_model=os.environ.get('QA_CLIENT_MODEL','gpt-5.5')
target_model=os.environ.get('QA_TARGET_MODEL','gemini-flash-latest')
schema_probe=os.environ.get('QA_SCHEMA_PROBE')=='1'
typed_probe=os.environ.get('QA_TYPED_ENUM_PROBE')=='1'
web_probe=os.environ.get('QA_WEB_SEARCH_PROBE')=='1'
switch_model=os.environ.get('QA_SWITCH_AFTER_SEARCH_MODEL','')
resume_search_path=os.environ.get('QA_RESUME_FROM_SEARCH_RESPONSE','')
agent_workflow=os.environ.get('QA_AGENT_WORKFLOW')=='1'
model_catalog=os.environ.get('QA_MODEL_CATALOG','')
if typed_probe and not schema_probe:raise SystemExit('QA_TYPED_ENUM_PROBE requires QA_SCHEMA_PROBE=1')
probe_only=os.environ.get('QA_SCHEMA_PROBE_ONLY')=='1'
if probe_only and not schema_probe:raise SystemExit('QA_SCHEMA_PROBE_ONLY requires QA_SCHEMA_PROBE=1')
suffix='-direct' if client_model!='gpt-5.5' else ''
class Handler(BaseHTTPRequestHandler):
 def log_message(self,*args):pass
 def do_POST(self):
  payload=json.loads(self.rfile.read(int(self.headers['Content-Length'])))
  original_model=payload.get('model')
  # Run the installed client's actual GPT tool catalog against the requested
  # Gemini model. No tools, schemas, flags or history items are removed.
  web_history=any(isinstance(item,dict) and item.get('type')=='web_search_call' for item in payload.get('input',[]))
  payload['model']=switch_model if web_history and switch_model else target_model
  catalog=list(payload.get('tools',[]))
  for item in payload.get('input',[]):
   if isinstance(item,dict) and item.get('type')=='tool_search_output':catalog.extend(item.get('tools',[]))
  def has_enum(value):
   if isinstance(value,dict):return any(isinstance(v,(bool,int,float)) for v in value.get('enum',[])) or any(has_enum(v) for v in value.values())
   if isinstance(value,list):return any(has_enum(v) for v in value)
   return False
  rec={'input_types':[x.get('type') for x in payload.get('input',[]) if isinstance(x,dict)],'tools':[{'type':t.get('type'),'name':t.get('name')} for t in payload.get('tools',[])],'path':self.path,'calls':[],'has_schema_refs':'"$defs"' in json.dumps(catalog),'has_typed_enum':has_enum(catalog),'has_web_search_history':web_history,'target_model':payload['model']}
  rec['client_model']=original_model
  records.append(rec)
  (base/'actual-codex-tools.json').write_text(json.dumps(catalog,indent=2))
  req=urllib.request.Request('http://127.0.0.1:2048/'+channel+self.path,data=json.dumps(payload).encode(),headers={'Content-Type':'application/json','Authorization':'Bearer '+key})
  try:r=urllib.request.urlopen(req,timeout=180)
  except urllib.error.HTTPError as e:
   data=e.read();rec.update(status=e.code,error=data.decode()[:1000]);self.send_response(e.code);self.send_header('Content-Type','application/json');self.end_headers();self.wfile.write(data);return
  rec['status']=r.status;self.send_response(r.status);self.send_header('Content-Type',r.headers.get('Content-Type','application/json'));self.end_headers()
  raw=b''
  try:
   while True:
    chunk=r.read1(65536)
    if not chunk:break
    raw+=chunk;self.wfile.write(chunk);self.wfile.flush()
  except BrokenPipeError:pass
  for line in raw.decode(errors='replace').splitlines():
   if not line.startswith('data: '):continue
   try:event=json.loads(line[6:])
   except ValueError:continue
   if event.get('type')=='response.completed':rec['completed']=True;rec['usage']=event.get('response',{}).get('usage')
   if event.get('type')=='response.failed':rec['stream_error']=event.get('response',{}).get('error')
   if event.get('type')=='response.output_item.done':
    item=event.get('item',{})
    if item.get('type') in ['function_call','custom_tool_call','tool_search_call','web_search_call']:
     call={'type':item.get('type'),'name':item.get('name'),'namespace':item.get('namespace')}
     if agent_workflow and item.get('name') in ['exec_command','shell_command']:
      try:call['command']=json.loads(item.get('arguments','{}')).get('cmd',json.loads(item.get('arguments','{}')).get('command',''))
      except ValueError:call['command']='<invalid>'
     rec['calls'].append(call)
server=ThreadingHTTPServer(('127.0.0.1',0),Handler);threading.Thread(target=server.serve_forever,daemon=True).start()
home=base/('home-'+channel+suffix);home.mkdir(exist_ok=True);work=base/('agent-'+channel+suffix);work.mkdir(exist_ok=True)
(work/'verified.txt').unlink(missing_ok=True)
(work/'seed.txt').write_text('CODEX_TOOL_ACCEPTANCE_42\n')
(home/'config.toml').write_text(('model = "'+client_model+'"\nmodel_provider = "aistudio"\n[model_providers.aistudio]\nname = "AIStudio local QA"\nbase_url = "http://127.0.0.1:%s/v1"\nwire_api = "responses"\nenv_key = "AISTUDIO_QA_KEY"\nrequest_max_retries = 0\nstream_max_retries = 0\n'%server.server_port))
env=dict(os.environ,CODEX_HOME=str(home),AISTUDIO_QA_KEY=key)
if model_catalog:
 with (home/'config.toml').open('a') as config:config.write('\n')
 # Insert the catalog as a top-level setting, before the provider table.
 path=home/'config.toml';path.write_text('model_catalog_json = '+json.dumps(str(Path(model_catalog).resolve()))+'\n'+path.read_text())
probe_evidence=base/(channel+'-mcp-probe.json')
if schema_probe:
 probe_evidence.unlink(missing_ok=True)
 with (home/'config.toml').open('a') as config:
  probe_args=[str(root/'scripts/schema-probe-mcp.py'),str(probe_evidence)]
  if typed_probe:probe_args.append('typed-enum')
  config.write('\n[mcp_servers.schema_probe]\ncommand = '+json.dumps(sys.executable)+'\nargs = '+json.dumps(probe_args)+'\n')
  # This fixture only records a marker in the disposable QA directory. Trust
  # that single test tool within the isolated home; user policy is unchanged.
  config.write('enabled_tools = ["record_reference_probe"]\n[mcp_servers.schema_probe.tools.record_reference_probe]\napproval_mode = "approve"\n')
prompt='This is a local API compatibility test. Use exec_command to read seed.txt. Then use the FREEFORM apply_patch tool to add verified.txt containing exactly the seed value. Run exec_command to check that both files match. Do not create goals, use agents, send network requests, or touch files outside this working directory. End with CODEX_QA_PASS only after the actual tools and verification succeed.'
if suffix:prompt='Use exec_command to read seed.txt, copy it to verified.txt, then compare the files with cmp. Do not use agents, goals or network requests. Reply CODEX_QA_PASS after actual tool verification succeeds.'
if agent_workflow:
 (work/'AGENTS.md').write_text('Use rg for file discovery and scoped reads before edits. Use the dedicated apply_patch tool for text-file changes. Do not write files through shell heredocs or Python. Preserve unrelated files. Do not access the network, use agents or create goals.\n')
 (work/'editme.txt').write_text('KEEP_BEFORE\nOLD_VALUE\nKEEP_AFTER\n')
 # Keep Git verification scoped to the fixture; the parent's runtime/ ignore
 # otherwise makes diff/status inspect unrelated project work and loop.
 subprocess.run(['git','init','-q',str(work)],check=True,capture_output=True)
 subprocess.run(['git','-C',str(work),'add','AGENTS.md','seed.txt','editme.txt'],check=True,capture_output=True)
 prompt='Follow the repository instructions. Read seed.txt and editme.txt. Add verified.txt containing exactly the seed file contents, and change only OLD_VALUE to NEW_VALUE in editme.txt. Verify both changes with actual local commands. Reply CODEX_QA_PASS only after verification succeeds.'
if schema_probe:prompt='First discover and call schema_probe.record_reference_probe with points containing exactly one object, line 17 and label SCHEMA_REF_OK_17. Wait for the actual MCP result, which must be SCHEMA_REF_OK_17. Then '+prompt
if schema_probe and probe_only:prompt='Discover and call schema_probe.record_reference_probe with points containing exactly one object, line 17 and label SCHEMA_REF_OK_17. Wait for the actual MCP result, then reply SCHEMA_REF_OK_17 CODEX_QA_PASS. Do not call exec_command, create goals, use agents, read files or make other tool calls.'
if typed_probe:
 expected_args={'points':[{'line':22,'label':'ENUM_TYPED_OK_22','enabled':True}],'allowAsync':False,'allowAsyncTrue':True,'attempts':22,'ratio':1.25}
 task='Use exec_command to read seed.txt, then use the FREEFORM apply_patch tool to add verified.txt with exactly the seed value. Use exec_command to verify byte-for-byte equality with cmp. '
 if probe_only:task=''
 prompt='Discover and call schema_probe.record_reference_probe with these exact JSON arguments: '+json.dumps(expected_args)+'. Wait for its actual ENUM_TYPED_OK_22 result. '+task+'Reply ENUM_TYPED_OK_22 CODEX_QA_PASS after actual verification. Do not create goals, use agents, access the network or read files outside this test directory.'
if web_probe and not resume_search_path:
 prompt='Before invoking local tools, use the built-in web search to find the official OpenAI Responses API documentation for web_search_call. Wait for the actual search result, then perform the following local test completely: '+prompt.replace('send network requests','send network requests from the shell').replace('access the network','access the network from the shell').replace('use agents, goals or network requests','use agents, goals or shell network requests')
success=False
try:
 command=['codex','exec','--skip-git-repo-check','--ephemeral','-C',str(work),'-s','workspace-write','--json',prompt]
 resume_provenance=None
 if resume_search_path:
  # A disposable native session receives real hosted output from the local
  # service. The installed Codex parser, not this HTTP relay, reconstructs the
  # old history on resume. User desktop sessions/configuration are untouched.
  search_response=json.loads(Path(resume_search_path).read_text())
  search_items=search_response['output']
  assert search_response.get('status')=='completed' and any(x.get('type')=='web_search_call' for x in search_items)
  bootstrap=subprocess.run(['codex','exec','--skip-git-repo-check','-C',str(work),'-s','workspace-write','--json','Reply OLD_MODEL_HISTORY_READY only. Do not use any tools.'],env=env,capture_output=True,text=True,timeout=90)
  assert bootstrap.returncode==0, 'Native session bootstrap failed'
  events=[json.loads(line) for line in bootstrap.stdout.splitlines() if line.startswith('{')]
  thread_id=next(x['thread_id'] for x in events if x.get('type')=='thread.started')
  rollouts=list(home.glob('sessions/**/rollout-*-'+thread_id+'.jsonl'))
  assert len(rollouts)==1, 'Native session rollout not found'
  existing=[json.loads(line) for line in rollouts[0].read_text().splitlines()]
  ordinal=max(x.get('ordinal',-1) for x in existing)+1
  with rollouts[0].open('a') as rollout:
   for item in search_items:
    rollout.write(json.dumps({'timestamp':datetime.datetime.now(datetime.timezone.utc).isoformat(),'ordinal':ordinal,'type':'response_item','payload':item})+'\n')
    ordinal+=1
  resume_provenance={'kind':'real hosted output appended to an isolated native session fixture','source_model':search_response.get('model'),'search_call_count':sum(x.get('type')=='web_search_call' for x in search_items),'native_session_id':thread_id}
  command=['codex','exec','resume','--skip-git-repo-check','--ephemeral','--json','-c','sandbox_mode="workspace-write"',thread_id,prompt]
 r=subprocess.run(command,env=env,cwd=str(work),capture_output=True,text=True,timeout=220)
 # QA workspace and prompts contain no user data; replace key defensively.
 out=(r.stdout+'\n'+r.stderr).replace(key,'<redacted>') if key else r.stdout+'\n'+r.stderr
 (base/(channel+suffix+'-codex-output.txt')).write_text(out)
 file_ok=probe_only or ((work/'verified.txt').exists() and (work/'verified.txt').read_text().strip()=='CODEX_TOOL_ACCEPTANCE_42')
 success=r.returncode==0 and bool(records) and all(x.get('status')==200 and x.get('completed') and not x.get('stream_error') for x in records) and file_ok and 'CODEX_QA_PASS' in r.stdout
 if schema_probe:success=success and probe_evidence.exists() and json.loads(probe_evidence.read_text()).get('passed') and any(x['has_schema_refs'] for x in records)
 if typed_probe:
  success=success and any(x['has_typed_enum'] for x in records)
  if not probe_only:success=success and any(c.get('type')=='custom_tool_call' and c.get('name')=='apply_patch' for x in records for c in x['calls'])
 if agent_workflow:
  calls=[c for x in records for c in x['calls']]
  success=success and any(c.get('type')=='custom_tool_call' and c.get('name')=='apply_patch' for c in calls)
  success=success and not any('<<' in c.get('command','') for c in calls)
  patch_index=next((i for i,c in enumerate(calls) if c.get('type')=='custom_tool_call' and c.get('name')=='apply_patch'),-1)
  success=success and any('seed.txt' in c.get('command','') and 'editme.txt' in c.get('command','') for c in calls[:patch_index])
  success=success and all(x.get('client_model')==target_model for x in records)
  success=success and not any(re.search(r'(?:cat|printf|echo|tee)\b[^\n]*>\s*[^&]|\.write(?:_text|_bytes)?\(',c.get('command','')) for c in calls)
  success=success and (work/'verified.txt').read_bytes()==(work/'seed.txt').read_bytes() and (work/'editme.txt').read_text()=='KEEP_BEFORE\nNEW_VALUE\nKEEP_AFTER\n'
 if web_probe:
  success=success and (bool(resume_provenance) or any(c.get('type')=='web_search_call' for x in records for c in x['calls'])) and any(x['has_web_search_history'] for x in records)
  if switch_model:success=success and any(x['has_web_search_history'] and x['target_model']==switch_model for x in records)
 evidence={'channel':channel,'target_model':target_model,'cli_version':subprocess.check_output(['codex','--version'],text=True).strip(),'exit_code':r.returncode,'passed':bool(success),'schema_probe':schema_probe,'typed_enum_probe':typed_probe,'schema_probe_only':probe_only,'web_search_probe':web_probe,'switch_after_search_model':switch_model,'resume_provenance':resume_provenance,'requests':records}
 evidence['agent_workflow']=agent_workflow;evidence['actual_client_model']=client_model;evidence['model_catalog']=bool(model_catalog)
 (base/(channel+suffix+'-codex.json')).write_text(json.dumps(evidence,indent=2));print(json.dumps(evidence));print(out[-2200:])
except subprocess.TimeoutExpired:
 print('Codex test timed out');(base/(channel+suffix+'-codex.json')).write_text(json.dumps({'channel':channel,'passed':False,'timeout':True,'requests':records},indent=2))
finally:server.shutdown()
if not success:raise SystemExit(1)
