"""Barrido DETERMINISTA de la clase «caracter/escape pegado a un secreto» (U3-T06, ronda 3). No usa Hypothesis ni seed.
Los secretos se arman en ejecucion (ningun literal con forma de secreto)."""
import pytest

from agent_reporter import redact
from agent_reporter.errors import ReportRejected
from agent_reporter.redact import redact_text_counted

SECRET = "ZQ7" + "k9X2" + "mP4w"
BEAR = "Bear" + "er"
JWT = "ey" + "Jhb" + "GciOiJIUzI1NiJ9." + "eyJzdWIiOiIxMjM0NTYifQ." + "c2lnbmF0dXJl" + "9"
PW = "pass" + "word"
ALNUM_ = set("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_")


def _cps():
    yield from (c for c in range(0x10000) if not 0xD800 <= c <= 0xDFFF)
    yield from range(0x10000, 0x110000, 97)  # muestra astral determinista
    yield from (0x10000, 0x1F600, 0x10FFFF, 0xF0000, 0xE000)


def _leaks(prefix, text, secret):
    out, _ = redact_text_counted(prefix + text)
    return secret in out


def test_sweep_bearer_after_every_character():
    bad = [hex(c) for c in _cps() if chr(c) not in ALNUM_ and _leaks(chr(c), f"{BEAR} {SECRET}", SECRET)]
    assert bad == []


def test_sweep_jwt_after_every_character():
    tail = JWT.split(".")[2]
    bad = [hex(c) for c in _cps() if chr(c) not in ALNUM_ | {"-"} and _leaks(chr(c), JWT, tail)]
    assert bad == []


def test_sweep_url_credentials_after_every_character():
    url = "postgres://user:" + SECRET + "@db/x"
    bad = [hex(c) for c in _cps() if chr(c) not in ALNUM_ | {"+", ".", "-"} and _leaks(chr(c), url, SECRET)]
    assert bad == []


def test_sweep_assignment_after_every_character():
    bad = [hex(c) for c in _cps() if _leaks(chr(c), f"{PW}={SECRET}", SECRET)]
    assert bad == []


# separadores que cierran un valor crudo por diseno
_CIERRAN = set(" \t\r\n\x0b\x0c\"',;&") | {chr(c) for c in range(0x100) if chr(c).isspace()}  # \s de str: incluye \x1c-\x1f y \x85\xa0


@pytest.mark.parametrize("x", [chr(c) for c in range(0, 0x80) if chr(c) not in _CIERRAN and chr(c) not in "nrtbf"])
def test_sweep_backslash_escape_inside_assignment_value(x):
    """Todo `\\X` (letra ASCII o simbolo) dentro del valor: la cola no se filtra. Solo los cinco escapes JSON en
    minuscula (\\n \\r \\t \\b \\f) cortan el valor: decision documentada en redact.py."""
    for tail in (SECRET, "00001111"):
        out, _ = redact_text_counted(f"{PW}=AAAA\\{x}{tail}\n")
        assert tail not in out, repr(x)


def test_windows_path_and_double_backslash_values_are_redacted():
    for v in ("C:\\Users\\Tom\\pw1111", "AAAA\\BBBB1111", "AAAA\\\\BBBB1111", "AAAA\\NBBBB1111"):
        out, _ = redact_text_counted(f"{PW}={v}\n")
        assert out == f"{PW}=[REDACTED:secret_assignment]\n", v


def test_json_escape_in_value_is_documented_cut():
    out, _ = redact_text_counted(f"{PW}=AAAA\\nBBBB1111 x")
    assert out.startswith(f"{PW}=[REDACTED:secret_assignment]")


def test_sentinel_never_raises_and_is_not_word_char():
    s = "".join(map(chr, range(0x2400, 0x2500)))  # los 256 U+24xx (antes: StopIteration)
    out, _ = redact_text_counted(f"{s}\n{BEAR} {SECRET}\n")
    assert SECRET not in out and out.startswith(s)
    s2 = "".join(map(chr, range(0x2400, 0x2460)))  # faltaban solo U+2400..245F: el centinela caia en U+2460 (\w)
    assert SECRET not in redact_text_counted("\\n" + BEAR + " " + SECRET + s2)[0]
    for c in (redact._sentinel(""), redact._sentinel("".join(map(chr, range(0xE000, 0xE100))))):
        assert not c.isalnum() and c != "_" and not __import__("re").match(r"\w", c)


def test_sentinel_exhausted_fails_closed_with_typed_error():
    full = "".join(chr(c) for lo, hi in redact._SENT_RANGES for c in range(lo, hi + 1))
    with pytest.raises(ReportRejected):
        redact_text_counted(full)
