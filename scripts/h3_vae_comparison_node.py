"""Diagnostic ComfyUI node: decode one audio latent with both H3 VAE files."""
import json
import logging
import subprocess
import sys
import uuid
import wave
from pathlib import Path

import torch
import folder_paths

LOG = logging.getLogger(__name__)


class H3AudioVAEComparison:
    @classmethod
    def INPUT_TYPES(cls):
        return {'required': {'samples': ('LATENT',)}}

    RETURN_TYPES = ('STRING',)
    FUNCTION = 'compare'
    CATEGORY = 'MiniMax H3/Diagnostics'
    OUTPUT_NODE = True

    def compare(self, samples):
        latent = samples['samples']
        if latent.is_nested:
            latent = latent.unbind()[-1]
        latent = latent.detach().float().cpu().contiguous()
        if not torch.isfinite(latent).all():
            raise ValueError('Audio latent contains non-finite values')
        subfolder = 'vae_comparison/' + uuid.uuid4().hex
        directory = Path(folder_paths.get_output_directory()) / subfolder
        directory.mkdir(parents=True)
        source = directory / 'shared_audio_latent.pt'
        torch.save(latent, source)
        worker = Path(__file__).parent / 'ComfyUI-MiniMaxH3/audio_decode_worker.py'
        outputs, decoded = [], []
        for label, filename in [('original_fp32', 'minimax_h3_audio_vae_original_fp32.safetensors'),
                                ('fp32', 'minimax_h3_audio_vae_fp32.safetensors')]:
            model = folder_paths.get_full_path_or_raise('vae', filename)
            target = directory / (label + '.pt')
            process = subprocess.run([sys.executable, str(worker), str(source), str(target), model],
                                     capture_output=True, text=True, timeout=600)
            (directory / (label + '-decode.log')).write_text(process.stdout + process.stderr)
            if process.returncode:
                raise RuntimeError('VAE worker failed: ' + process.stderr[-1500:])
            audio = torch.load(target, map_location='cpu', weights_only=True).float()
            if not torch.isfinite(audio).all() or audio.ndim != 3:
                raise ValueError('Decoded audio is invalid')
            audio /= (audio.std(dim=[1, 2], keepdim=True) * 5).clamp(min=1)
            decoded.append(audio)
            with wave.open(str(directory / (label + '.wav')), 'wb') as stream:
                stream.setnchannels(audio.shape[1])
                stream.setsampwidth(2)
                stream.setframerate(32000)
                stream.writeframes((audio[0].T.clamp(-1, 1) * 32767).round().short().contiguous().numpy().tobytes())
            outputs.append({'filename': label + '.wav', 'subfolder': subfolder, 'type': 'output'})
        a, b = decoded
        if a.shape != b.shape:
            raise ValueError('Decoded audio shapes differ')
        metrics = {'latent_shape': list(latent.shape), 'correlation': torch.corrcoef(
            torch.stack([a.flatten(), b.flatten()]))[0, 1].item(),
            'difference_rms': (a-b).square().mean().sqrt().item(),
            'max_abs_difference': (a-b).abs().max().item(),
            'models': ['minimax_h3_audio_vae_original_fp32.safetensors',
                       'minimax_h3_audio_vae_fp32.safetensors']}
        (directory / 'metrics.json').write_text(json.dumps(metrics, indent=2))
        LOG.info('H3 VAE comparison %s', metrics)
        return {'ui': {'audio': outputs}, 'result': (json.dumps(metrics),)}


NODE_CLASS_MAPPINGS = {'H3AudioVAEComparison': H3AudioVAEComparison}
