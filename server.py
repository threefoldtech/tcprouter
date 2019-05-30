from gevent import monkey; monkey.patch_all()
import logging
from gevent.server import StreamServer


logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)


class Receiver(object):
    """ Interface for a receiver - mimics Twisted's protocols
    """
    def __init__(self):
        self.socket = None
        self.address = None

    def connection_made(self, socket, address):
        self.socket = socket
        self.address = address

    def connection_lost(self):
        pass

    def line_received(self, line):
        pass

    def send_line(self, line):
        self.socket.sendall(line + b'\n')


class EchoReceiver(Receiver):
    """ A basic implementation of a receiver which echoes back every line it
    receives.
    """
    def line_received(self, line):
        self.send_line(line)


def Handler(receiver_class):
    """ A basic connection handler that applies a receiver object to each
    connection.
    """
    def handle(socket, address):
        logger.info('Client (%s) connected', address)

        receiver = receiver_class()
        receiver.connection_made(socket, address)

        try:
            f = socket.makefile()

            while True:
                line = f.readline().strip()
                if line == "":
                    break
                logger.info('Received line from client: %s', line)
                receiver.line_received(line.encode())
            logger.info('Client (%s) disconnected.', address)

        except Exception as e:
            logger.exception(e)
        finally:
            try:
                f.close()
                receiver.connection_lost()
            except:
                pass
    return handle


server = StreamServer(('0.0.0.0', 9092), Handler(EchoReceiver), keyfile='server.key', certfile='server.crt')
logger.info('Server running')
server.serve_forever()
