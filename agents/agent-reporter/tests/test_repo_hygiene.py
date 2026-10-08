import re

from support import ROOT


def test_no_secret_shaped_strings_in_repo_sources():
    pat = re.compile(r"AKIA[0-9A-Z]{16}|gh[pousr]_[A-Za-z0-9]{36}")
    for base in ("agents", "contracts"):
        for f in (ROOT / base).rglob("*"):
            if f.is_file() and not {".venv", "__pycache__", ".pytest_cache"} & set(f.parts):
                try:
                    txt = f.read_text(encoding="utf-8")
                except UnicodeDecodeError:
                    continue
                assert not pat.search(txt), f
