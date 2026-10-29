"""Isolated SGLang HTTP client. Submission is never retried automatically."""
import json
import logging
import time
from pathlib import Path
import requests

LOG = logging.getLogger(__name__)


class SGLangClient:
    def __init__(self, base):
        self.base = base.rstrip('/')

    def get(self, endpoint):
        response = requests.get(self.base + endpoint, timeout=20)
        response.raise_for_status()
        return response.json()

    def idle(self):
        listing = self.get('/v1/videos?limit=100')
        active = []
        for item in listing.get('data', []):
            if item.get('status') in ['queued', 'in_progress', 'running', 'processing']:
                fresh = self.get('/v1/videos/' + item['id'])
                if fresh.get('status') in ['queued', 'in_progress', 'running', 'processing']:
                    active.append(item['id'])
        if active:
            raise RuntimeError('SGLang has active tasks; sandbox will not submit: ' + ','.join(active))

    def wait_until_idle(self, timeout=14400):
        deadline = time.monotonic() + timeout
        while True:
            try:
                self.idle()
                return
            except (RuntimeError, requests.RequestException) as error:
                if time.monotonic() >= deadline:
                    raise TimeoutError('Timed out waiting for existing SGLang tasks; no test submitted') from error
                LOG.info('sglang_waiting_for_idle reason=%s', error)
                time.sleep(30)

    def generate(self, payload, directory, timeout=1800):
        directory = Path(directory)
        directory.mkdir(parents=True, exist_ok=True)
        record = directory / 'request.json'
        if record.exists():
            previous = json.loads(record.read_text())
            if previous.get('payload') != payload:
                raise RuntimeError('Existing run has different inputs; use a new run directory')
            if not previous.get('id'):
                raise RuntimeError('Submission status is unknown. Inspect server; do not resubmit automatically.')
            task_id = previous['id']
        else:
            self.wait_until_idle()
            record.write_text(json.dumps({'payload': payload, 'state': 'submitting'}, indent=2))
            response = requests.post(self.base + '/v1/videos', json=payload, timeout=180)
            if not response.ok:
                record.write_text(json.dumps({'payload': payload, 'state': 'rejected', 'error': response.text}, indent=2))
                raise RuntimeError('SGLang rejected request: ' + response.text[:1800])
            task = response.json()
            task_id = task['id']
            record.write_text(json.dumps({'payload': payload, 'id': task_id}, indent=2))
            LOG.info('sglang_submission_accepted task_id=%s', task_id)
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            task = self.get('/v1/videos/' + task_id)
            (directory / 'status.json').write_text(json.dumps(task, indent=2))
            if task.get('status') in ['failed', 'error', 'cancelled']:
                raise RuntimeError('SGLang task failed: ' + json.dumps(task.get('error')))
            if task.get('status') == 'completed':
                response = requests.get(self.base + '/v1/videos/' + task_id + '/content', timeout=180, stream=True)
                response.raise_for_status()
                target = directory / 'output.mp4'
                with target.with_suffix('.tmp').open('wb') as stream:
                    for chunk in response.iter_content(1024 * 1024):
                        stream.write(chunk)
                target.with_suffix('.tmp').replace(target)
                return target
            time.sleep(3)
        raise TimeoutError('Task remains on server; resume with the SAME run directory, not a new submission')
