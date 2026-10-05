import json,urllib.request
from pathlib import Path
root=Path(__file__).resolve().parents[1]
base=root/"runtime"/"codex-qa"
base.mkdir(parents=True,exist_ok=True)
key=''
for line in (root/'.env').read_text().splitlines():
 if line.startswith('PROXY_API_KEY='):key=line.split('=',1)[1].strip().strip('\"\'')
results=[]
def ask(channel,tools,history):
 req=urllib.request.Request('http://127.0.0.1:2048/'+channel+'/v1/responses',data=json.dumps({'model':'gemini-flash-latest','tools':tools,'input':history}).encode(),headers={'Content-Type':'application/json','Authorization':'Bearer '+key})
 with urllib.request.urlopen(req,timeout=120) as r:return json.load(r)
for channel in ['playground','build']:
 tools=[{'type':'tool_search','execution':'client','description':'Discover audit tools before calling them.','parameters':{'type':'object','properties':{'query':{'type':'string'}},'required':['query'],'additionalProperties':False}}]
 history=[{'role':'user','content':'Call tool_search now with query audit to discover a tool. Do not answer in prose.'}]
 response=ask(channel,tools,history);call=next(x for x in response['output'] if x['type']=='tool_search_call');assert call['execution']=='client'
 loaded=[{'type':'namespace','name':'audit','tools':[{'type':'function','name':'echo','description':'Returns the supplied proof marker.','parameters':{'type':'object','properties':{'proof':{'type':'string'}},'required':['proof'],'additionalProperties':False},'strict':True}]}]
 history+=response['output']+[{'type':'tool_search_output','execution':'client','status':'completed','call_id':call['call_id'],'tools':loaded},{'role':'user','content':'Call audit.echo with proof DISCOVERY_OK_91, then stop and wait for the result.'}]
 response=ask(channel,tools,history);function=next(x for x in response['output'] if x['type']=='function_call');assert function['namespace']=='audit' and function['name']=='echo';assert json.loads(function['arguments'])['proof']=='DISCOVERY_OK_91'
 history+=response['output']+[{'type':'function_call_output','call_id':function['call_id'],'output':'DISCOVERY_OK_91'},{'role':'user','content':'Reply with the proof returned by the tool only. Do not call any more tools.'}]
 response=ask(channel,tools,history);text=''.join(p.get('text','') for x in response.get('output',[]) for p in x.get('content',[]));assert 'DISCOVERY_OK_91' in text
 record={'channel':channel,'tool_search':True,'dynamic_namespace':True,'strict_function_accepted':True,'history_result_return':True,'reply':text};results.append(record);(base/'discovery-live.json').write_text(json.dumps(results,indent=2));print(json.dumps(record),flush=True)
