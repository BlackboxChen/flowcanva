"""Fail-closed translation for stage-one T2VA/FL2VA capability probes."""
import math


def compile_segments(finite, seed, steps, audio_policy='reject', task='t2va', reference_uri='', reference_uris=None):
    if isinstance(seed, bool) or not isinstance(seed, int) or not 0 <= seed < 2**63:
        raise ValueError('Seed must be an unsigned signed-63-bit-compatible integer')
    if not 1 <= steps <= 50:
        raise ValueError('Steps must be in [1, 50]')
    if finite.get('second_pass'):
        raise ValueError('Second-pass sampling has not been ported; no request submitted')
    if any(finite.get('segment_overlaps', [])) or finite.get('overlap_frames'):
        raise ValueError('Latent overlap continuation has not been ported; no request submitted')
    prompts, plans = finite.get('prompts', []), finite.get('segment_plans', [])
    if not prompts or len(prompts) != len(plans):
        raise ValueError('Finite plan prompts/materials are incomplete')
    if reference_uris is not None and (task != 'fl2va' or len(reference_uris) != len(prompts) or not all(reference_uris)):
        raise ValueError('FL2VA requires exactly one explicit first frame per Timeline segment')
    payloads = []
    for index, (prompt, plan) in enumerate(zip(prompts, plans)):
        timeline = plan.get('timeline', {})
        if timeline.get('images') and task == 't2va':
            raise ValueError('This plan has image references: T2VA must not silently discard them')
        if timeline.get('audios') and audio_policy != 'external_post':
            raise ValueError('Locked/reference audio is not ported; use an explicitly approved external-post test')
        width, height = int(plan['width']), int(plan['height'])
        length = int(plan['length'])
        if length < 5 or (length - 5) % 17:
            raise ValueError('Timeline frame count must be 5+17*n')
        if not 4 <= length / 24 <= 15:
            raise ValueError('SGLang H3 requires each segment duration in [4, 15] seconds')
        common = math.gcd(width, height)
        target = {'short_edge': min(width, height), 'aspect_ratio': f'{width//common}:{height//common}', 'duration_seconds': length / 24}
        conditions = []
        if task == 'fl2va':
            uri = reference_uris[index] if reference_uris is not None else reference_uri
            if reference_uris is None and (len(prompts) != 1 or not uri):
                raise ValueError('Stage-one FL2VA permits one segment and one explicit first frame only')
            if len(timeline.get('images', [])) > 1:
                raise ValueError('Multiple Timeline references cannot be reduced to one first frame')
            target['aspect_ratio'] = 'auto'
            conditions = [{'type': 'image', 'uri': uri, 'role': 'keyframe', 'frame_index': 0}]
        elif task != 't2va':
            raise ValueError('REF2VA partition is not enabled for this isolated adapter yet')
        payloads.append({'prompt': prompt, 'seed': int(seed) + index, 'num_inference_steps': int(steps), 'task': task, 'conditions': conditions, 'target': target})
    return payloads
