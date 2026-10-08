import hashlib
import os
import re
import tempfile
from pathlib import Path
from typing import Protocol

ID_RE = re.compile(r"[a-z0-9]([a-z0-9-]{0,40}[a-z0-9])?")
KEY_RE = re.compile(r"flows/([a-z0-9]([a-z0-9-]{0,40}[a-z0-9])?)/([a-z0-9]([a-z0-9-]{0,40}[a-z0-9])?)\.k6\.js")


class FlowStore(Protocol):
    """Puerto de almacen. get de una clave ausente DEBE lanzar KeyError o FileNotFoundError
    (publish_flows lo usa para distinguir claves nuevas de sobrescrituras)."""

    def put(self, key: str, data: bytes) -> None: ...
    def get(self, key: str) -> bytes: ...
    def delete(self, key: str) -> None: ...


def flow_key(run_id: str, flow_id: str) -> str:
    if not ID_RE.fullmatch(run_id) or not ID_RE.fullmatch(flow_id):
        raise ValueError("run_id/flow_id fuera del patron permitido")
    return f"flows/{run_id}/{flow_id}.k6.js"


def sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


class DirFlowStore:
    def __init__(self, root):
        self.root = Path(root)

    def _path(self, key: str) -> Path:
        if not KEY_RE.fullmatch(key):
            raise ValueError(f"clave invalida: {key!r}")
        return self.root / key

    def put(self, key: str, data: bytes) -> None:
        p = self._path(key)
        p.parent.mkdir(parents=True, exist_ok=True)
        fd, tmp = tempfile.mkstemp(dir=p.parent, prefix=".tmp-")
        try:
            with os.fdopen(fd, "wb") as f:
                f.write(data)
                f.flush()
                os.fsync(f.fileno())
            os.replace(tmp, p)
        except BaseException:
            if os.path.exists(tmp):
                os.unlink(tmp)
            raise
        if sha256(p.read_bytes()) != sha256(data):
            p.unlink()
            raise OSError(f"lectura de vuelta no coincide: {key}")

    def get(self, key: str) -> bytes:
        return self._path(key).read_bytes()

    def delete(self, key: str) -> None:
        p = self._path(key)
        p.unlink(missing_ok=True)
        try:
            p.parent.rmdir()
        except OSError:
            pass


class FakeFlowStore:
    """En memoria con fallos programables."""

    def __init__(self, fail_put_on=None, corrupt_get=False):
        self.objects: dict[str, bytes] = {}
        self.fail_put_on = fail_put_on  # numero de put (1-based) que falla
        self.corrupt_get = corrupt_get
        self.puts = 0

    def put(self, key, data):
        self.puts += 1
        if self.fail_put_on == self.puts:
            raise OSError("fallo de escritura programado")
        self.objects[key] = bytes(data)

    def get(self, key):
        data = self.objects[key]
        return data + b"x" if self.corrupt_get else data

    def delete(self, key):
        self.objects.pop(key, None)
