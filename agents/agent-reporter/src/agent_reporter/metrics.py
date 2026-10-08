from __future__ import annotations

import threading
from collections import Counter


class Metrics:
    def __init__(self) -> None:
        self._lock = threading.Lock()
        self.requests: Counter = Counter()
        self.redactions: Counter = Counter()

    def request(self, result: str) -> None:
        with self._lock:
            self.requests[result] += 1

    def redaction(self, counts: Counter) -> None:
        with self._lock:
            self.redactions.update(counts)

    def render(self) -> str:
        with self._lock:
            lines = ["# TYPE aqs_reporter_requests_total counter"]
            for k, v in sorted(self.requests.items()):
                lines.append(f'aqs_reporter_requests_total{{result="{k}"}} {v}')
            lines.append("# TYPE aqs_reporter_redactions_total counter")
            for k, v in sorted(self.redactions.items()):
                lines.append(f'aqs_reporter_redactions_total{{type="{k}"}} {v}')
        return "\n".join(lines) + "\n"


METRICS = Metrics()
