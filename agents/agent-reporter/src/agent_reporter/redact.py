"""redact_secrets: reemplazo de secretos por [REDACTED:<tipo>]. Puro, idempotente, sin red.

Mejor esfuerzo: una lista de patrones no detecta cualquier secreto.
"""
from __future__ import annotations

import re
from collections import Counter

_KEYWORDS = r"(?:password|passwd|secret|token|api[_-]?key|access[_-]?key)"

_PRIVATE_KEY = re.compile(
    r"-----BEGIN [A-Z ]*PRIVATE KEY-----.*?(?:-----END [A-Z ]*PRIVATE KEY-----|\Z)", re.S
)
_URL_CRED = re.compile(r"(?P<scheme>[A-Za-z][A-Za-z0-9+.-]*://)[^/\s@:]+:[^/\s@]*@")
_AUTH_HEADER = re.compile(r"(?i)(?P<k>authorization[ \t]*:)[ \t]*[^\r\n]*")
_BEARER = re.compile(r"(?i)\b(?P<k>bearer)[ \t]+[A-Za-z0-9._~+/=-]{4,}")
_JWT = re.compile(r"eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+")
_AWS = re.compile(r"AKIA[0-9A-Z]{16}")
_GH = re.compile(r"gh[pousr]_[A-Za-z0-9]{20,}")
_ASSIGN = re.compile(
    r"""(?ix)
    (?P<key>["']?[\w.-]{0,32}""" + _KEYWORDS + r"""[\w-]{0,32}["']?)
    (?P<sep>[ \t]*[=:][ \t]*)
    (?:"(?P<dq>[^"\r\n]*)"|'(?P<sq>[^'\r\n]*)'|(?P<raw>[^\s"',;&]+))
    """
)


def _tag(kind: str) -> str:
    return f"[REDACTED:{kind}]"


def _redact_str(s: str, counts: Counter) -> str:
    def sub_simple(rx, kind, text):
        def f(m):
            counts[kind] += 1
            return _tag(kind)

        return rx.sub(f, text)

    s = sub_simple(_PRIVATE_KEY, "private_key", s)

    def url(m):
        counts["url_credentials"] += 1
        return m.group("scheme") + _tag("url_credentials")

    s = _URL_CRED.sub(url, s)

    def auth(m):
        counts["authorization"] += 1
        return f"{m.group('k')} {_tag('authorization')}"

    s = _AUTH_HEADER.sub(auth, s)

    def bearer(m):
        counts["bearer"] += 1
        return f"{m.group('k')} {_tag('bearer')}"

    s = _BEARER.sub(bearer, s)
    s = sub_simple(_JWT, "jwt", s)
    s = sub_simple(_AWS, "aws_access_key_id", s)
    s = sub_simple(_GH, "github_token", s)

    def assign(m):
        val = m.group("dq") if m.group("dq") is not None else (
            m.group("sq") if m.group("sq") is not None else m.group("raw")
        )
        if val.startswith("[REDACTED:") or val == "":
            return m.group(0)
        counts["secret_assignment"] += 1
        if m.group("dq") is not None:
            return f'{m.group("key")}{m.group("sep")}"{_tag("secret_assignment")}"'
        if m.group("sq") is not None:
            return f"{m.group('key')}{m.group('sep')}'{_tag('secret_assignment')}'"
        return f"{m.group('key')}{m.group('sep')}{_tag('secret_assignment')}"

    return _ASSIGN.sub(assign, s)


def redact_text_counted(text: str) -> tuple[str, Counter]:
    counts: Counter = Counter()
    return _redact_str(text, counts), counts


def redact_secrets_counted(content: bytes) -> tuple[bytes, Counter]:
    """latin-1 hace el viaje bytes<->str sin perdida: la entrada no UTF-8 no lanza."""
    counts: Counter = Counter()
    out = _redact_str(content.decode("latin-1"), counts)
    return out.encode("latin-1"), counts


def redact_secrets(content: bytes) -> bytes:
    return redact_secrets_counted(content)[0]
