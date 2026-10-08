import tempfile
from dataclasses import dataclass
from pathlib import Path

from .errors import FlowValidationError, K6InspectError
from .render import render_flow
from .store import flow_key, sha256
from .validate import inspect_with_k6, validate_plan, validate_script


@dataclass(frozen=True)
class PublishedFlow:
    flow_id: str
    key: str
    sha256: str


def publish_flows(plan, store, *, k6=inspect_with_k6) -> list[PublishedFlow]:
    """Valida TODO (3 capas) y solo entonces escribe. Todo o nada."""
    validate_plan(plan)
    rendered = []
    with tempfile.TemporaryDirectory(prefix="aqs-k6-") as tmp:
        for flow in plan["flows"]:
            fid = flow["flow_id"]
            script = render_flow(plan, flow)
            validate_script(script, plan, flow)
            f = Path(tmp) / f"{fid}.k6.js"
            f.write_bytes(script.encode("utf-8"))
            try:
                k6(f)
            except K6InspectError as e:
                raise FlowValidationError(fid, "k6", str(e)) from e
            rendered.append((fid, flow_key(plan["run_id"], fid), script.encode("utf-8")))
    # previos: si algo falla se restauran los bytes anteriores (o se borra lo que era nuevo)
    written, out = [], []
    try:
        for fid, key, data in rendered:
            try:
                prev = store.get(key)
            except (KeyError, FileNotFoundError):
                prev = None
            written.append((key, prev))
            store.put(key, data)
            if sha256(store.get(key)) != sha256(data):
                raise OSError(f"lectura de vuelta no coincide: {key}")
            out.append(PublishedFlow(fid, key, sha256(data)))
    except BaseException as err:
        for key, prev in written:
            try:
                if prev is None:
                    store.delete(key)
                else:
                    store.put(key, prev)
            except Exception as rb:
                err.add_note(f"rollback fallo para {key}: {rb!r}")
        raise
    return out
