import ipaddress
import socket

import pytest


class NetworkBlocked(Exception):
    """Se intento abrir una conexion a un destino que no es loopback."""


def _is_loopback(host) -> bool:
    if isinstance(host, bytes):
        host = host.decode()
    if host in ("localhost", ""):
        return True
    try:
        return ipaddress.ip_address(host.split("%")[0]).is_loopback
    except ValueError:
        return False


@pytest.fixture(autouse=True)
def _block_non_local_network(monkeypatch):
    real_connect = socket.socket.connect
    real_getaddrinfo = socket.getaddrinfo

    def connect(self, address):
        if isinstance(address, tuple) and not _is_loopback(address[0]):
            raise NetworkBlocked(f"destino no local: {address[0]}")
        return real_connect(self, address)

    def getaddrinfo(host, *a, **k):
        if host is not None and not _is_loopback(host):
            raise NetworkBlocked(f"resolucion no local: {host}")
        return real_getaddrinfo(host, *a, **k)

    monkeypatch.setattr(socket.socket, "connect", connect)
    monkeypatch.setattr(socket, "getaddrinfo", getaddrinfo)
