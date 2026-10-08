"""Carga del dataset (manifest + regressions/*). Ids y rutas se tratan como no confiables."""
from __future__ import annotations

import json
import re
from dataclasses import dataclass
from pathlib import Path

from jsonschema import Draft202012Validator

DEFAULT_DATASET = Path(__file__).resolve().parents[3] / "dataset"
ID_RE = re.compile(r"[a-z0-9]+(?:-[a-z0-9]+)*")
KINDS = ("golden", "bug-sembrado", "trampa-esquema", "no-arranca")
REGRESSION_KIND = "regresion-fp"
LABELLED_KINDS = ("golden", "bug-sembrado", "trampa-esquema", REGRESSION_KIND)


class DatasetError(Exception):
    pass


@dataclass(frozen=True)
class Artifact:
    id: str
    kind: str
    dir: Path

    @property
    def is_regression(self) -> bool:
        return self.kind == REGRESSION_KIND


def read_json(p: Path):
    try:
        return json.loads(p.read_text(encoding="utf-8"))
    except (OSError, ValueError) as e:
        raise DatasetError(f"no se pudo leer {p.name}: {type(e).__name__}") from None


def _check_id(i, what: str) -> str:
    if not isinstance(i, str) or ID_RE.fullmatch(i) is None:
        raise DatasetError(f"id invalido en {what}")
    return i


def _check_dir(d: Path, root: Path) -> None:
    if d.is_symlink() or not d.is_dir():
        raise DatasetError(f"{d.name}: no es un directorio regular")
    if d.resolve().parent != root.resolve():
        raise DatasetError(f"{d.name}: fuera del dataset")


def load_dataset(ds: Path | str) -> list[Artifact]:
    ds = Path(ds)
    manifest = read_json(ds / "manifest.json")
    entries = manifest.get("artifacts") if isinstance(manifest, dict) else None
    if not isinstance(entries, list):
        raise DatasetError("manifest sin lista artifacts")
    out: list[Artifact] = []
    seen: set[str] = set()
    root = ds / "artifacts"
    for e in entries:
        if not isinstance(e, dict):
            raise DatasetError("entrada de manifest invalida")
        aid, kind = _check_id(e.get("id"), "manifest"), e.get("kind")
        if kind not in KINDS:
            raise DatasetError(f"{aid}: kind invalido")
        if aid in seen:
            raise DatasetError(f"{aid}: duplicado")
        seen.add(aid)
        d = root / aid
        _check_dir(d, root)
        out.append(Artifact(aid, kind, d))
    reg = ds / "regressions"
    if reg.is_dir():
        for d in sorted(reg.iterdir(), key=lambda p: p.name):
            if d.name.startswith("."):  # temporales de add-regression
                continue
            aid = _check_id(d.name, "regressions")
            _check_dir(d, reg)
            meta = read_json(d / "meta.json")
            if not isinstance(meta, dict) or meta.get("kind") != REGRESSION_KIND or meta.get("id") != aid:
                raise DatasetError(f"{aid}: meta de regresion invalida")
            if aid in seen:
                raise DatasetError(f"{aid}: duplicado")
            seen.add(aid)
            out.append(Artifact(aid, REGRESSION_KIND, d))
    return out


def expected_validator(ds: Path) -> Draft202012Validator:
    return Draft202012Validator(read_json(ds / "schema" / "expected.schema.json"))


def load_expected(art: Artifact, validator: Draft202012Validator) -> dict:
    exp = read_json(art.dir / "expected.json")
    if next(validator.iter_errors(exp), None) is not None:
        raise DatasetError(f"{art.id}: expected.json no cumple el esquema")
    return exp
