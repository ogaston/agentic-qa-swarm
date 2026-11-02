#!/usr/bin/env python3
"""Secreto del webhook que go-intake verifica para un repo registrado en go-governance.

Misma regla que services/go-intake/internal/intake/webhook_secret.go (vectores dorados en
contracts/examples/webhook-secrets.json): HMAC-SHA256 con la clave de plataforma sobre
"aqs/webhook-secret/v1" NUL repo-en-minúsculas NUL versión, con prefijo "whsec_".

Uso: webhook-secret.py <archivo-clave-plataforma> <owner/repo> <versión> <archivo-salida>
Lee la clave de un archivo y escribe el secreto en otro (modo 600): nunca pasa por argumentos ni stdout.
"""
from __future__ import annotations

import hashlib
import hmac
import os
import re
import sys

CONTEXT = b"aqs/webhook-secret/v1"
REPO_RE = re.compile(r"^[a-z0-9-]+/[a-z0-9._-]+$")


def derive(platform_key: bytes, repo: str, version: int) -> str:
    repo = repo.strip().lower()
    if not platform_key or version < 1 or not REPO_RE.match(repo):
        raise ValueError("parámetros de credencial webhook inválidos")
    msg = CONTEXT + b"\0" + repo.encode() + b"\0" + str(version).encode()
    return "whsec_" + hmac.new(platform_key, msg, hashlib.sha256).hexdigest()


def main(argv: list[str]) -> int:
    if len(argv) != 5:
        print("uso: webhook-secret.py <archivo-clave> <owner/repo> <versión> <archivo-salida>", file=sys.stderr)
        return 2
    with open(argv[1], "rb") as f:
        key = f.read()
    try:
        secret = derive(key, argv[2], int(argv[3]))
    except ValueError as e:
        print(f"webhook-secret: {e}", file=sys.stderr)
        return 1
    fd = os.open(argv[4], os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(fd, "w") as f:
        f.write(secret)
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
