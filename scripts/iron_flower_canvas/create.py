"""Populate the requested canvas without submitting any generation jobs."""
import argparse
import html
import json
import logging
from pathlib import Path
from client import CanvasClient, workflow, image_variable
from graphs import ASSETS, SHOTS, load, image_graph, video_graph, full_graph


def note(title, body):
    return '<h1>' + html.escape(title) + '</h1><p>' + body.replace('\n', '</p><p>') + '</p>'


def build(client, source):
    plan = load(source / 'production_plan.json')
    title = client.node('overview', 'text', 0, 0, 710, 320, note('打铁花｜序幕 45 秒', '纪实写实 · 16:9 · 24fps\n资产 → 关键帧 → <b>你确认并定稿</b> → 逐镜视频 → 后期交付\n此画布只预置工作流和既有产物，尚未提交新的生成任务。\n双击节点可查看；执行需手动点击。\n关键帧或动作修改后，视频节点的确认会自动失效。'))
    script = (source / 'source_storyboard.md').read_text(encoding='utf-8')
    prologue = script.split('### 序幕', 1)[1].split('### 第一章', 1)[0]
    script_node = client.node('script', 'text', 750, 0, 920, 320, note('原稿｜序幕 V01–V03', '<pre>' + html.escape(prologue) + '</pre>'))
    guide = client.node('guide', 'text', 2310, 0, 750, 320, note('使用顺序', '① 复用或运行 A 区资产。\n② 在 K 区修改提示词并生成关键帧。\n③ 查看各镜关键帧，修稿后定稿。\n④ 选中 V 节点，检查参考图，勾选“已确认当前关键帧”再执行。\n⑤ 后期裁时、混入旁白音效、添加片名，得到 45 秒。\n参考图自动沿连线传递，执行时由本地后端上传至内网 ComfyUI。'))
    client.node('asset_label', 'text', 0, 370, 3060, 78, note('A｜人物 / 场景 / 道具资产', 'Qwen Image 2.1 · 已导入既有产物，可按需单独重生；连线绑定到下方关键帧。'))
    asset_ids, asset_urls = {}, {}
    for i, (key, label, category) in enumerate(ASSETS):
        asset = client.asset('asset_' + key, source / 'assets' / (key + '.png'), label, category)
        graph, prompt, _ = image_graph(source, key)
        asset_ids[key] = client.node('asset_' + key, 'workflow', i * 390, 480, 340, 230, workflow('A' + str(i + 1) + '｜' + label, graph, [{'name': 'prompt', 'type': 'text', 'value': prompt}], asset['url']))
        asset_urls[key] = asset['url']
        client.edge(script_node, asset_ids[key])
    approved = client.asset('approved_grip', source / 'review_v3/03_grip_fixed.png', '第3镜·已确认握持参考', '人物')
    asset_ids['approved_grip'] = client.node('approved_grip', 'asset', 2730, 480, 340, 230, {'displayName': 'A8｜已确认·一手一棒', 'name': '已确认握持参考', 'url': approved['url'], 'mediaType': 'image'})
    asset_urls['approved_grip'] = approved['url']
    client.node('frame_label', 'text', 0, 760, 3060, 78, note('K｜逐镜关键帧', '新产物会自动更新对应 V 节点的参考图；图像生成不会自动触发视频生成。第3镜使用最新握持修正版。'))
    key_ids, key_urls = [], []
    for i, (key, label, seconds, ref) in enumerate(SHOTS):
        path = source / 'review_v3/03_grip_fixed.png' if i == 2 else source / 'keyframes' / (key + '.png')
        asset = client.asset('key_' + str(i), path, f'K{i+1:02d}·{label}', '场景' if i not in [1, 2, 4] else '人物')
        graph, prompt, references = image_graph(source, key)
        variables = []
        if references:
            variables += [image_variable(name, asset_urls[ref], asset_ids[ref]) for name in references]
        variables.append({'name': 'prompt', 'type': 'text', 'value': prompt})
        nid = client.node('key_' + str(i), 'workflow', i * 390, 870, 340, 245, workflow(f'K{i+1:02d}｜{label} · {seconds}s', graph, variables, asset['url']))
        key_ids.append(nid)
        key_urls.append(asset['url'])
        if ref:
            client.edge(asset_ids[ref], nid)
    gate = client.node('gate', 'text', 0, 1170, 820, 225, note('人工确认｜修稿完成后再生视频', '<b>逐镜检查：</b>人物身份与服装、场景构图、手指与道具接触、光线和出画状态。\n<b>第3镜：</b>一手一棒，手指包住握柄，棒尾可见，工具低于肩部，无悬浮或额外杆件。\n在对应视频节点勾选确认后才可执行；更换参考图或提示词会重新锁定。'))
    rows, cursor = [], 0
    for i, (_, label, seconds, _) in enumerate(SHOTS):
        rows.append([f'{i+1:02d}', label, f'{cursor:02d}–{cursor+seconds:02d}s', str(seconds) + 's', '待本画布确认'])
        cursor += seconds
    ledger = client.node('review_table', 'table', 900, 1170, 1120, 225, {'cols': ['镜头', '画面', '成片时间', '时长', '复核记录'], 'colWidths': [70, 180, 140, 80, 280], 'rows': rows})
    client.node('invariants', 'text', 2100, 1170, 960, 225, note('生成参数与连续性', 'Qwen Image 2.1：25步，保持原生产图尺寸。\nMiniMax H3 ref2va + Timeline：1024×576、24fps；主体镜头分别为 5/4/6/6/4/5/8 秒。\n模型按 17k+5 帧补齐，后期逐镜裁成 38 秒；片名动画 7 秒另做。\n各节点含本任务实际 API 工作流，不依赖公网模型配置。'))
    video_ids = []
    for i in range(7):
        _, label, seconds, _ = SHOTS[i]
        video_path = source / 'review_v3/03_low_stance_cut.mp4' if i == 2 else source / 'video' / (['01_molten', '02_resolve', '', '04_goldrain', '05_audience', '06_embers', '07_aerial'][i] + '_cut.mp4')
        asset = client.asset('video_' + str(i), video_path, f'V{i+1:02d}·{label}·已剪辑素材', '视频')
        graph, prompt = video_graph(source, i)
        variables = [image_variable('keyframe', key_urls[i], key_ids[i]), {'name': 'motion', 'type': 'text', 'value': prompt}]
        nid = client.node('video_' + str(i), 'workflow', i * 390, 1470, 340, 245, workflow(f'V{i+1:02d}｜{label} · {seconds}s', graph, variables, asset['url'], 'video', True))
        video_ids.append(nid)
        client.edge(key_ids[i], nid)
    client.node('title_post', 'text', 2730, 1470, 340, 245, note('V08｜片名动画 · 7s', '以 K08 黑底余烬为底，后期添加粒子和中文片名。\n片名：非遗传承 · 打铁花\n38–39s 留白，随后渐显，末尾淡出黑场。\n不让视频模型生成中文文字。'))
    final_asset = client.asset('final', source / 'review_v3/铁花_序幕_45秒_握持修复版.mp4', '打铁花·45秒·握持修复成片', '视频')
    final_id = client.node('final', 'asset', 0, 1830, 650, 390, {'displayName': '交付参考｜最新45秒成片', 'name': '已完成历史成片·新镜头需重新剪辑', 'url': final_asset['url'], 'mediaType': 'video'})
    audio = client.asset('audio', source / 'review_v3/audio/master.wav', '旁白与环境音·45秒', '音频')
    audio_id = client.node('audio', 'asset', 740, 1830, 420, 155, {'displayName': '音轨｜旁白与音效', 'url': audio['url'], 'name': '旁白与音效·45秒', 'mediaType': 'audio'})
    post = client.node('post', 'text', 1240, 1830, 900, 390, note('后期交付清单｜总长45秒', '① 将主体素材裁为 5/4/6/6/4/5/8 秒，共38秒。\n② 接7秒黑底余烬与中文片名。\n③ 叠加这条45秒旁白音效；更换击打动作后，重新对齐撞击声。\n④ 输出1920×1080、24fps、H.264/AAC。\n⑤ 检查手与工具逐帧接触、音画时点、黑场结尾。\n<b>当前画布不提供剪辑渲染引擎：</b>此步骤由 FFmpeg / 剪辑软件完成，已有成片作为参考。\n新生成素材不会自动覆盖已导入成片。'))
    client.edge(audio_id, post)
    for nid in video_ids:
        client.edge(nid, post)
    client.edge(post, final_id)
    variables = [image_variable('frame_' + str(i+1), key_urls[i], key_ids[i]) for i in range(7)]
    batch = client.node('batch', 'workflow', 2220, 1830, 840, 390, workflow('高级｜Timeline七镜批量素材', full_graph(source), variables, output_type='video', approval=True))
    for nid in key_ids[:7]:
        client.edge(nid, batch)
    segments = []
    shot_cursor = 0
    for part, indices in enumerate([[0, 1, 2], [3, 4, 5], [6, 7]]):
        shots = []
        for index in indices:
            _, label, seconds, _ = SHOTS[index]
            shots.append({'idx': index+1, 't': str(seconds)+'s', 'action': label, 'composition': plan['shots'][index]['frame_prompt'], 'camera': '固定或缓推，按原稿', 'performance': plan['shots'][index]['action'], 'dialogue': '旁白按原稿保留' if index in [2, 5] else '', 'sound': '按原稿及统一音轨', 'transition': '硬切；片尾淡出'})
        segments.append({'id': f'V{part+1:02d}', 'title': ['黑暗中的火', '万点金雨', '片名'][part], 'duration_s': 15, 'form': 'multi_shot', 'coverage': {'EV': [f'EV{i+1:02d}' for i in indices], 'DLG': [f'DLG{part+1:02d}'] if part < 2 else []}, 'shots': shots, 'storyboard': {'mode': 'use', 'rows': 1, 'cols': len(indices), 'n': len(indices), 'review': 'pending'}, 'prompt': '\n'.join(plan['shots'][i]['action'] for i in indices)})
    board = {'version': '3.7', 'project': {'title': '非遗传承·打铁花｜序幕', 'target_platform': 'MiniMax H3 Timeline', 'target_duration_s': 45}, 'state': 'PLAN_CONFIRMED', 'gates': {}, 'script': {'source_node_id': script_node, 'version': '序幕原稿', 'EV': [{'id': f'EV{i+1:02d}', 'text': shot[1]} for i, shot in enumerate(SHOTS)], 'DLG': [{'id': 'DLG01', 'speaker': '旁白', 'text': '一千多年前，有人把铁水泼向了夜空。'}, {'id': 'DLG02', 'speaker': '旁白', 'text': '从那以后，这团火，就再没熄过。'}], 'LOCK': [{'id': 'LOCK01', 'fact': '第3镜低位、一手一棒、无悬浮；新关键帧先确认再生成视频。'}]}, 'assets': [{'handle': '@'+label, 'node_id': asset_ids[key], 'name': label, 'use': '身份、材质、场景保持一致', 'not_use': '不复制设定图排版'} for key, label, _ in ASSETS], 'duration': {'target_s': 45, 'faithful_estimate_s': 45, 'verdict': 'ok', 'note': '3段各15秒；主体38秒+片名7秒。'}, 'spatial': {'needed': True, 'reason': '工具接触与花棚空间方向连续', 'blocking': '人物左、开阔空间右，双棒保持在腹前', 'axis': '保持同侧轴线'}, 'segments': segments, 'ledger': [], 'notes': '此处管理脚本与分镜；实际生图、生视频在K/V节点手动执行。', 'spawned': {'script': script_node, 'plan': ledger}}
    planner = client.node('planner', 'shotlist', 1720, 0, 520, 320, board)
    client.edge(script_node, planner)
    client.edge(planner, gate)
    client.edge(guide, title)
    client.call('PUT', '/api/canvases/' + client.canvas_id, json={'name': '打铁花｜序幕45秒·关键帧确认工作流'})
    after = client.call('GET', '/api/canvases/' + client.canvas_id)
    client.write(client.directory / 'after.json', after)
    logging.info('canvas_ready canvas=%s nodes=%s edges=%s', client.canvas_id, len(after['nodes']), len(after['edges']))


if __name__ == '__main__':
    logging.basicConfig(level=logging.INFO, format='%(asctime)s %(levelname)s %(name)s %(funcName)s %(message)s')
    parser = argparse.ArgumentParser()
    parser.add_argument('--source', type=Path, required=True)
    parser.add_argument('--canvas', default='cv_7306ce4b')
    parser.add_argument('--base', default='http://127.0.0.1:16789')
    args = parser.parse_args()
    build(CanvasClient(args.base, args.canvas, Path.cwd()), args.source)
