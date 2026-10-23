"""Plan non-overlapping H3 segments and preserve edit frame counts."""
import math


def plan_frames(target_frames):
    rows = []
    cursor = 0
    for target in target_frames:
        if type(target) is not int or target <= 0:
            raise ValueError('Target frames must be positive integers')
        length = 5 + 17 * max(0, math.ceil((target - 5) / 17))
        if length > 3592:
            raise ValueError('Segment exceeds H3 frame limit')
        rows.append({'generated_start': cursor, 'generated_end': cursor + length,
                     'keep_frames': target, 'generated_frames': length})
        cursor += length
    if not rows:
        raise ValueError('At least one segment is required')
    return rows


def validate_segments(segments):
    previous_end = 0
    for segment in segments:
        start, end = segment['startFrame'], segment['endFrame']
        if type(start) is not int or type(end) is not int:
            raise ValueError('Boundaries must be integers')
        length = end - start
        if start != previous_end or length < 5 or length > 3592 or (length - 5) % 17:
            raise ValueError('Invalid non-overlapping H3 segment window')
        if not str(segment.get('prompt', '')).strip():
            raise ValueError('Missing segment prompt')
        previous_end = end
    if not segments:
        raise ValueError('Missing segments')
    return previous_end
