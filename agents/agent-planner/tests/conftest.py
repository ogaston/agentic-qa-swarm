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
    real_connect_ex = socket.socket.connect_ex
    real_sendto = socket.socket.sendto
    real_sendmsg = socket.socket.sendmsg
    real_getaddrinfo = socket.getaddrinfo

    def check(address):
        if isinstance(address, tuple) and address and not _is_loopback(address[0]):
            raise NetworkBlocked(f"destino no local: {address[0]}")

    def connect(self, address):
        check(address)
        return real_connect(self, address)

    def connect_ex(self, address):
        check(address)
        return real_connect_ex(self, address)

    def sendto(self, data, *args):
        check(args[-1])
        return real_sendto(self, data, *args)

    def sendmsg(self, buffers, *args):
        if len(args) >= 3 and args[2] is not None:
            check(args[2])
        return real_sendmsg(self, buffers, *args)

    def getaddrinfo(host, *a, **k):
        if host is not None and not _is_loopback(host):
            raise NetworkBlocked(f"resolucion no local: {host}")
        return real_getaddrinfo(host, *a, **k)

    def gethostbyname(host):
        if not _is_loopback(host):
            raise NetworkBlocked(f"resolucion no local: {host}")
        return real_gethostbyname(host)

    def gethostbyname_ex(host):
        if not _is_loopback(host):
            raise NetworkBlocked(f"resolucion no local: {host}")
        return real_gethostbyname_ex(host)

    real_gethostbyname = socket.gethostbyname
    real_gethostbyname_ex = socket.gethostbyname_ex
    monkeypatch.setattr(socket.socket, "connect", connect)
    monkeypatch.setattr(socket.socket, "connect_ex", connect_ex)
    monkeypatch.setattr(socket.socket, "sendto", sendto)
    monkeypatch.setattr(socket.socket, "sendmsg", sendmsg)
    monkeypatch.setattr(socket, "getaddrinfo", getaddrinfo)
    monkeypatch.setattr(socket, "gethostbyname", gethostbyname)
    monkeypatch.setattr(socket, "gethostbyname_ex", gethostbyname_ex)
