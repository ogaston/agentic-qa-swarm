"""Punto de entrada: python -m agent_planner."""
from __future__ import annotations

import logging
import os
import sys

from .config import ConfigError, load_config
from .server import make_server


def main() -> int:
    logging.basicConfig(level=logging.INFO, format="%(message)s", stream=sys.stdout)
    try:
        cfg = load_config(os.environ)
    except ConfigError as e:
        print(f"agent-planner: configuracion invalida: {e}", file=sys.stderr)
        return 2
    srv = make_server(cfg.host, cfg.port, cfg.llm, cfg.limits)
    try:
        srv.serve_forever()
    except KeyboardInterrupt:
        pass
    return 0


if __name__ == "__main__":
    sys.exit(main())
