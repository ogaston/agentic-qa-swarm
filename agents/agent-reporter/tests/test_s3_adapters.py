"""Adaptadores S3 del reporter (U8-T05). Stubber: sin red. La prueba contra MinIO real va con la marca minio."""
from __future__ import annotations

import hashlib
import io
import os

import pytest
from botocore.exceptions import ClientError
from botocore.stub import Stubber

from agent_reporter.errors import EvidenceUnavailable, InvalidEvidenceUri, StoreUnavailable
from agent_reporter.s3 import S3EvidenceReader, S3ReportStore, make_s3_client

BUCKET = "evidence"
RUN = "run-7"


def _client():
    import boto3

    return boto3.client(
        "s3", endpoint_url="https://minio.invalid:9000", region_name="us-east-1",
        aws_access_key_id="k", aws_secret_access_key="s",
    )


def test_reader_lee_objeto_del_bucket_configurado():
    c = _client()
    st = Stubber(c)
    st.add_response("get_object", {"Body": io.BytesIO(b"{}")}, {"Bucket": BUCKET, "Key": "runs/run-7/flow-1/result.json"})
    with st:
        got = S3EvidenceReader(c, BUCKET).get(f"s3://{BUCKET}/runs/run-7/flow-1/result.json")
    assert got == b"{}"
    st.assert_no_pending_responses()


def test_reader_rechaza_otro_bucket_sin_llamar_a_s3():
    c = _client()
    st = Stubber(c)  # sin respuestas encoladas: cualquier llamada falla
    with st:
        with pytest.raises(InvalidEvidenceUri):
            S3EvidenceReader(c, BUCKET).get("s3://aqs-evidence/runs/run-7/x")
    st.assert_no_pending_responses()


@pytest.mark.parametrize("uri", ["s3://evidence/../secreto", "s3://evidence/", "http://evidence/a", "s3://evidence/a/./b"])
def test_reader_rechaza_uris_mal_formadas(uri):
    c = _client()
    with Stubber(c):
        with pytest.raises(InvalidEvidenceUri):
            S3EvidenceReader(c, BUCKET).get(uri)


def test_reader_objeto_inexistente_es_evidence_unavailable():
    c = _client()
    st = Stubber(c)
    st.add_client_error("get_object", service_error_code="NoSuchKey", http_status_code=404, service_message="no")
    with st:
        with pytest.raises(EvidenceUnavailable):
            S3EvidenceReader(c, BUCKET).get(f"s3://{BUCKET}/runs/run-7/nada")


def test_store_put_usa_clave_bajo_runs_y_devuelve_uri_s3():
    c = _client()
    data = b'{"verdict":"bug"}'
    digest = hashlib.sha256(data).hexdigest()
    key = f"runs/{RUN}/report-{digest}.json"
    st = Stubber(c)
    st.add_response("put_object", {}, {
        "Bucket": BUCKET, "Key": key, "Body": data, "ContentType": "application/json",
    })
    st.add_response("get_object", {"Body": io.BytesIO(data)}, {"Bucket": BUCKET, "Key": key})
    with st:
        uri = S3ReportStore(c, BUCKET).put(RUN, data)
        assert uri == f"s3://{BUCKET}/{key}"
        assert S3ReportStore(c, BUCKET).get(uri) == data
    st.assert_no_pending_responses()


def test_store_put_fallido_es_store_unavailable():
    c = _client()
    st = Stubber(c)
    st.add_client_error("put_object", service_error_code="AccessDenied", http_status_code=403, service_message="no")
    with st:
        with pytest.raises(StoreUnavailable):
            S3ReportStore(c, BUCKET).put(RUN, b"{}")


def test_store_get_de_otro_bucket_se_rechaza():
    c = _client()
    with Stubber(c):
        with pytest.raises(StoreUnavailable):
            S3ReportStore(c, BUCKET).get("s3://otro/runs/run-7/report-x.json")


def test_make_s3_client_exige_endpoint_y_credenciales(tmp_path):
    (tmp_path / "ak").write_text("AK\n")
    (tmp_path / "sk").write_text("SK\n")
    env = {"S3_ENDPOINT": "https://minio.aqs-system.svc:9000", "S3_ACCESS_KEY_FILE": str(tmp_path / "ak"),
           "S3_SECRET_KEY_FILE": str(tmp_path / "sk"), "S3_CA_FILE": str(tmp_path / "ca")}
    (tmp_path / "ca").write_text("x")
    c = make_s3_client(env)
    assert c.meta.endpoint_url == "https://minio.aqs-system.svc:9000"
    with pytest.raises(SystemExit):
        make_s3_client({"S3_ENDPOINT": "https://x"})
    with pytest.raises(SystemExit):
        make_s3_client({"S3_ENDPOINT": "http://x", "S3_ACCESS_KEY_FILE": str(tmp_path / "ak"), "S3_SECRET_KEY_FILE": str(tmp_path / "sk")})


@pytest.mark.minio
def test_minio_real_put_get_y_sin_otro_bucket():
    """CA-2/CA-3 en local: requiere AQS_S3_TEST_ENDPOINT, AQS_S3_TEST_ACCESS_KEY, AQS_S3_TEST_SECRET_KEY, bucket evidence."""
    ep = os.environ.get("AQS_S3_TEST_ENDPOINT")
    if not ep:
        pytest.skip("sin MinIO local")
    import boto3

    c = boto3.client("s3", endpoint_url=ep, region_name="us-east-1",
                     aws_access_key_id=os.environ["AQS_S3_TEST_ACCESS_KEY"],
                     aws_secret_access_key=os.environ["AQS_S3_TEST_SECRET_KEY"])
    store = S3ReportStore(c, BUCKET)
    uri = store.put("run-minio", b'{"ok":true}')
    assert store.get(uri) == b'{"ok":true}'
    with pytest.raises(StoreUnavailable):
        store.get(f"s3://otro-bucket/runs/run-minio/report-x.json")


def test_build_service_selecciona_s3_con_bucket_configurado(tmp_path):
    from agent_reporter.server import build_service
    from agent_reporter.s3 import S3EvidenceReader, S3ReportStore

    (tmp_path / "ak").write_text("AK")
    (tmp_path / "sk").write_text("SK")
    (tmp_path / "ca").write_text("x")
    env = {"LLM_PROVIDER": "fake", "REPORTER_ALLOW_FAKE": "1", "REPORTER_EVIDENCE": "s3", "EVIDENCE_BUCKET": "evidence-x",
           "S3_ENDPOINT": "https://minio.aqs-system.svc:9000", "S3_ACCESS_KEY_FILE": str(tmp_path / "ak"),
           "S3_SECRET_KEY_FILE": str(tmp_path / "sk"), "S3_CA_FILE": str(tmp_path / "ca")}
    svc = build_service(env)
    assert isinstance(svc.reader, S3EvidenceReader) and svc.reader.bucket == "evidence-x"
    assert isinstance(svc.store, S3ReportStore) and svc.store.bucket == "evidence-x"
    with pytest.raises(SystemExit):
        build_service({**env, "EVIDENCE_BUCKET": ""})
