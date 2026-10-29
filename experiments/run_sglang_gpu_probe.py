"""Run inside CPU sandbox; wait for existing four-card jobs, never cancel them."""
import importlib.util
import json
import logging
from pathlib import Path
import subprocess


def module(name):
    path = Path('/sandbox/custom_nodes/FlowTimelineSGLangSandbox') / (name + '.py')
    spec = importlib.util.spec_from_file_location(name, path)
    value = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(value)
    return value


def run():
    root = Path('/sandbox/reports/gpu-probe-v1')
    root.mkdir(parents=True, exist_ok=True)
    logging.basicConfig(level=logging.INFO, format='%(asctime)s %(levelname)s %(name)s %(funcName)s %(message)s', handlers=[logging.FileHandler(root / 'probe.log'), logging.StreamHandler()])
    client = module('client').SGLangClient('http://127.0.0.1:30010')
    contract = module('contract')
    plan = json.loads(Path('/sandbox/output/timeline_sglang_sandbox/inspect-002/finite_plan.json').read_text())
    if plan['segment_lengths'] != [73]:
        raise ValueError('Unexpected Timeline planner duration: refusing to submit GPU probe')
    results = []
    cases = [('t2va', ''), ('fl2va', Path('/sandbox/reports/reference_uri.txt').read_text().strip())]
    for task, reference in cases:
        payload = contract.compile_segments(plan, 20261029, 2, task=task, reference_uri=reference)[0]
        try:
            output = client.generate(payload, root / task)
            checked = subprocess.run(['ffprobe', '-v', 'error', '-show_streams', '-show_format', '-of', 'json', str(output)], capture_output=True, text=True, check=True)
            metadata = json.loads(checked.stdout)
            result = {'task': task, 'status': 'completed', 'path': str(output), 'media': metadata}
            logging.info('probe_completed task=%s path=%s', task, output)
        except Exception as error:
            result = {'task': task, 'status': 'failed_or_blocked', 'error': str(error)}
            logging.exception('probe_incomplete task=%s', task)
        results.append(result)
        (root / 'results.json').write_text(json.dumps(results, indent=2))


if __name__ == '__main__':
    run()
