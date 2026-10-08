from __future__ import annotations

from agent_reporter.errors import StoreUnavailable


class MemoryReportStore:
    def __init__(self, fail_put: bool = False, corrupt_readback: bool = False) -> None:
        self.objects: dict[str, bytes] = {}
        self.fail_put = fail_put
        self.corrupt_readback = corrupt_readback

    def put(self, run_id: str, data: bytes) -> str:
        if self.fail_put:
            raise StoreUnavailable("put programado para fallar")
        uri = f"mem://reports/{run_id}.json"
        self.objects[uri] = data
        return uri

    def get(self, uri: str) -> bytes:
        data = self.objects[uri]
        return data + b" " if self.corrupt_readback else data
