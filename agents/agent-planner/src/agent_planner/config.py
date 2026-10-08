"""Cableado por entorno. Fail-closed: sin configuracion valida el proceso no arranca."""
from __future__ import annotations

import math
from dataclasses import dataclass
from typing import Mapping

from .errors import Limits
from .fakes.fake_llm import FakeLLM
from .llm import LLMClient


class ConfigError(Exception):
    pass


@dataclass(frozen=True)
class Config:
    llm: LLMClient
    limits: Limits
    host: str
    port: int


def _num(env: Mapping[str, str], key: str, default, cast):
    raw = env.get(key)
    if raw is None or raw == "":
        return default
    try:
        v = cast(raw)
    except ValueError:
        raise ConfigError(f"{key} no es un numero valido") from None
    if not (isinstance(v, int) or math.isfinite(v)) or v <= 0:
        raise ConfigError(f"{key} debe ser positivo")
    return v


def load_config(env: Mapping[str, str]) -> Config:
    limits = Limits(
        timeout_s=_num(env, "PLANNER_TIMEOUT_S", 25.0, float),
        max_input_tokens=_num(env, "PLANNER_MAX_INPUT_TOKENS", 8000, int),
        max_output_tokens=_num(env, "PLANNER_MAX_OUTPUT_TOKENS", 4000, int),
        max_flows=_num(env, "PLANNER_MAX_FLOWS", 10, int),
        max_steps=_num(env, "PLANNER_MAX_STEPS", 20, int),
    )
    port = _num(env, "PLANNER_PORT", 8080, int)
    provider = env.get("LLM_PROVIDER", "")
    if not provider:
        raise ConfigError("LLM_PROVIDER no configurado")
    if provider != "fake":
        raise ConfigError("proveedor no implementado")
    if env.get("PLANNER_ENV", "").strip().lower() in ("prod", "production"):
        raise ConfigError("LLM_PROVIDER=fake no se permite con PLANNER_ENV=prod")
    if env.get("PLANNER_ALLOW_FAKE") != "true":
        raise ConfigError("LLM_PROVIDER=fake requiere PLANNER_ALLOW_FAKE=true")
    return Config(FakeLLM(), limits, env.get("PLANNER_HOST", "0.0.0.0"), port)
