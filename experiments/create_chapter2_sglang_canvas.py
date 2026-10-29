"""Import chapter two into a new isolated test canvas; never submit generation."""
import copy
import hashlib
import html
import importlib.util
import json
import logging
from pathlib import Path
import re
import requests

BASE = 'http://127.0.0.1:16789'
SERVER = 'http://127.0.0.1:18299'
SOURCE = Path('C:/Users/win10-pc/Downloads/打铁花_分镜脚本.md')
STATE = Path('.run/chapter2-sglang')
LOG = logging.getLogger(__name__)


def call(method, path, **kwargs):
    response = requests.request(method, BASE + path, timeout=30, **kwargs)
    response.raise_for_status()
    return response.json()


def main():
    STATE.mkdir(parents=True, exist_ok=True)
    if (STATE / 'canvas-id.txt').exists():
        raise RuntimeError('Canvas already created; inspect the saved ID rather than creating duplicates')
    source = SOURCE.read_text(encoding='utf-8-sig')
    chapter = re.search(r'^### 第二章.*?(?=^### 第三章)', source, re.M | re.S)[0].strip()
    definitions = re.findall(r'^#### \{@[^\n]+', source, re.M)
    scenes = re.findall(r'^\| \{@S0[356]:[^\n]+', source, re.M)
    props = source.split('### 1.3', 1)[1].split('### 1.4', 1)[0] if '### 1.3' in source else ''
    source_hash = hashlib.sha256(source.encode()).hexdigest()
    index = call('GET', '/api/canvases')
    (STATE / 'before-index.json').write_text(json.dumps(index, ensure_ascii=False, indent=2), encoding='utf-8')
    for item in index['items']:
        snapshot = call('GET', '/api/canvases/' + item['id'])
        (STATE / (item['id'] + '-before.json')).write_text(json.dumps(snapshot, ensure_ascii=False), encoding='utf-8')
    response = requests.get(SERVER + '/object_info', timeout=20)
    response.raise_for_status()
    info = response.json()
    for name in ['MiniMaxH3TimelinePlanner', 'FlowTimelineSGLangSandbox']:
        if name not in info:
            raise RuntimeError('Required sandbox node missing: ' + name)
    canvas = call('POST', '/api/canvases', json={'name': '打铁花｜第二章·传承｜四卡SGLang实验', 'project_type': 'video'})
    cid = canvas['id']
    (STATE / 'canvas-id.txt').write_text(cid)
    prefix = '/api/canvases/' + cid

    def node(kind, x, y, data, width=440, height=300):
        content = data if isinstance(data, str) else json.dumps(data, ensure_ascii=False)
        return call('POST', prefix + '/nodes', json={'node_type': kind, 'x': x, 'y': y, 'width': width, 'height': height, 'content': content, 'config': '{}'})['id']

    def text(x, y, title, body, width=900, height=300):
        return node('text', x, y, '<h2>' + html.escape(title) + '</h2><pre>' + html.escape(body) + '</pre>', width, height)

    def edge(start, end):
        call('POST', prefix + '/edges', json={'source_node_id': start, 'target_node_id': end})

    text(0, 0, '第二章·传承｜120秒｜四卡SGLang实验',
         'V11–V18，23镜。仅已建图，未执行生成。\n路径：本地18299 SSH通道 → 独立CPU ComfyUI → 四卡SGLang30010。\n'
         '本画布测试T2VA文字驱动，不绑定参考图或锁定音频，人物一致性、原稿对白和音效待生成后验收。\n'
         '4步、512×288用于接口测试，不代表最终画质；各镜独立生成，无跨镜latent连续性。\n'
         '建议先V11测试。一个V节点会顺序生成该段全部镜头，返回多个视频，不会自动剪辑。', 1050, 400)
    text(1100, 0, '操作说明｜每次执行随机种子',
         '点击视频节点 → 配置 → 执行。mode默认generate；改inspect只检查Timeline不推理。\n'
         '默认每次随机种子，后端自动注入新的run_id目录标识，重新执行会创建新任务；实际种子和任务ID会保留。\n'
         '原任务由已有prompt_id继续查询；状态未知或超时先查原任务，不要点击执行来恢复。\n'
         '服务忙时等待，不取消别人的任务。当前锁定音频、多参考身份及自动后期尚未适配。\n'
         '先逐镜检查结果；如后续改用关键帧，应先由你确认，再配置图生视频。', 1050, 400)
    source_node = text(2200, 0, '第二章原稿｜完整保留', chapter, 1000, 700)
    context_node = text(0, 470, '角色·场景·道具设定｜原稿文字参考', '\n'.join(definitions + scenes) + '\n' + props, 1050, 600)
    text(1100, 470, '声音和后期验收清单',
         '保留提示词中全部台词、旁白、音效要求；T2VA声音准确度及中文口型待实测。未提供锁定音轨。\n'
         '每镜素材按其原稿keep_frames裁尾，再按镜头顺序拼接为15秒段落；8段共120秒。\n'
         'V11章节卡按原稿后期叠加。无自动配音、裁切、拼接、字幕、音乐渲染节点；此处为清单。', 1050, 300)
    template = json.loads(Path('experiments/timeline_sglang/inspect_workflow_api.json').read_text())
    jobs = []
    total_keep = 0
    total_generated = 0
    for row, match in enumerate(re.finditer(r'^##### (V\d+) ｜ (.*?) ｜ 15s\n(.*?)(?=^#####|\Z)', chapter, re.M | re.S)):
        code, title, body = match.groups()
        raw_shots = [part.strip() for part in re.findall(r'^\[镜头\d+\] (.*?)(?=^\[镜头|^> 出画|^---|\Z)', body, re.M | re.S)]
        segments, manifest = [], []
        cursor = 0
        scene = next((line for line in scenes if '@' + re.search(r'@S\d+', body)[0][1:] + ':' in line), '')
        for number, raw in enumerate(raw_shots, 1):
            seconds = int(re.search(r'/(\d+)s\)', raw)[1])
            keep = seconds * 24
            length = 5 + max(0, (keep - 5 + 16) // 17) * 17
            context = '\n'.join(definitions) + '\n' + scene
            prompt = context + '\n电影质感纪录片，写实风格，35mm胶片颗粒，自然肤质，16:9，自然光低饱和。\n' + raw
            segments.append({'startFrame': cursor, 'endFrame': cursor + length, 'images': [], 'audios': [], 'prompt': prompt})
            manifest.append({'id': f'{code}-{number}', 'raw': raw, 'keep_frames': keep, 'generated_frames': length, 'generated_start': cursor, 'generated_end': cursor + length})
            cursor += length
            total_keep += keep
            total_generated += length
        if sum(item['keep_frames'] for item in manifest) != 360:
            raise ValueError('Unexpected original segment duration: ' + code)
        graph = copy.deepcopy(template)
        timeline = json.loads(graph['11']['inputs']['timeline_data'])
        timeline.update(globalPrompt='', selection={'start': 0, 'duration': cursor / 24}, segmentConfig={'count': len(segments), 'mode': 'timeline', 'segments': segments})
        graph['11']['inputs'].update(generation_seconds=cursor / 24, timeline_data=json.dumps(timeline, ensure_ascii=False))
        graph['12']['inputs'].update(mode='{{mode}}', run_token=cid + '-' + code + '-{{run_id}}', seed=20261029 + row, steps=4)
        data = {'displayName': code + '｜' + title + '｜四卡SGLang', 'name': code + '四卡测试',
                'serverUrl': SERVER, 'workflowJson': json.dumps(graph, ensure_ascii=False),
                'variables': [{'name': 'mode', 'type': 'text', 'value': 'generate'}],
                'outputType': 'video', 'outputVars': ['分镜视频'], 'outputs': [], 'lastPromptId': '',
                'seedMode': 'random', 'fixedSeed': str(20261029 + row), 'lastSeed': None,
                'approvalRequired': False, 'approvedFingerprint': '', 'sourceSha256': source_hash,
                'sourceShots': [{'id': item['id'], 'raw': item['raw']} for item in manifest],
                'shotManifest': manifest, 'referenceMode': 'experimental T2VA, no reference images', 'status': 'not_submitted'}
        y = 1150 + row * 450
        original = text(0, y, code + '｜' + title + '｜原稿', body, 700, 400)
        video = node('workflow', 800, y, data, 560, 330)
        review = text(1500, y, code + '｜审核与逐镜裁尾', '\n'.join(f"{item['id']}: 生成{item['generated_frames']}帧，保留前{item['keep_frames']}帧" for item in manifest) + '\n检查人物、持棒动作、道具接触、对白、口型、音效。未生成、未审核。', 650, 330)
        edge(original, video)
        edge(video, review)
        jobs.append({'code': code, 'node': video, 'shots': manifest})
    final = call('GET', prefix)
    (STATE / 'canvas-after.json').write_text(json.dumps(final, ensure_ascii=False, indent=2), encoding='utf-8')
    (STATE / 'manifest.json').write_text(json.dumps({'canvas': cid, 'source_sha256': source_hash, 'keep_frames': total_keep, 'generated_frames': total_generated, 'jobs': jobs, 'submissions': 0}, ensure_ascii=False, indent=2), encoding='utf-8')
    spec = importlib.util.spec_from_file_location('source_audit', '.codex/skills/flowcanva-production/scripts/audit_source.py')
    audit_module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(audit_module)
    audit = audit_module.audit(source, '第二章', final)
    (STATE / 'source-audit.json').write_text(json.dumps(audit, ensure_ascii=False, indent=2), encoding='utf-8')
    if not audit['passed']:
        raise RuntimeError(audit['errors'])
    LOG.info('canvas_created canvas=%s videos=%s shots=%s keep_seconds=%s submissions=0', cid, len(jobs), sum(len(job['shots']) for job in jobs), total_keep / 24)


if __name__ == '__main__':
    logging.basicConfig(level=logging.INFO, format='%(asctime)s %(module)s %(funcName)s %(message)s')
    main()
