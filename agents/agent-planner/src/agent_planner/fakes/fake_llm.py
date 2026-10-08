"""LLM fake determinista por prompt-hash. Falla cerrado. Copia duplicada en ambos paquetes."""
from __future__ import annotations

import hashlib
from dataclasses import dataclass
from typing import Callable

from agent_planner.llm import LLMResult, LLMTimeout, UnscriptedPrompt


def prompt_hash(prompt: str) -> str:
    """sha256 hex del prompt en UTF-8 con saltos de linea normalizados a LF."""
    norm = prompt.replace("\r\n", "\n").replace("\r", "\n")
    return hashlib.sha256(norm.encode("utf-8")).hexdigest()


@dataclass
class _Scripted:
    text: str
    input_tokens: int
    output_tokens: int
    latency_s: float
    error: Exception | None


class FakeLLM:
    def __init__(self, sleep: Callable[[float], None] | None = None) -> None:
        self._responses: dict[str, _Scripted] = {}
        self._sleep = sleep
        self.calls = 0
        self.simulated_elapsed_s = 0.0

    def register(
        self,
        prompt: str,
        text: str = "",
        *,
        input_tokens: int = 0,
        output_tokens: int = 0,
        latency_s: float = 0.0,
        error: Exception | None = None,
    ) -> str:
        h = prompt_hash(prompt)
        self._responses[h] = _Scripted(text, input_tokens, output_tokens, latency_s, error)
        return h

    def complete(self, prompt: str, *, max_tokens: int, timeout_s: float) -> LLMResult:
        self.calls += 1
        r = self._responses.get(prompt_hash(prompt))
        if r is None:
            raise UnscriptedPrompt(prompt_hash(prompt))
        if r.latency_s:
            self.simulated_elapsed_s += min(r.latency_s, timeout_s)
            if self._sleep is not None:
                self._sleep(min(r.latency_s, timeout_s))
            if r.latency_s > timeout_s:
                raise LLMTimeout(f"latencia simulada {r.latency_s}s > {timeout_s}s")
        if r.error is not None:
            raise r.error
        return LLMResult(r.text, r.input_tokens, r.output_tokens)
