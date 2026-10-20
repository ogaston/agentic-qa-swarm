"""Cableado por entorno. Fail-closed: sin configuracion valida el proceso no arranca."""
from __future__ import annotations

import math
from dataclasses import dataclass
from typing import Mapping

from .errors import Limits
from .fakes.fake_llm import FakeLLM
from .fakes.surfaces import register_surfaces
from .llm import LLMClient
from .llm_http import HttpLLM


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
    if provider == "http":
        llm = _http_llm(env)
    elif provider == "fake":
        llm = _fake_llm(env)
    else:
        raise ConfigError("proveedor no implementado")
    return Config(llm, limits, env.get("PLANNER_HOST", "0.0.0.0"), port)


def _http_llm(env: Mapping[str, str]) -> HttpLLM:
    base, model, key_file = env.get("LLM_BASE_URL", ""), env.get("LLM_MODEL", ""), env.get("LLM_API_KEY_FILE", "")
    if not base or not model or not key_file:
        raise ConfigError("LLM_PROVIDER=http requiere LLM_BASE_URL, LLM_MODEL y LLM_API_KEY_FILE")
    # U8-T09 (ronda 4): opt-in explícito a http hacia UN host privado (kind). Por defecto, no.
    insecure_host = _insecure_http_host(env)
    try:
        return HttpLLM(base, model, key_file, insecure_http_host=insecure_host)
    except (ValueError, OSError):
        raise ConfigError("configuracion LLM_PROVIDER=http invalida") from None


def _insecure_http_host(env: Mapping[str, str]) -> str | None:
    """Solo con LLM_ALLOW_INSECURE_HTTP=1 (exacto) y LLM_INSECURE_HTTP_HOST; si no, None."""
    if env.get("LLM_ALLOW_INSECURE_HTTP", "") != "1":
        return None
    return env.get("LLM_INSECURE_HTTP_HOST", "") or None


def _fake_llm(env: Mapping[str, str]) -> FakeLLM:
    if env.get("PLANNER_ENV", "").strip().lower() in ("prod", "production"):
        raise ConfigError("LLM_PROVIDER=fake no se permite con PLANNER_ENV=prod")
    if env.get("PLANNER_ALLOW_FAKE") != "true":
        raise ConfigError("LLM_PROVIDER=fake requiere PLANNER_ALLOW_FAKE=true")
    llm = FakeLLM()
    surfaces = env.get("U3_FAKE_SURFACES_DIR", "")
    if surfaces:
        try:
            register_surfaces(llm, surfaces)
        except (OSError, ValueError, KeyError, TypeError):
            raise ConfigError("U3_FAKE_SURFACES_DIR invalido") from None
    return llm
