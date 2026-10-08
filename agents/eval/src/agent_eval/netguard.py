"""Guarda sin red: bloquea y CUENTA los intentos de conexion no locales durante la corrida."""
from __future__ import annotations

import ipaddress
import socket


class NetworkBlocked(Exception):
    pass


def is_loopback(host) -> bool:
    if isinstance(host, bytes):
        host = host.decode("latin-1")
    if host in ("localhost", ""):
        return True
    try:
        return ipaddress.ip_address(str(host).split("%")[0]).is_loopback
    except ValueError:
        return False


class NetworkGuard:
    def __init__(self) -> None:
        self.attempts: list[str] = []
        self._saved: list[tuple[object, str, object]] = []

    def _deny(self, host) -> None:
        self.attempts.append(str(host))
        raise NetworkBlocked(f"destino no local: {host}")

    def _check_addr(self, address) -> None:
        if isinstance(address, tuple) and address and not is_loopback(address[0]):
            self._deny(address[0])

    def _check_host(self, host) -> None:
        if host is not None and not is_loopback(host):
            self._deny(host)

    def __enter__(self) -> "NetworkGuard":
        g = self
        real = {n: getattr(socket.socket, n) for n in ("connect", "connect_ex", "sendto", "sendmsg")}
        real_gai, real_ghbn, real_ghbne = socket.getaddrinfo, socket.gethostbyname, socket.gethostbyname_ex

        def connect(s, address):
            g._check_addr(address)
            return real["connect"](s, address)

        def connect_ex(s, address):
            g._check_addr(address)
            return real["connect_ex"](s, address)

        def sendto(s, data, *args):
            g._check_addr(args[-1])
            return real["sendto"](s, data, *args)

        def sendmsg(s, buffers, *args):
            if len(args) >= 3 and args[2] is not None:
                g._check_addr(args[2])
            return real["sendmsg"](s, buffers, *args)

        def gai(host, *a, **k):
            g._check_host(host)
            return real_gai(host, *a, **k)

        def ghbn(host):
            g._check_host(host)
            return real_ghbn(host)

        def ghbne(host):
            g._check_host(host)
            return real_ghbne(host)

        for obj, name, fn in (
            (socket.socket, "connect", connect), (socket.socket, "connect_ex", connect_ex),
            (socket.socket, "sendto", sendto), (socket.socket, "sendmsg", sendmsg),
            (socket, "getaddrinfo", gai), (socket, "gethostbyname", ghbn), (socket, "gethostbyname_ex", ghbne),
        ):
            self._saved.append((obj, name, getattr(obj, name)))
            setattr(obj, name, fn)
        return self

    def __exit__(self, *exc) -> None:
        for obj, name, orig in reversed(self._saved):
            setattr(obj, name, orig)
        self._saved.clear()
