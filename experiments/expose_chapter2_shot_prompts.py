"""Expose literal Timeline segment prompts as editable canvas shot variables."""
import copy
import json
import logging
from pathlib import Path
import requests

BASE = 'http://127.0.0.1:16789'
CID = 'cv_3fef3e02'
ROOT = Path('.run/chapter2-sglang')
LOG = logging.getLogger(__name__)


def read_canvas():
    response = requests.get(BASE + '/api/canvases/' + CID, timeout=20)
    response.raise_for_status()
    return response.json()


def resolved(prompt, variables):
    for variable in variables:
        prompt = prompt.replace('{{' + variable['name'] + '}}', str(variable.get('value', '')))
    return prompt


def main():
    before = read_canvas()
    ROOT.mkdir(parents=True, exist_ok=True)
    (ROOT / 'before-expose-prompts.json').write_text(json.dumps(before, ensure_ascii=False, indent=2), encoding='utf-8')
    updated = {}
    count = 0
    for node in before['nodes']:
        if node['node_type'] != 'workflow':
            continue
        data = json.loads(node['content'])
        if data.get('outputType') != 'video':
            continue
        graph = json.loads(data['workflowJson'])
        planner = next((item for item in graph.values() if item['class_type'] == 'MiniMaxH3TimelinePlanner'), None)
        if not planner:
            continue
        timeline = json.loads(planner['inputs']['timeline_data'])
        segments = timeline['segmentConfig']['segments']
        old_variables = copy.deepcopy(data.get('variables', []))
        variables = copy.deepcopy(old_variables)
        names = {v['name'] for v in variables}
        expected = [resolved(segment['prompt'], old_variables) for segment in segments]
        changed = False
        for index, segment in enumerate(segments, 1):
            name = 'shot_' + str(index)
            placeholder = '{{' + name + '}}'
            if segment['prompt'] == placeholder and name in names:
                continue
            if name in names:
                raise ValueError('Existing shot variable conflicts; refusing to overwrite user edits: ' + node['id'])
            variables.append({'name': name, 'type': 'text', 'value': expected[index - 1]})
            segment['prompt'] = placeholder
            names.add(name)
            count += 1
            changed = True
        if not changed:
            continue
        assert [resolved(segment['prompt'], variables) for segment in segments] == expected
        planner['inputs']['timeline_data'] = json.dumps(timeline, ensure_ascii=False)
        data['workflowJson'] = json.dumps(graph, ensure_ascii=False)
        data['variables'] = variables
        # The UI fingerprint includes workflow representation and variables.
        # Never transfer an old approval to this changed configuration.
        if data.get('approvalRequired'):
            data['approvedFingerprint'] = ''
            data['reviewReason'] = 'Shot prompts exposed as editable variables; please review current inputs and confirm again.'
        response = requests.put(BASE + '/api/canvases/' + CID + '/nodes/' + node['id'],
                                json={'content': json.dumps(data, ensure_ascii=False)}, timeout=20)
        response.raise_for_status()
        updated[node['id']] = {'expected': expected, 'before': json.loads(node['content'])}
    after = read_canvas()
    for node in after['nodes']:
        if node['id'] not in updated:
            continue
        data = json.loads(node['content'])
        original = updated[node['id']]['before']
        graph = json.loads(data['workflowJson'])
        planner = next(item for item in graph.values() if item['class_type'] == 'MiniMaxH3TimelinePlanner')
        segments = json.loads(planner['inputs']['timeline_data'])['segmentConfig']['segments']
        assert [resolved(s['prompt'], data['variables']) for s in segments] == updated[node['id']]['expected']
        for field in ['outputs', 'lastPromptId', 'seedMode', 'lastSeed', 'shotManifest', 'sourceShots']:
            assert data.get(field) == original.get(field), field
        assert all(variable in data['variables'] for variable in original.get('variables', []))
    (ROOT / 'after-expose-prompts.json').write_text(json.dumps(after, ensure_ascii=False, indent=2), encoding='utf-8')
    scoped = copy.deepcopy(after)
    scoped['nodes'] = [n for n in scoped['nodes'] if n['node_type'] != 'workflow'
                       or json.loads(n['content']).get('outputType') != 'video'
                       or json.loads(n['content']).get('productionRole') == 'gated_keyframe_video']
    (ROOT / 'exposed-prompts-audit-scope.json').write_text(json.dumps(scoped, ensure_ascii=False, indent=2), encoding='utf-8')
    LOG.info('shot_prompts_exposed nodes=%s fields=%s effective_prompts_unchanged=true results_preserved=true submissions=0', len(updated), count)


if __name__ == '__main__':
    logging.basicConfig(level=logging.INFO, format='%(asctime)s %(module)s %(funcName)s %(message)s')
    main()
