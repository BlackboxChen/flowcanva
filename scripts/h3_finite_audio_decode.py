"""Isolated CPU audio decode for the server's finite H3 sampler."""
import logging
import subprocess
import sys
import tempfile
from pathlib import Path

import torch
import folder_paths

LOG = logging.getLogger(__name__)


def decode_finite_audio(vae, samples):
    latent = samples['samples']
    if latent.is_nested:
        latent = latent.unbind()[-1]
    latent = latent.detach().float().cpu().contiguous()
    if not torch.isfinite(latent).all():
        raise ValueError('H3 audio latent contains NaN/Inf before decode')
    worker = Path(__file__).resolve().parent.parent / 'ComfyUI-MiniMaxH3/audio_decode_worker.py'
    model = folder_paths.get_full_path_or_raise('vae', 'minimax_h3_audio_vae_original_fp32.safetensors')
    if not worker.is_file():
        raise RuntimeError('H3 isolated audio decode worker is missing')
    with tempfile.TemporaryDirectory(prefix='h3_finite_audio_') as directory:
        source, target = Path(directory) / 'latent.pt', Path(directory) / 'audio.pt'
        torch.save(latent, source)
        result = subprocess.run([sys.executable, str(worker), str(source), str(target), model],
                                capture_output=True, text=True, timeout=600)
        if result.returncode or not target.is_file():
            raise RuntimeError('H3 CPU audio worker failed: ' + result.stderr[-1500:])
        audio = torch.load(target, map_location='cpu', weights_only=True).float()
    if audio.ndim != 3 or audio.shape[-1] == 0 or not torch.isfinite(audio).all():
        raise ValueError('H3 isolated decoded audio is empty or contains NaN/Inf')
    # Match ComfyUI audio normalization, retaining the original waveform.
    audio /= (audio.std(dim=[1, 2], keepdim=True) * 5).clamp(min=1)
    LOG.info('H3 finite CPU audio decoded shape=%s finite=True', tuple(audio.shape))
    return {'waveform': audio, 'sample_rate': 32000}
