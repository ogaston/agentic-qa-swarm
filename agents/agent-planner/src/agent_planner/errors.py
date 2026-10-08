"""Errores tipados y limites del planner."""
from __future__ import annotations

from dataclasses import dataclass


class PlannerError(Exception):
    """Base."""


class NoSurface(PlannerError):
    """Superficie invalida o vacia: nunca se inventa un plan."""


class InvalidRequest(PlannerError):
    """Peticion mal formada (workflow invalido, cuerpo incorrecto)."""


class PlanRejected(PlannerError):
    """La salida del modelo no cumple el contrato. `reason` es un codigo fijo, sin contenido externo."""

    def __init__(self, reason: str) -> None:
        super().__init__(reason)
        self.reason = reason


class BudgetExceeded(PlannerError):
    """Tope duro excedido. kind: input | output | time."""

    def __init__(self, kind: str) -> None:
        super().__init__(kind)
        self.kind = kind


@dataclass(frozen=True)
class Limits:
    timeout_s: float = 25.0
    max_input_tokens: int = 8000
    max_output_tokens: int = 4000
    max_flows: int = 10
    max_steps: int = 20
