"""redact_secrets: reemplazo de secretos por [REDACTED:<tipo>]. Puro, idempotente, sin red.

Mejor esfuerzo: una lista de patrones no detecta cualquier secreto.
"""
from __future__ import annotations

import re
from collections import Counter

from .errors import ReportRejected

_KEYWORDS = r"(?:password|passwd|secret|token|api[_-]?key|access[_-]?key)"

_PRIVATE_KEY = re.compile(
    r"-----BEGIN [A-Z ]*PRIVATE KEY-----.*?(?:-----END [A-Z ]*PRIVATE KEY-----|\Z)", re.S
)
# Todos los patrones arrancan en un limite (lookbehind) o llevan cotas {0,N}: el costo es lineal
# (una cadena de 64 KiB de letras, "token", "eyJ"... no dispara backtracking cuadratico).
# Limite izquierdo de bearer/JWT/URL: lo que deja una letra/digito pegado a la palabra sin ser parte de ella (un escape JSON
# \n \t \uXXXX, o el final de una secuencia ANSI ESC[..m) no cuenta como caracter de palabra. En vez de ~25 lookbehinds en
# cada patron (10x mas lento), un pre-paso marca el final de esos escapes con un centinela (simbolo Unicode, no \w) que ningun patron
# trata como palabra, y se retira al final. Los patrones conservan sus limites simples y su costo lineal.
_ESC_END = re.compile(r"\\[nrtbf]|\\u001[bB]\[[0-9;]{0,11}m|\\u[0-9a-fA-F]{4}|\x1b\[[0-9;]{0,11}m")
_URL_CRED = re.compile(
    r"(?<![A-Za-z0-9+.-])(?P<scheme>[A-Za-z][A-Za-z0-9+.-]{0,31}://)[^/\s@:]*:[^/\s]*@"
)
# `bq`: valor entre comillas ESCAPADAS (\"v\"), el JSON serializado dentro de un campo de cadena de otro JSON.
_VALUE = r"""(?:\\"(?P<bq>[^"\\\r\n]*)\\"|"(?P<dq>(?:[^"\\\r\n]|\\.)*)"|'(?P<sq>(?:[^'\\\r\n]|\\.)*)'|(?P<uq>["'][^\r\n]+)|(?P<raw>%s))"""
_AUTH_HEADER = re.compile(
    r"""(?i)(?P<k>authorization(?:\\?["'])?[ \t]*[=:][ \t]*)""" + _VALUE % r"[^\r\n]+"
)
# Limite ASCII explicito (no \b, que en str es Unicode: "ªBearer" contaria como palabra) y fuera de IGNORECASE
# (bajo (?i) [a-z] casaria tambien U+017F y U+212A).
_BEARER = re.compile(r"(?i)(?-i:(?<![A-Za-z0-9_]))(?P<k>bearer)[ \t]+[A-Za-z0-9._~+/=-]{4,}")
_JWT = re.compile(
    r"(?<![A-Za-z0-9_-])eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+"
)
_AWS = re.compile(r"AKIA[0-9A-Z]{16}")
_GH = re.compile(r"gh[pousr]_[A-Za-z0-9]{20,}")
_ASSIGN = re.compile(
    r"(?i)(?P<key>" + _KEYWORDS + r"""[\w-]{0,32}(?:\\?["'])?)(?P<sep>[ \t]*[=:][ \t]*)"""
    + _VALUE % r"""(?:\\(?!(?-i:[nrtbf]))|[^\s"',;&\\])+"""
    # La barra de un escape JSON en MINUSCULA (\n \t \r \b \f) cierra el valor; \B \T \N \\ y C:\Users\Tom\pw no.
    # Decision documentada: un \n/\t literal en minuscula dentro de un valor (p. ej. C:\temp\new) es un corte aceptado:
    # es indistinguible de un escape JSON y cerrar ahi evita tragar el resto de la cadena JSON.
)


def _tag(kind: str) -> str:
    return f"[REDACTED:{kind}]"


def _repl_value(m, head: str, kind: str, counts: Counter) -> str:
    if m.group("uq") is not None:  # comilla sin cerrar (log truncado): se redacta hasta el fin de linea
        uq = m.group("uq")
        if uq[1:].startswith("[REDACTED:"):
            return m.group(0)
        counts[kind] += 1
        return f"{head}{uq[0]}{_tag(kind)}"
    if m.group("bq") is not None:  # comillas escapadas: se conserva la forma \"<etiqueta>\"
        if m.group("bq").startswith("[REDACTED:") or m.group("bq") == "":
            return m.group(0)
        counts[kind] += 1
        return f'{head}\\"{_tag(kind)}\\"'
    q = "\"" if m.group("dq") is not None else ("'" if m.group("sq") is not None else "")
    val = next(v for v in (m.group("dq"), m.group("sq"), m.group("raw")) if v is not None)
    if val.startswith("[REDACTED:") or val == "":
        return m.group(0)
    counts[kind] += 1
    return f"{head}{q}{_tag(kind)}{q}"


def _sentinel(s: str) -> str:
    """Centinela NO-\\w ausente de la entrada: area de uso privado (categoria Co, nunca palabra), BMP y luego plano 15/16.
    Nunca lanza StopIteration: si la entrada contuviera los 137 468 caracteres, falla cerrado con ReportRejected."""
    used = set(s)
    for lo, hi in _SENT_RANGES:
        for cp in range(lo, hi + 1):
            if chr(cp) not in used:
                return chr(cp)
    raise ReportRejected("redaccion_sin_centinela")


_SENT_RANGES = ((0xE000, 0xF8FF), (0xF0000, 0xFFFFD), (0x100000, 0x10FFFD))


def _redact_str(s: str, counts: Counter) -> str:
    sent = _sentinel(s)
    s = _ESC_END.sub(lambda m: m.group(0) + sent, s)
    return _redact_marked(s, counts).replace(sent, "")


def _redact_marked(s: str, counts: Counter) -> str:
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
        r = _repl_value(m, m.group("k"), "authorization", counts)
        return r

    s = _AUTH_HEADER.sub(auth, s)

    def bearer(m):
        counts["bearer"] += 1
        return f"{m.group('k')} {_tag('bearer')}"

    s = _BEARER.sub(bearer, s)
    s = sub_simple(_JWT, "jwt", s)
    s = sub_simple(_AWS, "aws_access_key_id", s)
    s = sub_simple(_GH, "github_token", s)

    def assign(m):
        return _repl_value(m, m.group("key") + m.group("sep"), "secret_assignment", counts)

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
