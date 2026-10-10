"""Fixtures de superficie para el FakeLLM (U3-T07, recorrido local).

Cada `*.json` del directorio: {"surface": {...}, "workflow": "...", "plan": {...}}. Registra el prompt
exacto que el planner construiria para (surface, workflow) y como respuesta el plan dado. Sin fixture
para un prompt, el FakeLLM falla cerrado como siempre.
"""
from __future__ import annotations

import json
from pathlib import Path

from ..prompt import build_prompt
from .fake_llm import FakeLLM


def register_surfaces(llm: FakeLLM, directory: str | Path) -> int:
    n = 0
    for p in sorted(Path(directory).glob("*.json")):
        doc = json.loads(p.read_text(encoding="utf-8"))
        llm.register(build_prompt(doc["surface"], doc["workflow"]), json.dumps(doc["plan"]))
        n += 1
    return n
