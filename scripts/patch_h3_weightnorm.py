"""Apply the verified weight-norm fix to a local copy of the H3 VAE source."""
import argparse
import logging
from pathlib import Path

OLD = '_w = _g * _v / _v.norm(dim=1, keepdim=True)'
NEW = '_w = _g * _v / _v.square().sum(dim=tuple(range(1, _v.ndim)), keepdim=True).sqrt()'


def patch(path):
    source = path.read_text(encoding='utf-8')
    if NEW in source and OLD not in source:
        return False
    if source.count(OLD) != 1:
        raise ValueError('Unexpected VAE source; inspect before patching')
    updated = source.replace(OLD, NEW)
    compile(updated, str(path), 'exec')
    backup = path.with_name(path.name + '.before-weightnorm-fix')
    with backup.open('x', encoding='utf-8') as stream:
        stream.write(source)
    path.write_text(updated, encoding='utf-8')
    return True


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('source', type=Path)
    args = parser.parse_args()
    logging.basicConfig(level=logging.INFO, format='%(asctime)s %(module)s %(funcName)s %(message)s')
    logging.info('weightnorm_patch changed=%s', patch(args.source))
