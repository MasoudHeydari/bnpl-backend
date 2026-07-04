#!/usr/bin/env python3
"""Requires PyYAML + local Loki/Promtail 3.6.8 binaries. Isolated host-file test."""
import os, subprocess, tempfile, time, urllib.request, socket
from pathlib import Path
import yaml
root=Path(__file__).resolve().parent.parent
loki=os.environ['LOKI_BIN']; promtail=os.environ['PROMTAIL_BIN']
def port():
    with socket.socket() as s: s.bind(('127.0.0.1',0)); return s.getsockname()[1]
with tempfile.TemporaryDirectory(prefix='loki-eight-') as tmp:
    work=Path(tmp); http,grpc,metrics=port(),port(),port()
    lc=yaml.safe_load((root/'loki/local-config.yaml').read_text())
    def replace(x):
        if isinstance(x,dict): return {k:replace(v) for k,v in x.items()}
        if isinstance(x,list): return [replace(v) for v in x]
        return x.replace('/loki',str(work/'data')) if isinstance(x,str) else x
    lc=replace(lc); lc['server'].update(http_listen_address='127.0.0.1',http_listen_port=http,grpc_listen_port=grpc)
    (work/'loki.yml').write_text(yaml.safe_dump(lc))
    pc=yaml.safe_load((root/'promtail/config.yml').read_text()); pc['server'].update(http_listen_address='127.0.0.1',http_listen_port=metrics)
    pc['positions']['filename']=str(work/'positions.yml'); pc['clients'][0]['url']=f'http://127.0.0.1:{http}/loki/api/v1/push'
    # No Docker daemon on the test box: host-file runtime only.
    pc['scrape_configs']=pc['scrape_configs'][:-1]; env=dict(os.environ)
    for job in pc['scrape_configs']:
        name=job['job_name'].removesuffix('-host'); folder=work/name; folder.mkdir()
        key=name.removesuffix('-service').upper().replace('-','_')
        env[key+'_LOG_DIR']=str(folder); env[key+'_LOG_SOURCE']='host'
        for static in job['static_configs']:
            static['labels']['__path__']=static['labels']['__path__'].replace('/host-logs/'+name,str(folder))
    (work/'promtail.yml').write_text(yaml.safe_dump(pc,sort_keys=False))
    subprocess.run([loki,'-config.file='+str(work/'loki.yml'),'-verify-config=true'],check=True)
    procs=[]; files=[]
    try:
        for binary,cfg in [(loki,'loki.yml'),(promtail,'promtail.yml')]:
            out=open(work/(cfg+'.stdout'),'w'); files.append(out)
            procs.append(subprocess.Popen([binary,'-config.file='+str(work/cfg),*(['-config.expand-env=true'] if binary==promtail else [])],stdout=out,stderr=subprocess.STDOUT,env=env))
        base=f'http://127.0.0.1:{http}'
        for _ in range(100):
            if any(p.poll() is not None for p in procs): raise RuntimeError('native process exited')
            try:
                with urllib.request.urlopen(base+'/ready',timeout=1) as r:
                    if r.status==200: break
            except Exception: pass
            time.sleep(.5)
        else: raise RuntimeError('Loki readiness timeout')
        env['LOKI_URL']=base
        subprocess.run(['python3',str(root/'scripts/verify-loki.py')],env=env,check=True)
        print('NATIVE_ALL_8_PASS (host pipeline; Docker/Grafana/Compose runtime not exercised)')
    finally:
        for proc in reversed(procs):
            proc.terminate()
            try: proc.wait(timeout=15)
            except subprocess.TimeoutExpired: proc.kill(); proc.wait()
        for f in files: f.close()
        for cfg in ['loki.yml','promtail.yml']:
            text=(work/(cfg+'.stdout')).read_text()
            errors=[line for line in text.splitlines() if 'level=error' in line]
            if errors:
                from collections import Counter
                print(cfg, 'non-clean runtime log summary:', dict(Counter(line.split('msg=',1)[-1].split(' err',1)[0] for line in errors)))
