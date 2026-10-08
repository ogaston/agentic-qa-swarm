"""Demostracion PBT-08: una propiedad deliberadamente FALSA para ver contraejemplo reducido y reproduccion.

Excluida del pytest normal por addopts (-m 'not pbt_demo'). Correr: pytest -m pbt_demo.
"""
import pytest
from hypothesis import given, settings

import gen


@pytest.mark.pbt_demo
@settings(database=None)  # sin base de ejemplos: la salida no depende de corridas previas
@given(gen.run_id())
def test_pbt_demo_run_id_is_short(run_id):
    assert len(run_id) < 5  # falsa a proposito: el contraejemplo reducido es un run_id de 5 caracteres
