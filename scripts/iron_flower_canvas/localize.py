"""Persist existing workflow outputs through the running asset API; never generate."""
import argparse
import datetime
import hashlib
import json
import logging
from pathlib import Path
from urllib.parse import urlsplit, parse_qs
import requests

LOG = logging.getLogger(__name__)


def replace_refs(value, mapping):
    if isinstance(value, str):
        return mapping.get(value, value)
    if isinstance(value, list):
        return [replace_refs(item, mapping) for item in value]
    if isinstance(value, dict):
        return {key: item if key == 'sourceURL' else replace_refs(item, mapping) for key, item in value.items()}
    return value


def localize(base, canvas_id):
    endpoint = base + '/api/canvases/' + canvas_id
    response = requests.get(endpoint, timeout=30)
    response.raise_for_status()
    canvas = response.json()
    folder = Path('.run/iron-canvas') / ('localize-' + datetime.datetime.now().strftime('%Y%m%d-%H%M%S'))
    folder.mkdir(parents=True, exist_ok=False)
    (folder / 'before.json').write_text(json.dumps(canvas, ensure_ascii=False, indent=2), encoding='utf-8')
    mapping = {}
    for node in canvas['nodes']:
        if node['node_type'] != 'workflow':
            continue
        data = json.loads(node['content'])
        for output in data.get('outputs', []):
            remote = output.get('value', '')
            if output.get('type') not in ['image', 'video', 'audio'] or not remote.startswith(('http://', 'https://')) or remote in mapping:
                continue
            parsed = urlsplit(remote)
            filename = Path(parse_qs(parsed.query).get('filename', [parsed.path.rsplit('/', 1)[-1]])[0]).name
            response = requests.get(remote, timeout=(15, 120))
            response.raise_for_status()
            mime = response.headers.get('Content-Type', '')
            if not mime.startswith(('image/', 'video/', 'audio/')) or not response.content:
                raise RuntimeError('Remote output is not a media file: ' + filename)
            digest = hashlib.sha256(response.content).hexdigest()
            uploaded = requests.post(base + '/api/admin/assets/upload', files={'file': (filename, response.content, mime)}, timeout=180)
            uploaded.raise_for_status()
            asset = uploaded.json()
            local = requests.get(base + asset['url'], timeout=120)
            local.raise_for_status()
            if hashlib.sha256(local.content).hexdigest() != digest:
                raise RuntimeError('Local copy mismatch: ' + filename)
            mapping[remote] = asset['url']
            (folder / 'mapping.json').write_text(json.dumps(mapping, ensure_ascii=False, indent=2), encoding='utf-8')
            LOG.info('media_persisted name=%s bytes=%s sha256=%s url=%s', data.get('name'), len(local.content), digest, asset['url'])
    changed = 0
    for node in canvas['nodes']:
        try:
            original = json.loads(node['content'])
        except (ValueError, TypeError):
            continue
        updated = replace_refs(original, mapping)
        if updated == original:
            continue
        for old, new in zip(original.get('outputs', []), updated.get('outputs', [])):
            if old.get('value') in mapping:
                new['sourceURL'] = old.get('sourceURL') or old['value']
        if updated.get('approvalRequired'):
            updated['approvedFingerprint'] = ''
        # Re-read immediately before saving so a concurrent edit is never overwritten.
        current = requests.get(endpoint, timeout=30)
        current.raise_for_status()
        latest = next(n for n in current.json()['nodes'] if n['id'] == node['id'])
        if latest['content'] != node['content']:
            raise RuntimeError('Node changed during repair; restart against current data: ' + node['id'])
        saved = requests.put(endpoint + '/nodes/' + node['id'], json={'content': json.dumps(updated, ensure_ascii=False)}, timeout=30)
        saved.raise_for_status()
        changed += 1
    LOG.info('localization_complete media=%s changed_nodes=%s backup=%s', len(mapping), changed, folder)


if __name__ == '__main__':
    logging.basicConfig(level=logging.INFO, format='%(asctime)s %(levelname)s %(name)s %(funcName)s %(message)s')
    parser = argparse.ArgumentParser()
    parser.add_argument('--base', default='http://127.0.0.1:16789')
    parser.add_argument('--canvas', default='cv_7306ce4b')
    args = parser.parse_args()
    localize(args.base.rstrip('/'), args.canvas)
