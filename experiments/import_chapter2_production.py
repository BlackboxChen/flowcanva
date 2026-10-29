"""Incrementally add production assets, keyframes and review to chapter two."""
import copy
import hashlib
import html
import json
import logging
from pathlib import Path
import re
import requests

BASE = 'http://127.0.0.1:16789'
CID = 'cv_3fef3e02'
STATE = Path('.run/chapter2-sglang')
LOG = logging.getLogger(__name__)


def call(method, path, **kwargs):
    response = requests.request(method, BASE + path, timeout=30, **kwargs)
    response.raise_for_status()
    return response.json()


def main():
    before = call('GET', '/api/canvases/' + CID)
    statefile = STATE / 'production-import-map.json'
    mapping = json.loads(statefile.read_text(encoding='utf-8')) if statefile.exists() else {}
    source = Path('C:/Users/win10-pc/Downloads/打铁花_分镜脚本.md').read_text(encoding='utf-8-sig')
    sha = hashlib.sha256(source.encode()).hexdigest()
    chapter = re.search(r'^### 第二章.*?(?=^### 第三章)', source, re.M | re.S)[0]
    old = call('GET', '/api/canvases/cv_7306ce4b')
    workflow_data = [json.loads(node['content']) for node in old['nodes'] if node['node_type'] == 'workflow']
    image_template = next(data for data in workflow_data if '柳木花棒' in data.get('displayName', ''))
    server = image_template['serverUrl']
    info = requests.get(server + '/object_info', timeout=20).json()
    template = json.loads(image_template['workflowJson'])
    for item in template.values():
        if item['class_type'] not in info:
            raise ValueError('Missing image node: ' + item['class_type'])
    prefix = '/api/canvases/' + CID
    existing_edges = {(edge['source_node_id'], edge['target_node_id']) for edge in before['edges']}

    def node(key, kind, x, y, content, width=460, height=300):
        if key in mapping:
            return mapping[key]
        result = call('POST', prefix + '/nodes', json={'node_type': kind, 'x': x, 'y': y, 'width': width, 'height': height, 'content': content if isinstance(content, str) else json.dumps(content, ensure_ascii=False), 'config': '{}'})
        mapping[key] = result['id']
        statefile.write_text(json.dumps(mapping, ensure_ascii=False, indent=2), encoding='utf-8')
        return result['id']

    def note(key, x, y, title, text, width=900, height=300):
        return node(key, 'text', x, y, '<h2>' + html.escape(title) + '</h2><pre>' + html.escape(text) + '</pre>', width, height)

    def edge(start, end):
        if (start, end) not in existing_edges:
            call('POST', prefix + '/edges', json={'source_node_id': start, 'target_node_id': end})
            existing_edges.add((start, end))

    source_node = next(n['id'] for n in before['nodes'] if '第二章原稿｜完整保留' in n['content'])
    note('production-guide', 3400, 0, '制作流程｜资产→逐镜关键帧→你确认→视频→声音→交付',
         '本区依flowcanva-production技能补齐。现有V11生成结果与全部用户配置保留。\n'
         '先运行缺失资产，再运行K区关键帧；23镜逐镜修稿，由你确认后才能进入生产视频。\n'
         '左侧8个四卡T2VA节点保留为实验对照，不消费本区关键帧，不应当作已完成的图生视频链路。\n'
         '四卡关键帧输入、锁定音轨和跨镜一致性仍需适配；不通过假连线表示已支持。\n'
         '此轮未提交任何生成，未确认任何关键帧。生图沿用已验证Qwen Image 2.1服务，不占四卡SGLang路径。', 1100, 400)
    definitions = {m[1]: m[0] for m in re.finditer(r'^#### \{@([^:]+):[^\n]+', source, re.M)}
    scene_rows = {m[1]: m[0] for m in re.finditer(r'^\| \{@(S\d+):[^\n]+', source, re.M)}
    scene_prompts = {}
    for code in ['S03', 'S05', 'S06']:
        scene_prompts[code] = re.search(r'\*\*' + code + r' [^\n]+\*\*\s*```\n(.*?)\nNegative:', source, re.S)[1]
    specs = [('zhou', '周铁山·日常服装', definitions['周铁山'] + '。单人角色设定，穿日常藏青布褂，双手放松下垂，正面及四分之三视图，保持同一人物，无文字。', []),
             ('chuan', '周小川·日常与表演身份', definitions['周小川'] + '。日常造型角色设定，戴眼镜、浅灰卫衣、牛仔裤、白球鞋、黑背包。单人正面及四分之三视图，无文字。', []),
             *[(code, code + '场景', prompt, []) for code, prompt in scene_prompts.items()],
             ('photo', '老照片', '原稿道具：木框黑白老照片，泛黄卷边，照片中一位年轻男子与一位中年男子站在木花棚前。单幅道具设定，真实材质，无额外文字。', [])]
    asset_ids, urls = {}, {}

    def image(key, title, prompt, references, x, y, extra=None):
        graph = copy.deepcopy(template)
        encoder = next(v for v in graph.values() if v['class_type'] == 'TextEncodeQwenImage21')
        encoder['inputs']['prompt'] = '{{prompt}}'
        for name in list(encoder['inputs']):
            if name.startswith('images.'):
                del encoder['inputs'][name]
        for graph_id in list(graph):
            if graph[graph_id]['class_type'] == 'LoadImage':
                del graph[graph_id]
        variables = [{'name': 'prompt', 'type': 'text', 'value': prompt}]
        for index, ref in enumerate(references, 1):
            graph_id = str(100 + index)
            graph[graph_id] = {'class_type': 'LoadImage', 'inputs': {'image': '{{reference_' + str(index) + '}}'}}
            encoder['inputs']['images.image_' + str(index)] = [graph_id, 0]
            variables.append({'name': 'reference_' + str(index), 'type': 'image', 'value': urls.get(ref, ''), 'sourceNodeId': asset_ids[ref]})
        for item in graph.values():
            if item['class_type'] == 'SaveImage':
                item['inputs']['filename_prefix'] = 'flowcanva/{{run_id}}/' + key
        data = {'displayName': title, 'name': title, 'serverUrl': server, 'workflowJson': json.dumps(graph, ensure_ascii=False),
                'variables': variables, 'outputs': [], 'outputType': 'image', 'outputVars': ['图像'], 'lastPromptId': '',
                'seedMode': 'random', 'approvalRequired': False, 'approvedFingerprint': '', 'sourceSha256': sha,
                'status': 'not_submitted', **(extra or {})}
        result = node(key, 'workflow', x, y, data)
        for ref in references:
            edge(asset_ids[ref], result)
        return result

    for index, (key, label, prompt, refs) in enumerate(specs):
        asset_ids[key] = image('asset-' + key, 'A｜' + label, prompt, refs, 3400 + index % 3 * 520, 500 + index // 3 * 400)
        edge(source_node, asset_ids[key])
    for index, (key, phrase) in enumerate([('rods', '柳木花棒'), ('furnace', '泥砖熔炉'), ('gourd', '朱漆葫芦瓢')]):
        original = next(d for d in workflow_data if phrase in d.get('displayName', ''))
        output = next(o for o in original['outputs'] if o.get('type') == 'image')
        url = output['value']
        response = requests.get(BASE + url if url.startswith('/') else url, timeout=20)
        response.raise_for_status()
        asset_ids[key] = node('asset-' + key, 'asset', 3400 + index * 520, 1320,
                             {'displayName': 'A｜复用·' + phrase, 'name': '历史资产·' + phrase, 'url': url, 'mediaType': 'image',
                              'provenance': {'canvas': old['id'], 'label': original['displayName'], 'status': 'historical, not chapter-two approval'}})
        urls[key] = url
    for row, match in enumerate(re.finditer(r'^##### (V\d+) ｜ (.*?) ｜ 15s\n(.*?)(?=^#####|\Z)', chapter, re.M | re.S)):
        code, title, body = match.groups()
        shots = [s.strip() for s in re.findall(r'^\[镜头\d+\] (.*?)(?=^\[镜头|^> 出画|^---|\Z)', body, re.M | re.S)]
        scene = re.search(r'@S\d+', body)[0][1:]
        review = note('review-' + code, 5050, 1750 + row * 450, code + '｜关键帧待你审核',
                      '逐镜核对原稿：身份、日常/表演服装、构图、手与木棒接触、左右手、炉具距离、道具不悬浮。\n'
                      '当前所有关键帧均未生成、未确认。此审核节点不自动批准视频。\n'
                      '该V段关键帧通过后，才能配置和执行消费对应关键帧的生产视频路径。', 750, 350)
        for number, raw in enumerate(shots, 1):
            refs = [scene]
            # A context shot can include characters named in the segment's tags.
            if '@周铁山' in body and not ('特写' in raw and '掌心' in raw):
                refs.append('zhou')
            if '@周小川' in body:
                refs.append('chuan')
            if '@花棒' in raw:
                refs.append('rods')
            if '@熔炉' in raw:
                refs.append('furnace')
            if '@葫芦瓢' in raw:
                refs.append('gourd')
            tags = '\n'.join(definitions[k] for k in ['周铁山', '周小川'] if '@' + k in body)
            prompt = '电影质感纪录片，写实，35mm胶片颗粒，自然肤质，16:9，自然光低饱和。单幅关键帧，不是拼贴或设定图，无字幕。\n' + tags + '\n' + scene_rows[scene] + '\n' + raw + '\n此节点生成该镜开场静帧，音效、台词保留为原稿信息；不把对白渲染成画面文字。服装按本镜原稿，参考图仅约束身份、场景、材质。'
            key = f'K-{code}-{number}'
            frame = image(key, key + '｜' + title, prompt, refs, 3400 + (number - 1) * 520, 1750 + row * 450,
                          {'sourceShots': [{'id': f'{code}-{number}', 'raw': raw}], 'technicalPrefix': '单幅纪实关键帧，16:9', 'reviewState': 'pending'})
            edge(frame, review)
    audio = note('audio-plan', 3400, 5550, '音轨｜原稿声音清单·待制作',
                 '\n'.join(line for line in chapter.splitlines() if line.startswith('[镜头')) + '\n尚无真实锁定音频文件，未试听，不代表已完成配音或音效。', 1100, 450)
    post = note('delivery-plan', 4600, 5550, '交付｜120秒·待剪辑',
                '23镜生成素材按原稿5/9/6秒等逐镜裁尾，再按V11–V18顺序拼接为120秒。\n'
                '检查中文对白、旁白、音效、口型；V11章节卡按原稿后期叠加。\n'
                '此为真实待办清单；尚无自动渲染、锁定音轨、最终成片。', 1000, 350)
    edge(audio, post)
    after = call('GET', prefix)
    (STATE / 'production-after.json').write_text(json.dumps(after, ensure_ascii=False, indent=2), encoding='utf-8')
    for old_node in before['nodes']:
        current = next(n for n in after['nodes'] if n['id'] == old_node['id'])
        if current != old_node:
            raise RuntimeError('Existing user node changed: ' + old_node['id'])
    LOG.info('production_import canvas=%s added_nodes=%s keyframes=23 new_generation_submissions=0 existing_nodes_preserved=true', CID, len(after['nodes']) - len(before['nodes']))


if __name__ == '__main__':
    logging.basicConfig(level=logging.INFO, format='%(asctime)s %(module)s %(funcName)s %(message)s')
    main()
