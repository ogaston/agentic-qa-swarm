"""Puertos del reporter. Las implementaciones reales (S3, transporte de eventos) son de U3-T07 / C-45."""
from __future__ import annotations

from typing import Protocol


class EvidenceReader(Protocol):
    def get(self, uri: str) -> bytes: ...


class ReportStore(Protocol):
    def put(self, run_id: str, data: bytes) -> str: ...

    def get(self, uri: str) -> bytes: ...


class EventPublisher(Protocol):
    def publish(self, event: dict) -> None: ...
