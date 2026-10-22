"""Adapt the already-used iron-flower ComfyUI graphs into safe variables."""
import copy
import json

ASSETS = [('zhou_performance', '周铁山·表演造型', '人物'), ('villagers', '冬装观众', '人物'), ('rods', '柳木花棒', '物品'), ('gourd', '朱漆葫芦瓢', '物品'), ('furnace', '泥砖熔炉', '物品'), ('scaffold', '柳枝花棚', '场景'), ('riverbank', '夜间河滩', '场景')]
SHOTS = [('01_molten', '铁水特写', 5, 'rods'), ('02_resolve', '匠人蓄势', 4, 'zhou_performance'), ('03_grip_contact_review_v3', '低位握持击打', 6, 'approved_grip'), ('04_goldrain_dense', '花棚金雨', 6, 'riverbank'), ('05_audience', '观众惊叹', 4, 'villagers'), ('06_embers', '卵石余火', 5, 'riverbank'), ('07_aerial', '河滩鸟瞰', 8, 'riverbank'), ('08_title', '余烬片名', 7, '')]
VIDEO_NAMES = ['01_molten', '02_resolve', '03_low_stance', '04_goldrain_final', '05_audience', '06_embers', '07_aerial']


def load(path):
    return json.loads(path.read_text(encoding='utf-8'))


def image_graph(source, name):
    graph = load(source / 'workflows' / ('image_' + name + '.json'))
    graph = copy.deepcopy(graph)
    prompt = graph['4']['inputs']['prompt']
    graph['4']['inputs']['prompt'] = '{{prompt}}'
    refs = []
    for node in graph.values():
        if node['class_type'] == 'LoadImage':
            variable = 'reference_' + str(len(refs) + 1)
            refs.append(variable)
            node['inputs']['image'] = '{{' + variable + '}}'
        if node['class_type'] == 'SaveImage':
            node['inputs']['filename_prefix'] = 'flowcanva/{{run_id}}/' + name
    return graph, prompt, refs


def video_graph(source, index):
    path = source / 'workflows' / ('video_' + VIDEO_NAMES[index] + '.json')
    if index == 2:
        path = source / 'review_v3' / 'video_03_low_stance_api.json'
    graph = copy.deepcopy(load(path))
    timeline = json.loads(graph['11']['inputs']['timeline_data'])
    prompt = timeline['segmentConfig']['segments'][0]['prompt']
    timeline['images'][0]['file'] = '{{keyframe}}'
    timeline['segmentConfig']['segments'][0]['prompt'] = '{{motion}}'
    graph['11']['inputs']['timeline_data'] = json.dumps(timeline, ensure_ascii=False)
    graph['29']['inputs']['filename_prefix'] = 'flowcanva/{{run_id}}/iron_shot_' + str(index + 1)
    return graph, prompt


def full_graph(source):
    graph = copy.deepcopy(load(source / 'review_v3' / 'video_03_low_stance_api.json'))
    ui = load(source / 'Iron_Flower_Prologue_Timeline.json')
    planner = next(n for n in ui['nodes'] if n['type'] == 'MiniMaxH3TimelinePlanner')
    timeline = json.loads(planner['widgets_values'][3])
    for i, image in enumerate(timeline['images']):
        image['file'] = '{{frame_' + str(i + 1) + '}}'
    graph['11']['inputs'].update(generation_seconds=planner['widgets_values'][2], timeline_data=json.dumps(timeline, ensure_ascii=False))
    graph['29']['inputs']['filename_prefix'] = 'flowcanva/{{run_id}}/iron_seven_shots_source'
    return graph
