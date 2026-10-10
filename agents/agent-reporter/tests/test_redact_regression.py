"""Regresiones fijas halladas por test_pbt_redact_* (U3-T06). Los secretos se arman en tiempo de ejecucion."""
import json

from agent_reporter.redact import redact_secrets

AUTH = "Author" + "ization"
BEAR = "Bear" + "er"
JWT = "ey" + "Jhb" + "GciOiJIUzI1NiJ9." + "eyJzdWIiOiIxMjM0NTYifQ." + "c2lnbmF0dXJl" + "9"


def _out(s):
    return redact_secrets(s.encode()).decode()


def test_json_inside_json_string_authorization_is_redacted():
    # ejemplo reducido: {"msg": " {\"Authorization\": \"Basic 00000000\"} "} dejaba 00000000 en claro
    inner = json.dumps({AUTH: "Ba" + "sic 00000000"})
    out = _out(json.dumps({"msg": " " + inner + " ", "n": 1}))
    assert "00000000" not in out
    assert json.loads(out)["n"] == 1  # el JSON externo sigue siendo valido
    assert _out(out) == out


def test_json_inside_json_string_assignment_is_redacted():
    inner = json.dumps({"pass" + "word": "S3cr3tValue9"})
    out = _out(json.dumps({"msg": inner}))
    assert "S3cr3tValue9" not in out and json.loads(out)


def test_bearer_after_json_escaped_newline_is_redacted():
    # ejemplo reducido: {"msg": "\nBearer 00000000 "}: la `n` del escape \n pegaba la palabra y \b no cortaba
    out = _out(json.dumps({"msg": "\n" + BEAR + " 00000000 "}))
    assert "00000000" not in out


def test_jwt_after_json_escaped_newline_is_redacted():
    out = _out(json.dumps({"msg": "l1\nl2\t" + JWT}))
    assert JWT.split(".")[2] not in out


def test_second_raw_assignment_after_json_escaped_newlines_is_redacted():
    # F-02, ejemplo reducido (seed 21): el valor crudo se comia `\n\npassword:` y el segundo secreto sobrevivia
    k = "pass" + "word"
    out = _out(json.dumps({"msg": f" {k}=00000000\n\n{k}: 11111111 ", "n": 1}))
    assert "00000000" not in out and "11111111" not in out
    assert json.loads(out)["n"] == 1 and _out(out) == out


def test_bearer_and_jwt_after_unicode_control_and_ansi_are_redacted():
    # F-03: \uXXXX (termina en digito hex), no ASCII, control y ANSI crudo/escapado pegaban la palabra
    for pre in ["\x1b", "\x00", "\x7f", "é", "日", "\x1b[31m", "\x1b[1;31;4m", "\u001b[0m"]:
        for ea in (True, False):
            for body, needle in ((BEAR + " 00000000", "00000000"), (JWT, JWT.split(".")[2])):
                s = json.dumps({"m": pre + body}, ensure_ascii=ea)
                assert needle not in _out(s), (pre, ea, body[:6])
        assert "00000000" not in _out(pre + BEAR + " 00000000 x"), pre
        assert JWT.split(".")[2] not in _out(pre + JWT), pre


def test_url_credentials_after_ansi_color_are_redacted():
    # hallado por el barrido de seeds (101): el `m` de ESC[31m pegaba el esquema y el lookbehind lo descartaba
    url = "postgres" + "://0:00000000@db.internal:5432/app"
    for pre in ["\x1b[31m", "\x1b[1;31;4m", "\\n", "\\u00e9"]:
        assert "00000000" not in _out("x " + pre + url + " y"), pre


def test_bearer_after_non_ascii_letter_like_characters_is_redacted():
    # ronda 3 F-01: `\b` Unicode trataba ª µ ² ³ ¹ º ¼ ½ ¾ como palabra y `ªBearer 0000...` filtraba
    for ch in "\u00aa\u00b5\u00b2\u00b3\u00b9\u00ba\u00bc\u00bd\u00be\u2460\u017f":
        assert "00000000" not in _out(f"{ch}{BEAR} 00000000")


def test_assignment_values_with_uppercase_or_double_backslash_are_redacted():
    # ronda 3 F-02: `[nrtbf]` bajo (?i) cortaba el valor en \B \T \N \\ : password=AAAA\BBBB1111 filtraba la cola
    pw = "pass" + "word"
    for v in ("AAAA\\BBBB1111", "C:\\Users\\Tom\\pw1111", "AAAA\\NBBBB1111", "AAAA\\\\BBBB1111"):
        assert _out(f"{pw}={v}\n") == f"{pw}=[REDACTED:secret_assignment]\n"


def test_all_24xx_characters_in_text_do_not_crash_and_bearer_after_newline_is_redacted():
    # ronda 3 F-03: StopIteration con los 256 U+24xx; con U+2400..245F ausentes el centinela era U+2460 (\w)
    from agent_reporter.redact import redact_text_counted

    allc = "".join(map(chr, range(0x2400, 0x2500)))
    out, _ = redact_text_counted(allc + "\\n" + BEAR + " ZZZZ9999")
    assert "ZZZZ9999" not in out
