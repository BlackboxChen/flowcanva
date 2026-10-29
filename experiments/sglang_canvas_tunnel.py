"""Loopback-only SSH access to the isolated ComfyUI sandbox for FlowCanva."""
import logging
import select
import socketserver
from deploy_timeline_sglang import connect

LOG = logging.getLogger(__name__)


class Relay(socketserver.BaseRequestHandler):
    def handle(self):
        channel = None
        try:
            channel = self.server.ssh.get_transport().open_channel(
                'direct-tcpip', ('127.0.0.1', 18299), self.client_address)
            while True:
                readable, _, _ = select.select([self.request, channel], [], [], 30)
                for source in readable:
                    data = source.recv(65536)
                    if not data:
                        return
                    (channel if source is self.request else self.request).sendall(data)
        except Exception:
            LOG.exception('sandbox_relay_failed')
        finally:
            if channel:
                channel.close()


def main():
    logging.basicConfig(level=logging.INFO, format='%(asctime)s %(module)s %(funcName)s %(message)s')
    ssh = connect()
    ssh.get_transport().set_keepalive(30)
    with socketserver.ThreadingTCPServer(('127.0.0.1', 18299), Relay) as server:
        server.daemon_threads = True
        server.ssh = ssh
        LOG.info('sandbox_tunnel_listening address=127.0.0.1:18299')
        try:
            server.serve_forever()
        finally:
            ssh.close()


if __name__ == '__main__':
    main()
