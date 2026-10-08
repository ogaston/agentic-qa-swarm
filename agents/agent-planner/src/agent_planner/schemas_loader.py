"""Carga de los esquemas de contrato empaquetados (copia byte a byte de contracts/plans/, vigilada por prueba)."""
from __future__ import annotations

import json
from functools import lru_cache
from pathlib import Path

from jsonschema import Draft202012Validator

_DIR = Path(__file__).resolve().parent / "schemas"


@lru_cache(maxsize=None)
def validator(name: str) -> Draft202012Validator:
    return Draft202012Validator(json.loads((_DIR / name).read_text(encoding="utf-8")))
