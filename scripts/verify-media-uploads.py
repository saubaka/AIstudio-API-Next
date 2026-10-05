import json,urllib.request,urllib.error,hashlib
from pathlib import Path
root=Path(__file__).resolve().parents[1]
base=root/"runtime"/"codex-qa"
base.mkdir(parents=True,exist_ok=True)
key=''
for line in (root/'.env').read_text().splitlines():
 if line.startswith('PROXY_API_KEY='):key=line.split('=',1)[1].strip().strip('\"\'')
(base/'media'/'sample.txt').write_text('The acceptance marker is UPLOAD_FILE_ACCEPTANCE_73.')
def call(channel,path,data=None,content_type='application/json',method=None):
 req=urllib.request.Request('http://127.0.0.1:2048/'+channel+'/v1/'+path,data=data,method=method,headers={'Authorization':'Bearer '+key,'Content-Type':content_type})
 with urllib.request.urlopen(req,timeout=180) as r:return r.status,r.read()
records=[]
cases=[('images','red.png','image/png','input_image','What is the main color in this image? Answer with the color only.','red'),('audio','spoken.wav','audio/wav','input_audio','Transcribe this recording exactly.','blue sky'),('videos','blue.mp4','video/mp4','input_video','What is the main color in this video? Answer with the color only.','blue'),('files','sample.txt','text/plain','input_file','Return the acceptance marker from the attached file, with no other words.','UPLOAD_FILE_ACCEPTANCE_73'),('files','marker.pdf','application/pdf','input_file','Read this PDF and return its acceptance marker only.','PDF_FILE_ACCEPTANCE_19')]
for channel in ['playground','build']:
 for kind,name,mime,content_type,prompt,expected in cases:
  data=(base/'media'/name).read_bytes();boundary='qa-upload-boundary'
  body=('--'+boundary+'\r\nContent-Disposition: form-data; name="purpose"\r\n\r\nuser_data\r\n--'+boundary+'\r\nContent-Disposition: form-data; name="file"; filename="'+name+'"\r\nContent-Type: '+mime+'\r\n\r\n').encode()+data+('\r\n--'+boundary+'--\r\n').encode()
  status,raw=call(channel,'uploads/'+kind,body,'multipart/form-data; boundary='+boundary);file=json.loads(raw);fid=file['id'];record={'channel':channel,'kind':kind,'upload_status':status,'bytes':len(data)}
  try:
   status,received=call(channel,'files/'+fid+'/content');assert received==data;record['download_hash_matches']=True
   status,metadata=call(channel,'files/'+fid);assert json.loads(metadata)['bytes']==len(data);record['metadata_status']=status
   request={'model':'gemini-flash-latest','input':[{'role':'user','content':[{'type':'input_text','text':prompt},{'type':content_type,'file_id':fid}]}]}
   status,raw=call(channel,'responses',json.dumps(request).encode());response=json.loads(raw);text=''.join(p.get('text','') for x in response.get('output',[]) for p in x.get('content',[]))
   record.update(generation_status=status,reply=text,passed=expected.lower() in text.lower())
  except Exception as e:
   record.update(passed=False,error=str(e))
   if isinstance(e,urllib.error.HTTPError):record['response_error']=e.read().decode()[:1000]
  finally:
   status,_=call(channel,'files/'+fid,method='DELETE');record['delete_status']=status
  records.append(record);(base/'uploads-live.json').write_text(json.dumps(records,indent=2));print(json.dumps(record),flush=True)
assert all(r['passed'] for r in records),'One or more real media assertions failed'
print('PASSED: 10 upload/download/model-input roundtrips')
