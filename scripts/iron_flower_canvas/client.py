"""Create/update the named local canvas through its documented HTTP API."""
import json
import logging
from pathlib import Path
import requests

LOG = logging.getLogger(__name__)


class CanvasClient:
    def __init__(self, base, canvas_id, workspace):
        self.base = base.rstrip('/')
        self.canvas_id = canvas_id
        self.directory = Path(workspace) / '.run' / 'iron-canvas'
        self.directory.mkdir(parents=True, exist_ok=True)
        self.state_path = self.directory / 'state.json'
        self.state = json.loads(self.state_path.read_text(encoding='utf-8')) if self.state_path.exists() else {'assets': {}, 'nodes': {}}
        before = self.call('GET', '/api/canvases/' + canvas_id)
        if not (self.directory / 'before.json').exists():
            self.write(self.directory / 'before.json', before)
        self.existing = {node['id'] for node in before.get('nodes') or []}

    @staticmethod
    def write(path, data):
        path.write_text(json.dumps(data, ensure_ascii=False, indent=2), encoding='utf-8')

    def persist(self):
        self.write(self.state_path, self.state)

    def call(self, method, path, **kwargs):
        response = requests.request(method, self.base + path, timeout=180, **kwargs)
        response.raise_for_status()
        return response.json() if response.content else None

    def asset(self, key, path, label, category='其他'):
        if key in self.state['assets']:
            return self.state['assets'][key]
        with path.open('rb') as stream:
            asset = self.call('POST', '/api/admin/assets/upload', files={'file': (label + path.suffix, stream)})
        self.call('PUT', '/api/admin/assets/' + asset['id'], json={'category': category, 'tags': json.dumps(['打铁花', '序幕45秒'], ensure_ascii=False)})
        self.state['assets'][key] = asset
        self.persist()
        LOG.info('asset_imported key=%s name=%s', key, label)
        return asset

    def node(self, key, kind, x, y, width, height, content):
        body = {'node_type': kind, 'x': x, 'y': y, 'width': width, 'height': height, 'content': content if isinstance(content, str) else json.dumps(content, ensure_ascii=False), 'config': '{}'}
        prefix = '/api/canvases/' + self.canvas_id + '/nodes'
        existing = self.state['nodes'].get(key)
        if existing and existing in self.existing:
            node = self.call('PUT', prefix + '/' + existing, json=body)
        else:
            node = self.call('POST', prefix, json=body)
            self.state['nodes'][key] = node['id']
            self.persist()
        return node['id']

    def edge(self, source, target):
        prefix = '/api/canvases/' + self.canvas_id + '/edges'
        if not hasattr(self, 'edges'):
            self.edges = {(e['source_node_id'], e['target_node_id']) for e in self.call('GET', prefix) or []}
        if (source, target) not in self.edges:
            self.call('POST', prefix, json={'source_node_id': source, 'target_node_id': target})
            self.edges.add((source, target))


def workflow(name, graph, variables, output_url='', output_type='image', approval=False):
    return {'displayName': name, 'name': name, 'serverUrl': 'http://10.88.0.3:8179', 'workflowJson': json.dumps(graph, ensure_ascii=False), 'variables': variables, 'outputType': output_type, 'outputVars': ['生成结果'], 'outputs': [{'name': '已导入的现有产物', 'type': output_type, 'value': output_url}] if output_url else [], 'approvalRequired': approval, 'approvedFingerprint': '', 'seedMode': 'random'}


def image_variable(name, url, source_id):
    return {'name': name, 'type': 'image', 'value': url, 'sourceNodeId': source_id}
