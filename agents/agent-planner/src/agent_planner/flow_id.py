"""flow_id: normalizacion determinista del planner (U11-T04).

El contrato exige `^[a-z0-9]([-a-z0-9]{0,33}[a-z0-9])?$` (1 a 35 caracteres) porque el nombre del Job
runner es runner-<run_id>-<flow_id> y debe caber en 63. El modelo puede producir cualquier cadena; esta
funcion la lleva al patron sin perder la identidad: minusculas, [^a-z0-9] -> '-', guiones colapsados y
recortados, vacio -> 'flujo', y si pasa de 35 se corta a 30 y se anade '-' + 4 hex de un hash del id original.
"""
from __future__ import annotations

import hashlib
import re

FLOW_ID_PATTERN = r"^[a-z0-9]([-a-z0-9]{0,33}[a-z0-9])?$"  # igual que contracts/plans/flow-plan.schema.json (prueba de paridad)
FLOW_ID_RE = re.compile(FLOW_ID_PATTERN)
MAX_LEN = 35
_CUT = 30
_FALLBACK = "flujo"
_ALNUM = frozenset("abcdefghijklmnopqrstuvwxyz0123456789")
_DOUBLE_DASH = re.compile(r"-{2,}")


def _clean(raw: str) -> str:
    chars = ("".join(c if c in _ALNUM else "-" for c in raw.lower()))
    return _DOUBLE_DASH.sub("-", chars).strip("-")


def normalize_flow_id(raw: str) -> str:
    """Devuelve un id que cumple FLOW_ID_PATTERN. Idempotente; el mismo `raw` da siempre el mismo resultado."""
    t = _clean(raw)
    if not t:
        return _FALLBACK
    if len(t) <= MAX_LEN:
        return t
    # surrogatepass: json.loads admite sustitutos sueltos (\ud800); el hash debe ser total y determinista.
    digest = hashlib.sha256(raw.encode("utf-8", "surrogatepass")).hexdigest()[:4]
    return t[:_CUT].rstrip("-") + "-" + digest


def _with_suffix(base: str, k: int) -> str:
    suffix = f"-{k}"
    return base[: MAX_LEN - len(suffix)].rstrip("-") + suffix


def normalize_plan_flow_ids(plan):
    """Normaliza el flow_id de cada flujo y desambigua colisiones dentro del plan (orden de aparicion).

    Solo toca flujos con flow_id de cadena; el resto lo rechaza el esquema. Modifica `plan` in situ
    (es el dict recien parseado de la respuesta del modelo) y lo devuelve.
    """
    flows = plan.get("flows") if isinstance(plan, dict) else None
    if not isinstance(flows, list):
        return plan
    used: set[str] = set()
    for fl in flows:
        if not isinstance(fl, dict) or not isinstance(fl.get("flow_id"), str):
            continue
        base = normalize_flow_id(fl["flow_id"])
        cand, k = base, 2
        while cand in used:
            cand = _with_suffix(base, k)
            k += 1
        used.add(cand)
        fl["flow_id"] = cand
    return plan
