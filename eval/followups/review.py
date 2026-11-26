"""Run isolated scenario reviews against a locally hosted OpenAI-compatible model."""
import argparse
import json
import time
from pathlib import Path
from urllib.parse import urlparse
from urllib.request import Request, ProxyHandler, build_opener

ROOT = Path(__file__).resolve().parent
TOOLS = ('record', 'counterexample', 'metric')
SYSTEM = ('Ты работаешь рецензентом сценария для HR МГТС. Все примеры учебные. '
          'Не выдумывай новые факты или возможности продукта. Учитывай только возвращённые сведения. '
          'Пиши по-русски. Возвращай один объект JSON.')


def completion(opener, base_url, model, messages, tokens):
    body = json.dumps({'model': model, 'messages': messages, 'temperature': 0,
                       'response_format': {'type': 'json_object'}, 'max_tokens': tokens}).encode()
    req = Request(base_url.rstrip('/')+'/chat/completions', body, {'Content-Type':'application/json'})
    with opener.open(req, timeout=90) as response:
        return json.loads(json.load(response)['choices'][0]['message']['content'])


def run_review(opener, args, role, description, case):
    messages = [{'role':'system','content':SYSTEM}, {'role':'user','content':
        f'{description}\nСитуация: {case["situation"]}\nВыбери два источника для проверки. '
        'record — запись случая; counterexample — альтернативное объяснение; metric — условие подсчёта. '
        'Верни {"read":["код1","код2"]}.'}]
    picked = completion(opener,args.base_url,args.model,messages,100).get('read')
    if not isinstance(picked,list) or len(picked) != 2 or not all(key in TOOLS for key in picked) or len(set(picked)) != 2:
        raise ValueError('invalid tool selection')
    messages += [{'role':'assistant','content':json.dumps({'read':picked})},
                 {'role':'user','content': 'Сведения: '+json.dumps({k:case[k] for k in picked},ensure_ascii=False)
                  +'\nДай одно следующее действие HR и один способ проверить его корректность. '
                   'Укажи из прочитанного один код основания. Формат {"action":"...","check":"...","basis":"код"}. '
                   'Каждый ответ — одно короткое содержательное предложение.'}]
    result=completion(opener,args.base_url,args.model,messages,260)
    if result.get('basis') not in picked or not all(isinstance(result.get(k),str) and result[k].strip() for k in ('action','check')):
        raise ValueError('invalid review')
    return {'role':role,'case':case['id'],'read':picked,'basis':result['basis'],
            'action':result['action'],'check':result['check']}


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base-url',default='http://127.0.0.1:19176/v1')
    parser.add_argument('--model',default='Qwen2.5-7B-Instruct-Q4_K_M')
    parser.add_argument('--out',type=Path,required=True)
    args=parser.parse_args()
    if urlparse(args.base_url).hostname not in ('localhost','127.0.0.1','::1'):
        parser.error('Use a loopback endpoint')
    spec=json.loads((ROOT/'scenarios.json').read_text('utf8'))
    opener=build_opener(ProxyHandler({}))
    results=[]
    for case in spec['cases']:
        for role, description in spec['roles'].items():
            for attempt in range(3):
                try:
                    row=run_review(opener,args,role,description,case)
                    break
                except (ValueError, KeyError, TypeError, OSError) as exc:
                    if attempt==2: raise RuntimeError(f'{case["id"]}/{role}: {exc}') from exc
                    time.sleep(0.5)
            results.append(row)
            tmp=args.out.with_suffix('.tmp')
            tmp.write_text(json.dumps({'model':args.model,'reviews':results},ensure_ascii=False,indent=2),encoding='utf8')
            tmp.replace(args.out)
            print(f'{len(results)} {case["id"]} {role}',flush=True)


if __name__=='__main__':
    main()
