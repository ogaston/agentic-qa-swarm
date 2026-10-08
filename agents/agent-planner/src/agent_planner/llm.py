"""Puerto LLMClient y errores tipados. Sin SDK de proveedor (el adaptador real es de U3-T07)."""
from __future__ import annotations

from dataclasses import dataclass
from typing import Protocol


class LLMError(Exception):
    """Base de los errores del puerto."""


class LLMTimeout(LLMError):
    """El modelo no respondio dentro de timeout_s."""


class LLMUnavailable(LLMError):
    """El proveedor no esta disponible."""


class UnscriptedPrompt(LLMError):
    """El fake recibio un prompt sin respuesta registrada (falla cerrado)."""


@dataclass(frozen=True)
class LLMResult:
    text: str
    input_tokens: int
    output_tokens: int


class LLMClient(Protocol):
    def complete(self, prompt: str, *, max_tokens: int, timeout_s: float) -> LLMResult: ...
