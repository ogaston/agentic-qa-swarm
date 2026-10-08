"""Umbrales de aceptacion (specs/prd.md §10). Fracciones exactas: sin error de coma flotante en la frontera."""
from __future__ import annotations

from fractions import Fraction

# metrica -> (operador, valor, fuente)
THRESHOLDS: dict[str, tuple[str, Fraction, str]] = {
    "factualidad": (">", Fraction(4, 5), "propuesto: misma barra que la precision (prd.md §11, causa raiz = real conocida)"),
    "precision": (">", Fraction(4, 5), "prd.md §10: precision del post-mortem > 80 %"),
    "ruido": ("<", Fraction(1, 5), "prd.md §10: ruido < 20 % (saludable)"),
    "adherencia": ("==", Fraction(1), "propuesto (C-89): cumplimiento binario de reglas duras de U3 = 100 %"),
}

METRIC_ORDER = ("factualidad", "precision", "ruido", "adherencia")


def is_met(metric: str, value: Fraction) -> bool:
    op, bound, _ = THRESHOLDS[metric]
    if op == ">":
        return value > bound
    if op == "<":
        return value < bound
    if op == "==":
        return value == bound
    raise ValueError(f"operador desconocido: {op}")
