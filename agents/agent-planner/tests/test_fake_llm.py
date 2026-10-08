import json
from pathlib import Path

import pytest

from agent_planner.fakes.fake_llm import FakeLLM, prompt_hash
from agent_planner.llm import LLMTimeout, LLMUnavailable, UnscriptedPrompt

VECTORS = Path(__file__).resolve().parents[2] / "dataset" / "fake-llm-vectors.json"


def call(f, p):
    return f.complete(p, max_tokens=100, timeout_s=5.0)


def test_fake_llm_same_input_same_output():
    f = FakeLLM()
    f.register("hola", "respuesta", input_tokens=3, output_tokens=4)
    a, b = call(f, "hola"), call(f, "hola")
    assert a == b and a.text == "respuesta" and (a.input_tokens, a.output_tokens) == (3, 4)


def test_fake_llm_unscripted_prompt_fails_closed():
    f = FakeLLM()
    f.register("hola", "x")
    with pytest.raises(UnscriptedPrompt):
        call(f, "otro")


def test_fake_llm_crlf_normalized():
    f = FakeLLM()
    f.register("a\nb", "ok")
    assert call(f, "a\r\nb").text == "ok"
    assert prompt_hash("a\r\nb") == prompt_hash("a\nb")


def test_fake_llm_counts_calls():
    f = FakeLLM()
    f.register("p", "t")
    call(f, "p"); call(f, "p")
    with pytest.raises(UnscriptedPrompt):
        call(f, "q")
    assert f.calls == 3


def test_fake_llm_scripted_error():
    f = FakeLLM()
    f.register("p", error=LLMUnavailable("caido"))
    with pytest.raises(LLMUnavailable):
        call(f, "p")


def test_fake_llm_latency_timeout_without_real_sleep():
    slept = []
    f = FakeLLM(sleep=slept.append)
    f.register("lento", "x", latency_s=30.0)
    f.register("rapido", "y", latency_s=0.5)
    with pytest.raises(LLMTimeout):
        call(f, "lento")
    assert call(f, "rapido").text == "y"
    assert slept == [5.0, 0.5] and f.simulated_elapsed_s == 5.5


def test_fake_llm_shared_vectors():
    data = json.loads(VECTORS.read_text(encoding="utf-8"))
    assert len(data["vectors"]) >= 5
    for v in data["vectors"]:
        assert prompt_hash(v["prompt"]) == v["sha256"], v["prompt"]
