"""GET /v1/reports?uri=<s3-uri> del reporter (U9-T01). Sin LLM. Stubber y servidor real en localhost."""
from __future__ import annotations

import hashlib
import io
import json
import threading
import urllib.error
import urllib.parse
import urllib.request

import pytest

from agent_reporter.errors import StoreUnavailable
from agent_reporter.fakes.evidence import DirEvidenceReader
from agent_reporter.fakes.fake_llm import FakeLLM
from agent_reporter.fakes.publisher import RecordingPublisher
from agent_reporter.fakes.store import MemoryReportStore
from agent_reporter.reporter import Limits
from agent_reporter.s3 import S3ReportStore
from agent_reporter.server import Service, make_server

BUCKET = "evidence"
RUN = "run-7"
DATA = b'{"run_id":"run-7","verdict":"bug","summary":"s","findings":[]}'
SHA = hashlib.sha256(DATA).hexdigest()
GOOD = f"s3://{BUCKET}/runs/{RUN}/report-{SHA}.json"


class CountingS3:
    """Cliente S3 falso: cuenta llamadas a get_object y sirve DATA para cualquier clave."""

    def __init__(self, fail: bool = False) -> None:
        self.gets = 0
        self.fail = fail

    def get_object(self, Bucket, Key):  # noqa: N803 - firma de boto3
        self.gets += 1
        if self.fail:
            raise RuntimeError("MinIO caido")
        return {"Body": io.BytesIO(DATA)}


class CountingStore:
    """Envuelve un store y cuenta las llamadas a get."""

    def __init__(self, inner) -> None:
        self.inner = inner
        self.gets = 0

    def put(self, run_id, data):
        return self.inner.put(run_id, data)

    def get(self, uri):
        self.gets += 1
        return self.inner.get(uri)


@pytest.fixture
def srv(tmp_path):
    mem = MemoryReportStore()
    store = CountingStore(mem)
    svc = Service(DirEvidenceReader(tmp_path), FakeLLM(), store, RecordingPublisher(), Limits(), report_bucket=BUCKET)
    s = make_server(svc)
    threading.Thread(target=s.serve_forever, daemon=True).start()
    yield s, store
    s.shutdown()
    s.server_close()


def get(s, uri_param):
    q = urllib.parse.urlencode({"uri": uri_param}) if uri_param is not None else ""
    url = f"http://127.0.0.1:{s.server_address[1]}/v1/reports" + (f"?{q}" if q else "")
    try:
        with urllib.request.urlopen(url) as r:
            return r.status, r.read()
    except urllib.error.HTTPError as e:
        return e.code, e.read()


def test_get_report_uri_valida_devuelve_el_json_guardado(srv):
    s, store = srv
    store.inner.objects[GOOD] = DATA  # el store en memoria guarda bajo la URI s3 del bucket del servicio
    code, body = get(s, GOOD)
    assert code == 200
    assert body == DATA
    assert store.gets == 1


def test_get_report_bucket_distinto_es_400_sin_llamar_al_store(srv):
    s, store = srv
    code, body = get(s, f"s3://otro-bucket/runs/{RUN}/report-{SHA}.json")
    assert code == 400
    assert json.loads(body)["error"] == "invalid_uri"
    assert store.gets == 0


@pytest.mark.parametrize("bad", [
    f"s3://{BUCKET}/otro/{RUN}/report-{SHA}.json",
    f"s3://{BUCKET}/runs/{RUN}/evidence-{SHA}.json",
    f"s3://{BUCKET}/runs/{RUN}/report-{SHA[:-1]}.json",
    f"s3://{BUCKET}/runs/../report-{SHA}.json",
    f"s3://{BUCKET}/runs/{RUN}/report-{SHA}.json?x=1",
    f"http://{BUCKET}/runs/{RUN}/report-{SHA}.json",
    "no-es-una-uri",
])
def test_get_report_clave_o_esquema_fuera_de_forma_es_400(srv, bad):
    s, store = srv
    code, body = get(s, bad)
    assert code == 400
    assert json.loads(body)["error"] == "invalid_uri"
    assert store.gets == 0


def test_get_report_sin_uri_o_con_dos_es_400(srv):
    s, store = srv
    assert get(s, None)[0] == 400
    url = f"http://127.0.0.1:{s.server_address[1]}/v1/reports?uri=a&uri=b"
    with pytest.raises(urllib.error.HTTPError) as e:
        urllib.request.urlopen(url)
    assert e.value.code == 400
    assert store.gets == 0


def test_get_report_store_caido_es_502_store_unavailable(srv):
    s, store = srv

    def boom(uri):
        raise StoreUnavailable("get fallido")

    store.get = boom
    code, body = get(s, GOOD)
    assert code == 502
    assert json.loads(body)["error"] == "store_unavailable"


def test_get_report_s3_valida_clave_sin_tocar_minio():
    c = CountingS3()
    st = S3ReportStore(c, BUCKET)
    for bad in [
        f"s3://otro/runs/{RUN}/report-{SHA}.json",
        f"s3://{BUCKET}/runs/../report-{SHA}.json",
        f"s3://{BUCKET}/runs/{RUN}/report-XYZ.json",
        f"https://{BUCKET}/runs/{RUN}/report-{SHA}.json",
    ]:
        with pytest.raises(StoreUnavailable):
            st.get(bad)
    assert c.gets == 0
    assert st.get(GOOD) == DATA
    assert c.gets == 1


def test_get_report_s3_error_del_cliente_es_store_unavailable():
    with pytest.raises(StoreUnavailable):
        S3ReportStore(CountingS3(fail=True), BUCKET).get(GOOD)
