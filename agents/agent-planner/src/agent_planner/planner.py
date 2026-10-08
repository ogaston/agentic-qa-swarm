"""plan(): superficie -> FlowPlan o fallo cerrado. Una sola llamada al modelo, sin reintentos."""
from __future__ import annotations

import math

from .errors import BudgetExceeded, InvalidRequest, Limits, NoSurface
from .llm import LLMClient, LLMTimeout
from .prompt import build_prompt
from .schemas_loader import validator
from .validate import parse_and_validate

FlowPlan = dict
MAX_WORKFLOW_LEN = 128  # caracteres; mas largo es invalid_request (422)


def est_tokens(text: str) -> int:
    return math.ceil(len(text) / 4)


def plan(surface, workflow, llm: LLMClient, limits: Limits, usage: dict | None = None) -> FlowPlan:
    if not isinstance(workflow, str) or not workflow or len(workflow) > MAX_WORKFLOW_LEN:
        raise InvalidRequest("workflow_invalido")
    if not isinstance(surface, dict) or next(validator("surface-artifact.schema.json").iter_errors(surface), None):
        raise NoSurface("superficie_invalida")
    if not surface["endpoints"]:
        raise NoSurface("superficie_vacia")
    prompt = build_prompt(surface, workflow)
    if est_tokens(prompt) > limits.max_input_tokens:
        raise BudgetExceeded("input")
    try:
        res = llm.complete(prompt, max_tokens=limits.max_output_tokens, timeout_s=limits.timeout_s)
    except LLMTimeout:
        raise BudgetExceeded("time") from None
    if usage is not None:
        usage["input"] = res.input_tokens
        usage["output"] = res.output_tokens
    if res.output_tokens > limits.max_output_tokens or est_tokens(res.text) > limits.max_output_tokens:
        raise BudgetExceeded("output")
    return parse_and_validate(res.text, surface, workflow, limits)
