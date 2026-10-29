import copy
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest


def load(name):
    spec = importlib.util.spec_from_file_location(name, Path(__file__).parent / 'timeline_sglang' / (name + '.py'))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


contract, client, keyframes = load('contract'), load('client'), load('keyframes')


class ContractTests(unittest.TestCase):
    def setUp(self):
        self.plan = {'prompts': ['Preserve exact prompt.'], 'segment_plans': [{'width': 512, 'height': 288, 'length': 107, 'timeline': {}}], 'segment_overlaps': [0]}

    def test_canonical_timing_not_ignored_transport_fields(self):
        value = contract.compile_segments(self.plan, 42, 2)[0]
        self.assertEqual(value['target']['duration_seconds'], 107 / 24)
        self.assertEqual(value['prompt'], 'Preserve exact prompt.')
        self.assertNotIn('fps', value)
        self.assertNotIn('num_frames', value)

    def test_locked_audio_rejected_by_default(self):
        self.plan['segment_plans'][0]['timeline']['audios'] = [{'file': 'speech.wav'}]
        with self.assertRaises(ValueError): contract.compile_segments(self.plan, 42, 2)

    def test_external_post_must_be_explicit(self):
        self.plan['segment_plans'][0]['timeline']['audios'] = [{'file': 'speech.wav'}]
        self.assertEqual(len(contract.compile_segments(self.plan, 42, 2, 'external_post')), 1)

    def test_t2va_does_not_drop_reference_images(self):
        self.plan['segment_plans'][0]['timeline']['images'] = [{'file': 'hero.png'}]
        with self.assertRaises(ValueError): contract.compile_segments(self.plan, 42, 2)

    def test_fl2va_explicit_first_frame(self):
        value = contract.compile_segments(self.plan, 42, 2, task='fl2va', reference_uri='/shared/first.png')[0]
        self.assertEqual(value['conditions'][0]['frame_index'], 0)
        self.assertEqual(value['target']['aspect_ratio'], 'auto')

    def test_multiple_images_not_silently_collapsed(self):
        self.plan['segment_plans'][0]['timeline']['images'] = [{}, {}]
        with self.assertRaises(ValueError): contract.compile_segments(self.plan, 42, 2, task='fl2va', reference_uri='first.png')

    def test_overlap_and_second_pass_rejected(self):
        for key, value in [('segment_overlaps', [17]), ('second_pass', True)]:
            plan = copy.deepcopy(self.plan)
            plan[key] = value
            with self.assertRaises(ValueError): contract.compile_segments(plan, 42, 2)

    def test_invalid_frames_rejected(self):
        self.plan['segment_plans'][0]['length'] = 72
        with self.assertRaises(ValueError): contract.compile_segments(self.plan, 42, 2)

    def test_sglang_duration_limit(self):
        for length in [73, 362]:
            self.plan['segment_plans'][0]['length'] = length
            with self.assertRaises(ValueError): contract.compile_segments(self.plan, 42, 2)

    def test_one_first_frame_per_segment(self):
        self.plan['prompts'].append('Second shot.')
        self.plan['segment_plans'].append(copy.deepcopy(self.plan['segment_plans'][0]))
        values = contract.compile_segments(self.plan, 42, 4, task='fl2va', reference_uris=['a.png', 'b.png'])
        self.assertEqual([v['conditions'][0]['uri'] for v in values], ['a.png', 'b.png'])
        self.assertEqual([v['seed'] for v in values], [42, 43])
        for refs in [[], ['a.png'], ['a.png', '']]:
            with self.assertRaises(ValueError):
                contract.compile_segments(self.plan, 42, 4, task='fl2va', reference_uris=refs)

    def test_keyframes_validated_before_any_reference_written(self):
        from PIL import Image
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            shared = root / 'shared'
            shared.mkdir()
            Image.new('RGB', (16, 16)).save(root / 'valid.png')
            (root / 'invalid.png').write_bytes(b'not an image')
            with self.assertRaises(Exception):
                keyframes.export_keyframes('["valid.png", "invalid.png"]', 2, lambda f: root / f, shared)
            self.assertFalse(list(shared.iterdir()))
            values = keyframes.export_keyframes('["valid.png"]', 1, lambda f: root / f, shared)
            self.assertEqual(len(values), 1)
            self.assertTrue(values[0].startswith('/comfy/mnt/h3-out/timeline-references/'))
            self.assertEqual(len(list(shared.iterdir())), 1)

    def test_ambiguous_submission_never_resubmits(self):
        with tempfile.TemporaryDirectory() as directory:
            Path(directory, 'request.json').write_text(json.dumps({'payload': {}, 'state': 'submitting'}))
            with self.assertRaisesRegex(RuntimeError, 'unknown'):
                client.SGLangClient('http://unused.invalid').generate({}, directory)


if __name__ == '__main__':
    unittest.main()
