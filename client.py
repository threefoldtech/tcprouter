import time
from gevent import ssl, socket

s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)

# Require a certificate from the server. We used a self-signed certificate
# so here ca_certs must be the server certificate itself.
ssl_sock = ssl.wrap_socket(s,
                           ca_certs="server.crt",
                           cert_reqs=ssl.CERT_REQUIRED)

ssl_sock.connect(('localhost', 5500))
# ssl_sock.connect(('localhost', 9092))

ssl_sock.sendall(b'login superadmin password\n')

while True:
    time.sleep(5)
    ssl_sock.sendall(b'ping null\n')
    print(ssl_sock.recv(4096))

ssl_sock.close()
