#!/usr/bin/env python3
"""Append synthetic fixtures only; query a running Loki. No application requests."""
import json, os, time, uuid, urllib.request, urllib.parse
from pathlib import Path
from datetime import datetime, timezone
ROOT=Path(__file__).resolve().parent.parent
settings={}
envfile=Path(os.environ.get('ENV_FILE', ROOT/'.env'))
if envfile.exists():
    for line in envfile.read_text().splitlines():
        if line.strip() and not line.lstrip().startswith('#') and '=' in line:
            k,v=line.split('=',1); settings[k.strip()]=v.strip().strip('"\'')
def setting(k,default): return os.environ.get(k,settings.get(k,default))
services=['identity','merchant','location','bnpl','document','financial-core','customer','notification']
base=setting('LOKI_URL','http://127.0.0.1:3100').rstrip('/')
run=str(uuid.uuid4()); fixtures={}; paths=[]
def query(q):
    params=urllib.parse.urlencode(dict(query=q,start=time.time_ns()-600_000_000_000,end=time.time_ns(),limit=200,direction='FORWARD'))
    with urllib.request.urlopen(base+'/loki/api/v1/query_range?'+params,timeout=10) as r:
        return json.load(r)['data']['result']
def rows(q): return [(s['stream'],v[1]) for s in query(q) for v in s['values']]
try:
    for service in services:
        key=service.upper().replace('-','_'); name=service+'-service'
        if setting(key+'_LOG_SOURCE','host')!='host':
            raise SystemExit('Host verifier requires all eight *_LOG_SOURCE=host; Docker must be verified separately.')
        folder=Path(setting(key+'_LOG_DIR','./logs/'+name))
        if not folder.is_absolute(): folder=ROOT/folder
        if not folder.is_dir(): raise SystemExit('Missing log directory: '+str(folder))
        ts=datetime.now(timezone.utc).isoformat(timespec='milliseconds').replace('+00:00','Z')
        common=dict(timestamp=ts, level='INFO',service='ignored-untrusted-service',trace_id=run,span_id='span-'+run,subject_id='synthetic-subject',request_id='req-'+run,correlation_id='corr-'+run,thread='fixture')
        values=[dict(common,logger='fixture',message='json '+run),dict(common,logger='ir.parstis.common.http.access',message='http_access method=POST route=/api/v1/test/{id} status=201 duration_ms=12 request_id=req-'+run+' correlation_id=corr-'+run),dict(common,logger='not.access',message='http_access method=POST route=/api/v1/test/{id} status=201 duration_ms=12 request_id=req-'+run+' correlation_id=corr-'+run)]
        lines=[json.dumps(v,separators=(',',':')) for v in values]+['plain '+run,json.dumps(dict(level='WARN',message='missing-ts '+run,trace_id=run),separators=(',',':'))]
        path=folder/('loki-smoke.'+run+'.log'); path.write_text('\n'.join(lines)+'\n'); paths.append(path); fixtures[name]=lines
    deadline=time.monotonic()+90
    while True:
        counts={name:len(rows('{service="'+name+'"} |= "'+run+'"')) for name in fixtures}
        if all(n>=5 for n in counts.values()): break
        if time.monotonic()>deadline: raise AssertionError(counts)
        time.sleep(1)
    for name,expected in fixtures.items():
        selector='{service="'+name+'"}'
        actual=rows(selector+' |= "'+run+'"')
        assert sorted(v for _,v in actual)==sorted(expected),(name,actual)
        assert len(rows(selector+' | trace_id="'+run+'"'))==4,name
        access=rows(selector+' | trace_id="'+run+'" | logger="ir.parstis.common.http.access"')
        assert len(access)==1 and access[0][0]['http_path']=='/api/v1/test/{id}',access
        assert access[0][0]['span_id']=='span-'+run and access[0][0]['subject_id']=='synthetic-subject'
        nonaccess=rows(selector+' | trace_id="'+run+'" | logger="not.access"')
        assert len(nonaccess)==1 and not nonaccess[0][0].get('http_path'),nonaccess
        assert not rows('{service="'+name+'",trace_id="'+run+'"}'),'trace indexed'
        assert not rows('{service="'+name+'",subject_id="synthetic-subject"}'),'subject indexed'
        print('PASS',name,'5 raw lines, 4 trace matches, route/span/subject metadata; no indexed IDs')
    print('VERIFY_ALL_8_PASS run='+run)
finally:
    if setting('VERIFY_LOKI_KEEP','0')=='0':
        for path in paths: path.unlink(missing_ok=True)
