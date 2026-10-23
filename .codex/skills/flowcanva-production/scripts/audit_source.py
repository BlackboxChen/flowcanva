"""Audit this storyboard's numbered V segments against effective Timeline prompts."""
import argparse
import hashlib
import json
import logging
import re
from pathlib import Path


def parse_chapter(text, chapter):
    match = re.search(r'^### ' + re.escape(chapter) + r'[^\n]*\n(.*?)(?=^### |\Z)', text, re.M | re.S)
    if not match:
        raise ValueError('Chapter heading not found')
    result = {}
    for segment in re.finditer(r'^##### (V\d+) [^\n]*\n(.*?)(?=^##### |\Z)', match[1], re.M | re.S):
        shots = re.findall(r'^\[镜头\d+\] (.*?)(?=^\[镜头|^> 出画|^---|\Z)', segment[2], re.M | re.S)
        result[segment[1]] = [s.strip() for s in shots]
    if not result or any(not shots for shots in result.values()):
        raise ValueError('No numbered shots or unsupported storyboard format')
    return result


def audit(text, chapter, canvas):
    expected = parse_chapter(text, chapter)
    errors = []
    counts = {}
    for node in canvas['nodes']:
        if node['node_type'] != 'workflow':
            continue
        data = json.loads(node['content'])
        if data.get('outputType') != 'video':
            continue
        code = re.match(r'V\d+', data.get('displayName', ''))
        if not code or code[0] not in expected:
            continue
        code = code[0]
        counts[code] = counts.get(code, 0) + 1
        variables = {v['name']: v['value'] for v in data.get('variables', [])}
        graph = json.loads(data['workflowJson'])
        planner = next(v for v in graph.values() if v['class_type'] == 'MiniMaxH3TimelinePlanner')
        timeline = json.loads(planner['inputs']['timeline_data'])
        segments = timeline['segmentConfig']['segments']
        if len(segments) != len(expected[code]):
            errors.append(code + ': shot count differs')
        for i, (shot, segment) in enumerate(zip(expected[code], segments)):
            prompt = segment.get('prompt', '')
            for name, value in variables.items():
                prompt = prompt.replace('{{' + name + '}}', str(value))
            if shot not in prompt:
                errors.append(f'{code}/{i+1}: original text altered or missing')
            for forbidden in ['旁白与对白后期配音', 'No dialogue', '不生成对白']:
                if forbidden in prompt and forbidden not in shot:
                    errors.append(f'{code}/{i+1}: contradictory instruction {forbidden}')
        if timeline.get('audios'):
            errors.append(code + ': audio references require explicit source review')
    for code in expected:
        if counts.get(code) != 1:
            errors.append(code + ': missing or duplicate executable video node')
    return {'passed': not errors, 'errors': errors, 'segments': len(expected),
            'shots': sum(map(len, expected.values())),
            'source_sha256': hashlib.sha256(text.encode('utf-8')).hexdigest()}


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('--source', type=Path, required=True)
    parser.add_argument('--chapter', required=True)
    parser.add_argument('--canvas', type=Path, required=True)
    parser.add_argument('--report', type=Path, required=True)
    args = parser.parse_args()
    result = audit(args.source.read_text(encoding='utf-8-sig'), args.chapter,
                   json.loads(args.canvas.read_text(encoding='utf-8')))
    args.report.write_text(json.dumps(result, ensure_ascii=False, indent=2), encoding='utf-8')
    logging.basicConfig(level=logging.INFO, format='%(asctime)s %(module)s %(funcName)s %(message)s')
    logging.info('source_audit passed=%s errors=%s', result['passed'], result['errors'])
    raise SystemExit(0 if result['passed'] else 1)
