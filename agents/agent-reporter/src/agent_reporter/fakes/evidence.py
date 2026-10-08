"""Lector de evidencia sobre un directorio, y fake que falla por URI."""
from __future__ import annotations

from pathlib import Path
from urllib.parse import urlsplit

from agent_reporter.errors import EvidenceUnavailable


def _rel(uri: str) -> str:
    p = urlsplit(uri)
    parts = [x for x in p.path.split("/") if x]
    if any(x in (".", "..") for x in parts):
        raise EvidenceUnavailable("uri invalida")
    return "/".join(parts)


class DirEvidenceReader:
    """Mapea s3://bucket/a/b/c a <root>/a/b/c; si no existe, al plano <root>/a__b__c."""

    def __init__(self, root: Path | str, fail_uris: set[str] | None = None) -> None:
        self.root = Path(root)
        self.fail_uris = fail_uris or set()
        self.reads: list[str] = []

    def get(self, uri: str) -> bytes:
        self.reads.append(uri)
        if uri in self.fail_uris:
            raise EvidenceUnavailable("lectura programada para fallar")
        rel = _rel(uri)
        for cand in (self.root / rel, self.root / rel.replace("/", "__")):
            if cand.is_file():
                return cand.read_bytes()
        raise EvidenceUnavailable("objeto inexistente")
