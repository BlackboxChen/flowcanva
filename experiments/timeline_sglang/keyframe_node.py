"""Timeline segments with one approved uploaded first frame per shot."""
import json
import logging
from pathlib import Path
import re
import sys
import folder_paths
from .client import SGLangClient
from .contract import compile_segments
from .keyframes import export_keyframes

LOG = logging.getLogger(__name__)


class TimelineSGLangKeyframes:
    CATEGORY = 'FlowCanva/Experimental'
    RETURN_TYPES = ('STRING',)
    FUNCTION = 'run'
    OUTPUT_NODE = True

    @classmethod
    def INPUT_TYPES(cls):
        return {'required': {
            'finite_plan': ('MINIMAX_H3_FINITE_SEGMENT_PLAN',),
            'keyframes_json': ('STRING', {'default': '[]', 'multiline': True}),
            'server_url': ('STRING', {'default': 'http://127.0.0.1:30010'}),
            'mode': (['inspect', 'generate'],),
            'run_token': ('STRING', {'default': 'keyframes-001'}),
            'seed': ('INT', {'default': 42, 'min': 0, 'max': 2147483647}),
            'steps': ('INT', {'default': 4, 'min': 1, 'max': 50}),
        }}

    @classmethod
    def IS_CHANGED(cls, **kwargs):
        return float('nan')

    def run(self, finite_plan, keyframes_json, server_url, mode, run_token, seed, steps):
        normalizer = next((getattr(module, '_require_finite_plan') for name, module in list(sys.modules.items()) if name.endswith('.minimax_h3_finite_segments') and hasattr(module, '_require_finite_plan')), None)
        if not normalizer:
            raise RuntimeError('Original TimelineDirector planner is required')
        if not re.fullmatch(r'[A-Za-z0-9_-]{1,80}', run_token):
            raise ValueError('Invalid run_token')
        finite = normalizer(finite_plan)
        # Validate the complete plan and exact reference count before any GPU request.
        count = len(finite['prompts'])
        compile_segments(finite, seed, steps, task='fl2va', reference_uris=['validated-later'] * count)
        def uploaded_path(filename):
            path = Path(folder_paths.get_annotated_filepath(filename)).resolve()
            if not path.is_relative_to(Path(folder_paths.get_input_directory()).resolve()):
                raise ValueError('Keyframes must be uploaded into this sandbox input directory')
            return path
        uris = export_keyframes(keyframes_json, count, uploaded_path)
        payloads = compile_segments(finite, seed, steps, task='fl2va', reference_uris=uris)
        directory = Path(folder_paths.get_output_directory()) / 'timeline_sglang_keyframes' / run_token
        directory.mkdir(parents=True, exist_ok=True)
        manifest = {'mode': mode, 'experimental': True, 'payloads': payloads, 'gpu_submissions': 0,
                    'audio_path': 'native FL2VA audio; locked audio unsupported',
                    'continuity': 'independent shots; no latent continuation'}
        (directory / 'plan.json').write_text(json.dumps(manifest, ensure_ascii=False, indent=2))
        if mode == 'inspect':
            LOG.info('keyframes_inspected shots=%s gpu_submissions=0', count)
            return {'ui': {'text': [json.dumps(manifest, ensure_ascii=False)]}, 'result': (json.dumps(manifest),)}
        client = SGLangClient(server_url)
        outputs = []
        for index, payload in enumerate(payloads):
            output = client.generate(payload, directory / f'shot_{index+1:02d}')
            outputs.append({'filename': output.name, 'subfolder': str(output.parent.relative_to(folder_paths.get_output_directory())), 'type': 'output'})
        report = json.dumps({'shots': len(outputs), 'run_token': run_token, 'experimental': True})
        LOG.info('keyframes_generation_completed run_token=%s shots=%s', run_token, len(outputs))
        return {'ui': {'images': outputs, 'text': [report]}, 'result': (report,)}
