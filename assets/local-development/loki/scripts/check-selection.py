#!/usr/bin/env python3
"""Static configuration/rule regression tests (not a Docker runtime test)."""
import copy, re
from pathlib import Path
import yaml
root=Path(__file__).resolve().parent.parent
pc=yaml.safe_load((root/'promtail/config.yml').read_text())
names=[j['job_name'].removesuffix('-host') for j in pc['scrape_configs'][:-1]]
assert len(names)==len(set(names))==8

def expanded(value,settings):
    return re.sub(r'\$\{([A-Z_]+):-([^}]+)\}',lambda m:settings.get(m[1],m[2]),value)
def apply(labels,rules,settings):
    labels=dict(labels)
    for rule in rules:
        action=rule.get('action','replace'); expression=rule.get('regex','(.*)')
        text=';'.join(labels.get(k,'') for k in rule.get('source_labels',[]))
        match=re.fullmatch(expression,text)
        if action=='keep' and not match: return None
        if action=='labeldrop': labels={k:v for k,v in labels.items() if not re.fullmatch(expression,k)}
        if action=='replace' and match:
            replacement=expanded(rule.get('replacement','$1'),settings)
            labels[rule['target_label']]=re.sub(r'\$(\d+)',lambda m:match[int(m[1])],replacement)
    return labels
rules=pc['scrape_configs'][-1]['relabel_configs']
for name,job in zip(names,pc['scrape_configs']):
    key=name.removesuffix('-service').upper().replace('-','_')+'_LOG_SOURCE'
    for source in ['host','docker']:
        settings={key:source}
        host=apply({k:expanded(v,settings) for k,v in job['static_configs'][0]['labels'].items()},job['relabel_configs'],settings)
        docker=apply({'__meta_docker_container_label_com_docker_compose_service':name},rules,settings)
        assert (host is not None)+(docker is not None)==1
        assert (host or docker)['service']==name
assert apply({'__meta_docker_container_label_com_docker_compose_service':'unrelated-service'},rules,{}) is None
filters=pc['scrape_configs'][-1]['docker_sd_configs'][0]['filters']
assert filters==[{'name':'label','values':['logging.parstis.enabled=true']}]
print('PASS 16 source-selection cases; all eight canonical labels; unrelated Docker service denied; explicit opt-in filter')
