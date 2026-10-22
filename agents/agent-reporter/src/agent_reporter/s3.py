"""Adaptadores S3/MinIO del reporter (U8-T05): EvidenceReader y ReportStore.

Solo se usan con REPORTER_EVIDENCE=s3. El bucket llega por configuracion (EVIDENCE_BUCKET);
las URIs que no apunten a ese bucket se rechazan sin llamar a S3. TLS obligatorio con la CA de MinIO.
"""
from __future__ import annotations

import hashlib
import os
import re
from urllib.parse import urlsplit

from agent_reporter.errors import EvidenceUnavailable, InvalidEvidenceUri, InvalidReportUri, StoreUnavailable

_RUN_ID = re.compile(r"^[A-Za-z0-9._-]{1,128}$")
_SEG_BAD = ("", ".", "..")
_REPORT_KEY = re.compile(r"^runs/([A-Za-z0-9._-]{1,128})/report-([0-9a-f]{64})\.json$")


def report_key_of(uri: str, bucket: str) -> str:
    """Clave runs/<run_id>/report-<sha256>.json de una URI de reporte de ESTE bucket.

    Cualquier otra forma (otro bucket, otro esquema, query, clave fuera de la forma que escribe put) lanza
    InvalidReportUri sin llamar a S3 (U9-T01).
    """
    try:
        p = urlsplit(uri)
    except ValueError as e:
        raise InvalidReportUri("uri invalida") from e
    m = _REPORT_KEY.fullmatch(p.path[1:]) if p.path.startswith("/") else None
    if (
        p.scheme != "s3" or p.netloc != bucket or p.query or p.fragment or m is None
        or m.group(1) in (".", "..")
    ):
        raise InvalidReportUri("uri invalida")
    return p.path[1:]


def _key_of(uri: str, bucket: str, exc) -> str:
    """Devuelve la clave de s3://<bucket>/<key>; cualquier otra forma lanza exc."""
    try:
        p = urlsplit(uri)
    except ValueError as e:
        raise exc("uri invalida") from e
    key = p.path[1:] if p.path.startswith("/") else ""
    segs = key.split("/")
    if p.scheme != "s3" or p.netloc != bucket or not key or any(s in _SEG_BAD for s in segs):
        raise exc("uri invalida")
    return key


class S3EvidenceReader:
    def __init__(self, client, bucket: str) -> None:
        self.client = client
        self.bucket = bucket

    def get(self, uri: str) -> bytes:
        key = _key_of(uri, self.bucket, InvalidEvidenceUri)
        try:
            body = self.client.get_object(Bucket=self.bucket, Key=key)["Body"]
            return body.read()
        except Exception as e:  # noqa: BLE001 - sin detalle del proveedor en el mensaje
            raise EvidenceUnavailable("objeto no disponible") from None


class S3ReportStore:
    def __init__(self, client, bucket: str) -> None:
        self.client = client
        self.bucket = bucket

    def put(self, run_id: str, data: bytes) -> str:
        if not isinstance(run_id, str) or not _RUN_ID.fullmatch(run_id):
            raise StoreUnavailable("run_id no valido para clave")
        key = f"runs/{run_id}/report-{hashlib.sha256(data).hexdigest()}.json"
        try:
            self.client.put_object(Bucket=self.bucket, Key=key, Body=data, ContentType="application/json")
        except Exception:  # noqa: BLE001
            raise StoreUnavailable("put fallido") from None
        return f"s3://{self.bucket}/{key}"

    def get(self, uri: str) -> bytes:
        key = report_key_of(uri, self.bucket)
        try:
            return self.client.get_object(Bucket=self.bucket, Key=key)["Body"].read()
        except Exception:  # noqa: BLE001
            raise StoreUnavailable("get fallido") from None


def make_s3_client(env=None):
    """S3_ENDPOINT (https), S3_ACCESS_KEY_FILE, S3_SECRET_KEY_FILE, S3_CA_FILE (obligatorios)."""
    import boto3
    from botocore.config import Config

    e = os.environ if env is None else env
    endpoint = e.get("S3_ENDPOINT", "")
    ak_f, sk_f, ca = e.get("S3_ACCESS_KEY_FILE", ""), e.get("S3_SECRET_KEY_FILE", ""), e.get("S3_CA_FILE", "")
    if urlsplit(endpoint).scheme != "https" or not urlsplit(endpoint).netloc:
        raise SystemExit("S3_ENDPOINT debe ser https://host:puerto")
    if not (ak_f and sk_f and ca) or not os.path.isfile(ca):
        raise SystemExit("S3 requiere S3_ACCESS_KEY_FILE, S3_SECRET_KEY_FILE y S3_CA_FILE existentes")
    try:
        with open(ak_f, encoding="utf-8") as fh:
            ak = fh.read().strip()
        with open(sk_f, encoding="utf-8") as fh:
            sk = fh.read().strip()
    except OSError:
        raise SystemExit("no se pudieron leer las credenciales S3") from None
    cfg = Config(signature_version="s3v4", connect_timeout=5, read_timeout=15, retries={"max_attempts": 2})
    return boto3.client(
        "s3", endpoint_url=endpoint, region_name=e.get("S3_REGION", "us-east-1"),
        aws_access_key_id=ak, aws_secret_access_key=sk, verify=ca, config=cfg,
    )
