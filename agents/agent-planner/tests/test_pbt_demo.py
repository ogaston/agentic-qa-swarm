"""Demostracion PBT-08: una propiedad deliberadamente FALSA para ver contraejemplo reducido y reproduccion.

Excluida del pytest normal por addopts (-m 'not pbt_demo'). Correr: pytest -m pbt_demo.
"""
import pytest
from hypothesis import given, settings
from hypothesis import strategies as st

import gen


@pytest.mark.pbt_demo
@settings(database=None)  # sin base de ejemplos: la salida no depende de corridas previas
@given(gen.flow_id())
def test_pbt_demo_flow_id_is_short(flow_id):
    assert len(flow_id) < 5  # falsa a proposito: el contraejemplo reducido es un flow_id de 5 caracteres


@pytest.mark.pbt_demo
@settings(database=None)
@given(st.integers(2, 10**4), st.integers(2, 10**4), st.integers(2, 10**4))
def test_pbt_demo_seed_dependent_counterexample(a, b, c):
    # falsa a proposito y con muchos contraejemplos minimos locales: CUAL sale depende del orden de sorteo, es decir
    # del seed. Mismo seed -> mismo (a, b, c); otro seed -> otro. Demuestra que --hypothesis-seed reproduce.
    assert (a * b * c) % 31 != 1
