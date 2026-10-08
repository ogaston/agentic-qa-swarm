import json

import pytest

from agent_planner.errors import BudgetExceeded, Limits, PlanRejected
from agent_planner.llm import LLMUnavailable
from agent_planner.fakes.fake_llm import FakeLLM
from agent_planner.planner import plan
from agent_planner.prompt import build_prompt
from helpers import LIMITS, SURFACE, good_plan, llm_for


class Spy:
    def __init__(self, text):
        self.text, self.kw, self.n = text, None, 0

    def complete(self, prompt, *, max_tokens, timeout_s):
        from agent_planner.llm import LLMResult
        self.n += 1
        self.kw = (max_tokens, timeout_s)
        return LLMResult(self.text, 10, 10)


def test_budget_huge_surface_input_exceeded_without_calling_model():
    s = dict(SURFACE, endpoints=[{"method": "GET", "path": f"/r{i}"} for i in range(3000)])
    f = FakeLLM()
    with pytest.raises(BudgetExceeded) as e:
        plan(s, "wf-1", f, Limits(max_input_tokens=1000))
    assert e.value.kind == "input" and f.calls == 0


def test_budget_input_boundary_uses_ceil_len_over_4():
    n = -(-len(build_prompt(SURFACE, "wf-1")) // 4)
    f = llm_for(SURFACE, "wf-1", json.dumps(good_plan()))
    plan(SURFACE, "wf-1", f, Limits(max_input_tokens=n))
    with pytest.raises(BudgetExceeded):
        plan(SURFACE, "wf-1", f, Limits(max_input_tokens=n - 1))
    assert f.calls == 1


def test_budget_output_tokens_declared_over_cap():
    f = llm_for(SURFACE, "wf-1", json.dumps(good_plan()), output_tokens=5001)
    with pytest.raises(BudgetExceeded) as e:
        plan(SURFACE, "wf-1", f, Limits(max_output_tokens=5000))
    assert e.value.kind == "output"
    f = llm_for(SURFACE, "wf-1", json.dumps(good_plan()), output_tokens=5000)
    plan(SURFACE, "wf-1", f, Limits(max_output_tokens=5000))


def test_budget_output_text_longer_than_cap_even_if_declared_small():
    f = llm_for(SURFACE, "wf-1", "x" * 400, output_tokens=1)
    with pytest.raises(BudgetExceeded) as e:
        plan(SURFACE, "wf-1", f, Limits(max_output_tokens=50))
    assert e.value.kind == "output"


def test_timeout_simulated_latency_over_limit():
    f = llm_for(SURFACE, "wf-1", json.dumps(good_plan()), latency_s=30.0)
    with pytest.raises(BudgetExceeded) as e:
        plan(SURFACE, "wf-1", f, Limits(timeout_s=25.0))
    assert e.value.kind == "time"
    assert f.calls == 1  # sin reintento


def test_budget_limits_reach_client_unchanged():
    spy = Spy(json.dumps(good_plan()))
    plan(SURFACE, "wf-1", spy, Limits(timeout_s=7.5, max_output_tokens=1234))
    assert spy.kw == (1234, 7.5)


def test_budget_no_retry_after_failure():
    f = FakeLLM()
    f.register(build_prompt(SURFACE, "wf-1"), error=LLMUnavailable("x"))
    with pytest.raises(LLMUnavailable):
        plan(SURFACE, "wf-1", f, LIMITS)
    assert f.calls == 1
    f2 = llm_for(SURFACE, "wf-1", "no es json")
    with pytest.raises(PlanRejected):
        plan(SURFACE, "wf-1", f2, LIMITS)
    assert f2.calls == 1
