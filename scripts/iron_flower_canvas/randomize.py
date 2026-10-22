"""Switch existing seed-bearing workflows to random mode without generating media."""
import argparse
import datetime
import json
import logging
from pathlib import Path
import requests

LOG = logging.getLogger(__name__)
SEED_KEYS = {'seed', 'noise_seed', 'random_seed'}


def find_seed(value):
    if isinstance(value, dict):
        for key, item in value.items():
            if key in SEED_KEYS and isinstance(item, int):
                return item
            found = find_seed(item)
            if found is not None:
                return found
    elif isinstance(value, list):
        for item in value:
            found = find_seed(item)
            if found is not None:
                return found
    return None


def update(base, canvas_id):
    endpoint = base.rstrip('/') + '/api/canvases/' + canvas_id
    response = requests.get(endpoint, timeout=30)
    response.raise_for_status()
    canvas = response.json()
    backup = Path('.run/iron-canvas') / ('before-random-' + datetime.datetime.now().strftime('%Y%m%d-%H%M%S') + '.json')
    backup.parent.mkdir(parents=True, exist_ok=True)
    backup.write_text(json.dumps(canvas, ensure_ascii=False, indent=2), encoding='utf-8')
    changed = 0
    for node in canvas.get('nodes') or []:
        if node['node_type'] != 'workflow':
            continue
        data = json.loads(node['content'])
        seed = find_seed(json.loads(data.get('workflowJson', '{}')))
        if seed is None:
            continue
        if data.get('seedMode') == 'random':
            continue
        data['seedMode'] = 'random'
        data.setdefault('fixedSeed', str(seed))
        # The recorded mode is part of the video approval fingerprint.
        if data.get('approvalRequired'):
            data['approvedFingerprint'] = ''
        result = requests.put(endpoint + '/nodes/' + node['id'], json={'content': json.dumps(data, ensure_ascii=False)}, timeout=30)
        result.raise_for_status()
        changed += 1
        LOG.info('node_randomized name=%s original_seed=%s', data.get('name'), seed)
    LOG.info('random_mode_ready canvas=%s changed=%s backup=%s', canvas_id, changed, backup)


if __name__ == '__main__':
    logging.basicConfig(level=logging.INFO, format='%(asctime)s %(levelname)s %(name)s %(funcName)s %(message)s')
    parser = argparse.ArgumentParser()
    parser.add_argument('--base', default='http://127.0.0.1:16789')
    parser.add_argument('--canvas', default='cv_7306ce4b')
    args = parser.parse_args()
    update(args.base, args.canvas)
