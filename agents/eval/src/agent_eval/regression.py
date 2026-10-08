"""Bucle de regresion: un falso positivo marcado por el usuario pasa a ser una entrada del dataset."""
from __future__ import annotations

import json
import os
import re
import shutil
import sys
import tempfile
from pathlib import Path

from .dataset import (
    ID_RE,
    REGRESSION_KIND,
    DatasetError,
    expected_validator,
    load_dataset,
    load_expected,
    read_json,
)

LABELS = ("sin-hallazgos", "inconcluso")
FINDING_RE = re.compile(r"[A-Za-z0-9._-]{1,128}")


def _copy_tree(src: Path, dst: Path) -> None:
    """Copia sin seguir enlaces: un symlink en la evidencia no puede arrastrar archivos ajenos al dataset."""
    for p in sorted(src.rglob("*")):
        if p.is_symlink():
            raise DatasetError("enlace simbolico en la evidencia")
    shutil.copytree(src, dst, symlinks=True)


def add_regression(dataset: Path | str, artifact_id: str, finding_id: str, label: str) -> Path:
    ds = Path(dataset)
    if label not in LABELS:
        raise DatasetError("label debe ser sin-hallazgos o inconcluso")
    if not isinstance(artifact_id, str) or ID_RE.fullmatch(artifact_id) is None:
        raise DatasetError("artifact invalido")
    if not isinstance(finding_id, str) or FINDING_RE.fullmatch(finding_id) is None:
        raise DatasetError("finding invalido")
    arts = {a.id: a for a in load_dataset(ds)}
    src = arts.get(artifact_id)
    if src is None:
        raise DatasetError("artifact inexistente en el dataset")
    if not (src.dir / "evidence-uris.json").is_file() or not (src.dir / "reporter.response.json").is_file():
        raise DatasetError("el artefacto no tiene evidencia ni respuesta de reporter que regresionar")
    exp = load_expected(src, expected_validator(ds))
    resp = read_json(src.dir / "reporter.response.json")
    known = {f.get("flow_id") for f in resp.get("findings", []) if isinstance(f, dict)}
    if finding_id not in known:
        print(f"aviso: el hallazgo {finding_id!r} no figura en la respuesta canned de {artifact_id}", file=sys.stderr)
    meta = read_json(src.dir / "meta.json")
    reg_root = ds / "regressions"
    reg_root.mkdir(exist_ok=True)
    tmp = Path(tempfile.mkdtemp(prefix=".tmp-", dir=reg_root))
    try:
        for name in ("surface.json", "evidence-uris.json", "reporter.response.json", "planner.response.json"):
            if (src.dir / name).is_file():
                shutil.copyfile(src.dir / name, tmp / name)
        if (src.dir / "evidence").is_dir():
            _copy_tree(src.dir / "evidence", tmp / "evidence")
        expected = {"planner": exp["planner"], "reporter": {"verdict": label, "root_cause": None}}
        n = 1
        while True:
            rid = f"{artifact_id}-fp-{n}"
            (tmp / "meta.json").write_text(
                json.dumps(
                    {
                        "id": rid,
                        "domain": meta.get("domain"),
                        "kind": REGRESSION_KIND,
                        "description": f"Falso positivo marcado por un usuario sobre {artifact_id}.",
                        "source_artifact": artifact_id,
                        "source_finding": finding_id,
                    },
                    indent=2, sort_keys=True,
                ) + "\n",
                encoding="utf-8",
            )
            (tmp / "expected.json").write_text(json.dumps(expected, indent=2, sort_keys=True) + "\n", encoding="utf-8")
            target = reg_root / rid
            try:
                os.mkdir(target)  # reserva atomica del nombre
            except FileExistsError:
                n += 1
                continue
            os.rmdir(target)
            os.rename(tmp, target)
            return target
    except BaseException:
        shutil.rmtree(tmp, ignore_errors=True)
        raise
