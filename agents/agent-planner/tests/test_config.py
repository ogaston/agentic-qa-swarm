import pytest

from agent_planner.config import ConfigError, load_config

OK = {"LLM_PROVIDER": "fake", "PLANNER_ALLOW_FAKE": "true"}


def test_config_fake_allowed_with_flag():
    c = load_config(OK)
    assert c.limits.timeout_s == 25.0 and c.limits.max_flows == 10 and c.limits.max_steps == 20


@pytest.mark.parametrize("env", [
    {},
    {"LLM_PROVIDER": "fake"},
    {"LLM_PROVIDER": "fake", "PLANNER_ALLOW_FAKE": "false"},
    {**OK, "PLANNER_ENV": "prod"},
    {**OK, "PLANNER_ENV": "PROD"},
    {"LLM_PROVIDER": "otro", "PLANNER_ALLOW_FAKE": "true"},
    {**OK, "PLANNER_TIMEOUT_S": "0"},
    {**OK, "PLANNER_TIMEOUT_S": "nan"},
    {**OK, "PLANNER_TIMEOUT_S": "abc"},
    {**OK, "PLANNER_MAX_OUTPUT_TOKENS": "-1"},
    {**OK, "PLANNER_MAX_INPUT_TOKENS": "0"},
])
def test_config_fail_closed(env):
    with pytest.raises(ConfigError):
        load_config(env)


def test_config_provider_not_implemented_message():
    with pytest.raises(ConfigError, match="proveedor no implementado"):
        load_config({"LLM_PROVIDER": "otro"})


def _http_env(tmp_path, **extra):
    key = tmp_path / "k"
    key.write_text("x", encoding="utf-8")
    env = {"LLM_PROVIDER": "http", "LLM_BASE_URL": "http://10.20.30.40:8000/v1",
           "LLM_MODEL": "m", "LLM_API_KEY_FILE": str(key)}
    env.update(extra)
    return env


def test_config_http_gateway_sin_variable_rechazado(tmp_path):
    with pytest.raises(ConfigError):
        load_config(_http_env(tmp_path))


def test_config_http_gateway_con_variable_y_host_aceptado(tmp_path):
    c = load_config(_http_env(tmp_path, LLM_ALLOW_INSECURE_HTTP="1", LLM_INSECURE_HTTP_HOST="10.20.30.40"))
    assert c.llm is not None


def test_config_http_gateway_variable_distinta_de_1_rechazado(tmp_path):
    with pytest.raises(ConfigError):
        load_config(_http_env(tmp_path, LLM_ALLOW_INSECURE_HTTP="true", LLM_INSECURE_HTTP_HOST="10.20.30.40"))
