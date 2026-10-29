"""Validated first-frame export into a dedicated SGLang shared directory."""
import hashlib
import json
from pathlib import Path
from PIL import Image


def export_keyframes(raw, count, resolve, shared=Path('/sglang-references')):
    files = json.loads(raw)
    if not isinstance(files, list) or len(files) != count or not all(isinstance(f, str) and f.strip() for f in files):
        raise ValueError('Provide exactly one uploaded keyframe filename per Timeline segment')
    if not shared.is_dir():
        raise RuntimeError('Dedicated SGLang reference mount is unavailable')
    validated = []
    for filename in files:
        path = Path(resolve(filename)).resolve()
        with Image.open(path) as image:
            image.verify()
        suffix = path.suffix.lower()
        if suffix not in ['.png', '.jpg', '.jpeg', '.webp']:
            raise ValueError('Unsupported keyframe image format')
        data = path.read_bytes()
        digest = hashlib.sha256(data).hexdigest()
        validated.append((digest + suffix, data))
    # Validate all inputs before writing any references or submitting any request.
    uris = []
    for name, data in validated:
        target = shared / name
        if not target.exists():
            temp = target.with_suffix(target.suffix + '.tmp')
            temp.write_bytes(data)
            temp.replace(target)
        uris.append('/comfy/mnt/h3-out/timeline-references/' + name)
    return uris
