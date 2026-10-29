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


contract, client = load('contract'), load('client')


class ContractTests(unittest.TestCase):
    def setUp(self):
        self.plan = {'prompts': ['Preserve exact prompt.'], 'segment_plans': [{'width': 512, 'height': 288, 'length': 73, 'timeline': {}}], 'segment_overlaps': [0]}

    def test_canonical_timing_not_ignored_transport_fields(self):
        value = contract.compile_segments(self.plan, 42, 2)[0]
        self.assertEqual(value['target']['duration_seconds'], 73 / 24)
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

    def test_ambiguous_submission_never_resubmits(self):
        with tempfile.TemporaryDirectory() as directory:
            Path(directory, 'request.json').write_text(json.dumps({'payload': {}, 'state': 'submitting'}))
            with self.assertRaisesRegex(RuntimeError, 'unknown'):
                client.SGLangClient('http://unused.invalid').generate({}, directory)


if __name__ == '__main__':
    unittest.main()
