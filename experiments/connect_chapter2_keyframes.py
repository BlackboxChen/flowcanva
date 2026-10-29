"""Add real gated four-card first-frame video nodes, preserving T2VA comparisons."""
import copy
import importlib.util
import json
import logging
from pathlib import Path
import requests

BASE = 'http://127.0.0.1:16789'
SERVER = 'http://127.0.0.1:18299'
CID = 'cv_3fef3e02'
STATE = Path('.run/chapter2-sglang')
LOG = logging.getLogger(__name__)


def call(method, path, **kwargs):
    response = requests.request(method, BASE + path, timeout=30, **kwargs)
    response.raise_for_status()
    return response.json()


def main():
    path = '/api/canvases/' + CID
    before = call('GET', path)
    (STATE / 'before-keyframe-bridge.json').write_text(json.dumps(before, ensure_ascii=False, indent=2), encoding='utf-8')
    mapping = json.loads((STATE / 'production-import-map.json').read_text(encoding='utf-8'))
    record = STATE / 'keyframe-video-map.json'
    created = json.loads(record.read_text()) if record.exists() else {}
    response = requests.get(SERVER + '/object_info/FlowTimelineSGLangKeyframes', timeout=20)
    response.raise_for_status()
    if 'FlowTimelineSGLangKeyframes' not in response.json():
        raise RuntimeError('First-frame bridge node is not loaded')
    edges = {(e['source_node_id'], e['target_node_id']) for e in before['edges']}
    old_videos = [n for n in before['nodes'] if n['node_type'] == 'workflow' and json.loads(n['content']).get('outputType') == 'video']
    for index, original in enumerate(old_videos):
        data = json.loads(original['content'])
        code = data['displayName'][:3]
        if code in created:
            continue
        graph = json.loads(data['workflowJson'])
        planner_id = next(k for k, v in graph.items() if v['class_type'] == 'MiniMaxH3TimelinePlanner')
        bridge_id = next(k for k, v in graph.items() if v['class_type'] == 'FlowTimelineSGLangSandbox')
        old_input = graph[bridge_id]['inputs']
        timeline = json.loads(graph[planner_id]['inputs']['timeline_data'])
        count = len(timeline['segmentConfig']['segments'])
        variables = [{'name': 'mode', 'type': 'text', 'value': 'generate'}]
        names = []
        for shot in range(1, count + 1):
            name = 'keyframe_' + str(shot)
            source_id = mapping[f'K-{code}-{shot}']
            variables.append({'name': name, 'type': 'image', 'value': '', 'sourceNodeId': source_id})
            names.append('{{' + name + '}}')
        graph[bridge_id] = {'class_type': 'FlowTimelineSGLangKeyframes', 'inputs': {
            'finite_plan': [planner_id, 2], 'keyframes_json': json.dumps(names), 'server_url': old_input['server_url'],
            'mode': '{{mode}}', 'run_token': CID + '-' + code + '-kf-{{run_id}}', 'seed': 42, 'steps': old_input['steps']}}
        new_data = {key: copy.deepcopy(data[key]) for key in ['sourceShots', 'sourceSha256', 'shotManifest'] if key in data}
        new_data.update(displayName=code + '｜四卡首帧·确认后执行', name=code + '｜四卡首帧·确认后执行',
                        serverUrl=SERVER, workflowJson=json.dumps(graph, ensure_ascii=False), variables=variables,
                        outputType='video', outputVars=['逐镜视频'], outputs=[], lastPromptId='', seedMode='random',
                        approvalRequired=True, approvedFingerprint='', productionRole='gated_keyframe_video',
                        referenceMode='one uploaded approved first frame per Timeline segment', status='not_submitted',
                        audioPath='native FL2VA audio; locked audio unsupported; dialogue and effects need listening review')
        result = call('POST', path + '/nodes', json={'node_type': 'workflow', 'x': 6000, 'y': 1750 + index * 450,
                      'width': 560, 'height': 330, 'content': json.dumps(new_data, ensure_ascii=False), 'config': '{}'})
        created[code] = result['id']
        record.write_text(json.dumps(created, indent=2))
        for variable in variables:
            if variable.get('sourceNodeId'):
                pair = (variable['sourceNodeId'], result['id'])
                if pair not in edges:
                    call('POST', path + '/edges', json={'source_node_id': pair[0], 'target_node_id': pair[1]})
                    edges.add(pair)
        call('POST', path + '/edges', json={'source_node_id': result['id'], 'target_node_id': mapping['delivery-plan']})
    # Only update our capability note, never the user's existing video configuration.
    fresh = call('GET', path)
    guide = next(n for n in fresh['nodes'] if n['id'] == mapping['production-guide'])
    content = '<h2>四卡制作链路｜资产→关键帧→你确认→逐镜FL2VA→审核→剪辑</h2><pre>' + (
        '本区按flowcanva-production技能组织。先生成资产，再逐镜生成关键帧并由你确认修稿。\n'
        '最右侧8个“首帧·确认后执行”视频节点消费真实关键帧，逐镜传入四卡SGLang。\n'
        '每个首帧均已绑定sourceNodeId与工作流变量；未生成/未确认时禁止执行。\n'
        '左侧T2VA节点保留为诊断对照，V11已有输出完整保留。\n'
        '视频阶段仍为独立镜头，尚无跨镜latent连续性、锁定音轨或自动剪辑。原稿对白音效要求保留，需试听。\n'
        '当前4步、512×288用于接口调试。真实生成效果待你确认关键帧后执行测试。\n'
        '本轮未生成任何新资产或视频，也未替你批准关键帧。') + '</pre>'
    call('PUT', path + '/nodes/' + guide['id'], json={'content': content})
    after = call('GET', path)
    (STATE / 'keyframe-bridge-after.json').write_text(json.dumps(after, ensure_ascii=False, indent=2), encoding='utf-8')
    scoped = copy.deepcopy(after)
    scoped['nodes'] = [n for n in scoped['nodes'] if n['node_type'] != 'workflow' or json.loads(n['content']).get('outputType') != 'video' or json.loads(n['content']).get('productionRole') == 'gated_keyframe_video']
    (STATE / 'production-video-audit-scope.json').write_text(json.dumps(scoped, ensure_ascii=False, indent=2), encoding='utf-8')
    spec = importlib.util.spec_from_file_location('audit', '.codex/skills/flowcanva-production/scripts/audit_source.py')
    audit_module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(audit_module)
    source = Path('C:/Users/win10-pc/Downloads/打铁花_分镜脚本.md').read_text(encoding='utf-8-sig')
    audit = audit_module.audit(source, '第二章', scoped)
    audit['scope'] = 'eight gated keyframe video nodes; eight unchanged T2VA diagnostics excluded'
    (STATE / 'keyframe-video-source-audit.json').write_text(json.dumps(audit, ensure_ascii=False, indent=2), encoding='utf-8')
    if not audit['passed']:
        raise RuntimeError(audit['errors'])
    for original in old_videos:
        assert original == next(n for n in after['nodes'] if n['id'] == original['id'])
    LOG.info('keyframe_video_connected nodes=%s shots=23 approval_required=true new_submissions=0', len(created))


if __name__ == '__main__':
    logging.basicConfig(level=logging.INFO, format='%(asctime)s %(module)s %(funcName)s %(message)s')
    main()
