import socket

import pytest

from conftest import NetworkBlocked


def test_no_network_external_connect_blocked():
    with pytest.raises(NetworkBlocked):
        socket.create_connection(("example.com", 443))


def test_no_network_external_ip_blocked():
    s = socket.socket()
    try:
        with pytest.raises(NetworkBlocked):
            s.connect(("93.184.216.34", 80))
    finally:
        s.close()


def test_no_network_loopback_bind_allowed():
    s = socket.socket()
    try:
        s.bind(("127.0.0.1", 0))
        s.listen(1)
        c = socket.create_connection(s.getsockname())
        c.close()
    finally:
        s.close()
