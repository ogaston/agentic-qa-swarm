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


def test_no_network_connect_ex_blocked():
    s = socket.socket()
    try:
        with pytest.raises(NetworkBlocked):
            s.connect_ex(("93.184.216.34", 80))
    finally:
        s.close()


def test_no_network_udp_send_blocked():
    s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    try:
        with pytest.raises(NetworkBlocked):
            s.sendto(b"x", ("93.184.216.34", 53))
        with pytest.raises(NetworkBlocked):
            s.sendmsg([b"x"], [], 0, ("93.184.216.34", 53))
    finally:
        s.close()


def test_no_network_gethostbyname_blocked():
    with pytest.raises(NetworkBlocked):
        socket.gethostbyname("example.com")
    with pytest.raises(NetworkBlocked):
        socket.gethostbyname_ex("example.com")


def test_no_network_loopback_bind_allowed():
    s = socket.socket()
    try:
        s.bind(("127.0.0.1", 0))
        s.listen(1)
        c = socket.create_connection(s.getsockname())
        c.close()
    finally:
        s.close()
