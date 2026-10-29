"""Deploy a CPU-only, independent ComfyUI planner/HTTP bridge sandbox."""
import ast
import json
import logging
from pathlib import Path
import shlex
import paramiko

ROOT = Path(__file__).resolve().parent
REMOTE = '/home/thao/timeline-sglang-sandbox'
CONTAINER = 'flowcanva-timeline-sglang-sandbox'
PORT = 18299
REFERENCES = '/home/thao/comfyui-sglang-pilot/run/h3-out/timeline-references'
LOG = logging.getLogger(__name__)


def connect():
    tree = ast.parse(Path('D:/sk_show/remote.py').read_text(encoding='utf-8'))
    _, user, password = next(ast.literal_eval(n.value) for n in tree.body if isinstance(n, ast.Assign) and isinstance(n.targets[0], ast.Tuple))
    client = paramiko.SSHClient()
    client.load_system_host_keys()
    client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
    client.connect('10.88.0.30', username=user, password=password, timeout=10)
    return client


def command(client, cmd, timeout=60):
    _, output, error = client.exec_command(cmd, timeout=timeout)
    text = output.read().decode(errors='replace')
    errors = error.read().decode(errors='replace')
    if output.channel.recv_exit_status():
        raise RuntimeError(errors or text)
    return text


def deploy(image_source='h3-comfy-long'):
    client = connect()
    try:
        # Refuse to replace even an earlier sandbox without an explicit follow-up.
        exists = command(client, "docker ps -a --format '{{.Names}}'").splitlines()
        if CONTAINER in exists:
            raise RuntimeError('Sandbox already exists; inspect it instead of replacing it')
        if command(client, f"ss -ltnH 'sport = :{PORT}'").strip():
            raise RuntimeError('Sandbox port already occupied; no existing service will be changed')
        image = command(client, 'docker inspect ' + shlex.quote(image_source) + " --format '{{.Image}}'").strip()
        user = command(client, 'id -u').strip() + ':' + command(client, 'id -g').strip()
        command(client, 'mkdir -p ' + ' '.join(shlex.quote(REMOTE + '/' + name) for name in ['custom_nodes/FlowTimelineSGLangSandbox', 'input', 'output', 'temp', 'user', 'models', 'reports']))
        command(client, 'mkdir -p ' + shlex.quote(REFERENCES))
        sftp = client.open_sftp()
        for source in (ROOT / 'timeline_sglang').glob('*.py'):
            sftp.put(str(source), REMOTE + '/custom_nodes/FlowTimelineSGLangSandbox/' + source.name)
        sftp.close()
        planner_link = shlex.quote(REMOTE + '/custom_nodes/ComfyUI-MiniMaxH3-TimelineDirector')
        command(client, 'test -L ' + planner_link + ' || ln -s /planner ' + planner_link)
        argv = ['docker', 'run', '-d', '--name', CONTAINER, '--user', user, '--network', 'host', '--cpus', '2', '--memory', '6g',
                '-e', 'NVIDIA_VISIBLE_DEVICES=void', '-e', 'CUDA_VISIBLE_DEVICES=', '-e', 'PYTHONDONTWRITEBYTECODE=1',
                '-e', 'HF_HOME=/sandbox/cache',
                '-v', REMOTE + ':/sandbox',
                '-v', REFERENCES + ':/sglang-references',
                '-v', '/home/thao/comfyui/run:/comfy/mnt:ro',
                '-v', '/data/comfyui-109/basedir/custom_nodes/ComfyUI-MiniMaxH3-TimelineDirector:/planner:ro',
                '-w', '/comfy/mnt/ComfyUI', '--entrypoint', '/comfy/mnt/venv/bin/python3', image,
                'main.py', '--cpu', '--listen', '127.0.0.1', '--port', str(PORT), '--base-directory', '/sandbox',
                '--temp-directory', '/sandbox/temp', '--user-directory', '/sandbox/user', '--disable-auto-launch',
                '--disable-all-custom-nodes', '--whitelist-custom-nodes', 'FlowTimelineSGLangSandbox', 'ComfyUI-MiniMaxH3-TimelineDirector',
                '--disable-partner-nodes']
        container_id = command(client, shlex.join(argv)).strip()
        LOG.info('sandbox_started container=%s id=%s port=%s gpu=none', CONTAINER, container_id, PORT)
    finally:
        client.close()


if __name__ == '__main__':
    logging.basicConfig(level=logging.INFO, format='%(asctime)s %(levelname)s %(name)s %(funcName)s %(message)s')
    logging.getLogger('paramiko').setLevel(logging.WARNING)
    deploy()
