import socket

import pytest

from agent_eval import cli, report
from agent_eval.netguard import NetworkBlocked as GuardBlocked
from agent_eval.netguard import NetworkGuard
from conftest import NetworkBlocked
from support import DS, rule


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


def test_no_network_loopback_allowed():
    s = socket.socket()
    try:
        s.bind(("127.0.0.1", 0))
        s.listen(1)
        socket.create_connection(s.getsockname()).close()
    finally:
        s.close()


def test_no_network_guard_counts_blocks_and_restores():
    orig = socket.socket.connect
    g = NetworkGuard()
    with g:
        for host in ("93.184.216.34", "example.com", "127.0.0.1.evil.com"):
            with pytest.raises(GuardBlocked):
                socket.socket().connect((host, 80))
        with pytest.raises(GuardBlocked):
            socket.gethostbyname("example.com")
    assert len(g.attempts) == 4
    assert socket.socket.connect is orig


def test_no_network_guard_allows_loopback_and_unix():
    g = NetworkGuard()
    s = socket.socket()
    with g:
        s.bind(("127.0.0.1", 0)); s.listen(1)
        socket.create_connection(s.getsockname()).close()
    s.close()
    assert g.attempts == []


def test_no_network_full_run_has_zero_non_local_attempts_and_guard_active(tmp_path):
    rep = report.evaluate(DS)
    a6 = [c for r in rep["per_artifact"] for c in rule(r, "A6")]
    assert len(a6) == 12 and all(c["ok"] for c in a6)
    assert cli.main(["run", "--out", str(tmp_path / "o")]) == 0
