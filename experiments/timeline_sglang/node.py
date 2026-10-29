import json
import logging
import sys
import re
from pathlib import Path
import folder_paths
from .client import SGLangClient
from .contract import compile_segments

LOG = logging.getLogger(__name__)


class TimelineSGLangSandbox:
    CATEGORY = 'FlowCanva/Experimental'
    RETURN_TYPES = ('STRING',)
    FUNCTION = 'run'
    OUTPUT_NODE = True

    @classmethod
    def INPUT_TYPES(cls):
        return {'required': {
            'finite_plan': ('MINIMAX_H3_FINITE_SEGMENT_PLAN',),
            'server_url': ('STRING', {'default': 'http://127.0.0.1:30010'}),
            'mode': (['inspect', 'generate'],),
            'task': (['t2va', 'fl2va'],),
            'audio_policy': (['reject', 'external_post'],),
            'reference_uri': ('STRING', {'default': ''}),
            'run_token': ('STRING', {'default': 'probe-001'}),
            'seed': ('INT', {'default': 42, 'min': 0, 'max': 2147483647}),
            'steps': ('INT', {'default': 4, 'min': 1, 'max': 50}),
        }}

    @classmethod
    def IS_CHANGED(cls, **kwargs):
        return float('nan')

    def run(self, finite_plan, server_url, mode, task, audio_policy, reference_uri, run_token, seed, steps):
        normalizer = next((getattr(module, '_require_finite_plan') for name, module in list(sys.modules.items()) if name.endswith('.minimax_h3_finite_segments') and hasattr(module, '_require_finite_plan')), None)
        if not normalizer:
            raise RuntimeError('Load the original TimelineDirector planner in this sandbox')
        finite = normalizer(finite_plan)
        if not re.fullmatch(r'[A-Za-z0-9_-]{1,80}', run_token):
            raise ValueError('run_token must be an explicit safe identifier; reuse it to resume, change it for a new run')
        run_id = run_token
        directory = Path(folder_paths.get_output_directory()) / 'timeline_sglang_sandbox' / run_id
        directory.mkdir(parents=True, exist_ok=True)
        def metadata(value):
            return {'python_type': type(value).__name__, 'shape': list(value.shape) if hasattr(value, 'shape') else None}
        (directory / 'finite_plan.json').write_text(json.dumps(finite, default=metadata, ensure_ascii=False, indent=2))
        if mode == 'inspect':
            report = {'mode': 'inspect', 'segments': len(finite['prompts']), 'report_path': str(directory / 'finite_plan.json'), 'gpu_submissions': 0}
            return {'ui': {'text': [json.dumps(report)]}, 'result': (json.dumps(report),)}
        payloads = compile_segments(finite, seed, steps, audio_policy, task, reference_uri)
        client = SGLangClient(server_url)
        images = []
        for index, payload in enumerate(payloads):
            output = client.generate(payload, directory / f'segment_{index+1:02d}')
            images.append({'filename': output.name, 'subfolder': str(output.parent.relative_to(folder_paths.get_output_directory())), 'type': 'output'})
        report = json.dumps({'run_id': run_id, 'segments': len(images), 'experimental': True, 'output_policy': 'independent segments; no latent continuity claimed'})
        LOG.info('timeline_sglang_complete run_id=%s segments=%s', run_id, len(images))
        return {'ui': {'images': images, 'text': [report]}, 'result': (report,)}
