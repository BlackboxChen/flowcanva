"""Run inside the ComfyUI venv to test the actual isolated audio worker."""
import argparse
import logging
import runpy
import subprocess
import sys
import tempfile
from pathlib import Path

import torch


def verify(worker, model):
    loader = runpy.run_path(str(worker), run_name='audio_vae_verification')
    pack = loader['load_vae_pack']('', str(model), device='cpu', pin=False)
    time = torch.arange(32000) / 32000
    wave = (0.2 * torch.sin(2 * torch.pi * 440 * time)).reshape(1, 1, -1).repeat(1, 2, 1)
    with torch.no_grad():
        latent = pack.encode_audio(wave)
    if not torch.isfinite(latent).all():
        raise ValueError('Test latent contains non-finite values')
    with tempfile.TemporaryDirectory(prefix='h3_audio_verify_') as directory:
        source, target = Path(directory) / 'latent.pt', Path(directory) / 'wave.pt'
        torch.save(latent, source)
        result = subprocess.run([sys.executable, str(worker), str(source), str(target), str(model)],
                                capture_output=True, text=True, timeout=180)
        if result.returncode:
            raise RuntimeError(result.stderr[-2000:])
        decoded = torch.load(target, map_location='cpu', weights_only=True)
    if decoded.shape != wave.shape or not torch.isfinite(decoded).all():
        raise ValueError('Invalid decoded waveform')
    correlation = torch.corrcoef(torch.stack([wave.flatten(), decoded.flatten()]))[0, 1].item()
    rms = decoded.square().mean().sqrt().item()
    if correlation < 0.98 or not 0.10 < rms < 0.18:
        raise ValueError(f'Audio reconstruction failed: correlation={correlation} rms={rms}')
    logging.info('audio_roundtrip passed correlation=%.6f rms=%.6f', correlation, rms)


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('--worker', type=Path, required=True)
    parser.add_argument('--model', type=Path, required=True)
    args = parser.parse_args()
    logging.basicConfig(level=logging.INFO, format='%(asctime)s %(module)s %(funcName)s %(message)s')
    verify(args.worker, args.model)
