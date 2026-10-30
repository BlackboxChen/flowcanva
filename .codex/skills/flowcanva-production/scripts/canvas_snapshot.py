"""Read and save a canvas without modifying it (standard library only)."""
import argparse
import json
import logging
from pathlib import Path
from urllib.request import urlopen
from urllib.parse import quote


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--base', required=True, help='Explicit FlowCanva instance URL')
    parser.add_argument('--canvas', required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    with urlopen(args.base.rstrip('/') + '/api/canvases/' + quote(args.canvas, safe=''), timeout=20) as response:
        canvas = json.load(response)
    if canvas.get('id') != args.canvas or not isinstance(canvas.get('nodes'), list):
        raise ValueError('Unexpected canvas response')
    args.output.parent.mkdir(parents=True, exist_ok=True)
    with args.output.open('x', encoding='utf-8') as stream:
        json.dump(canvas, stream, ensure_ascii=False, indent=2)
    logging.info('snapshot_saved canvas=%s nodes=%s path=%s', args.canvas, len(canvas['nodes']), args.output)


if __name__ == '__main__':
    logging.basicConfig(level=logging.INFO, format='%(asctime)s %(module)s %(funcName)s %(message)s')
    main()
