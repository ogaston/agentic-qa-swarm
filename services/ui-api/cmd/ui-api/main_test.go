package main

import "testing"

func TestAuthIsMandatory(t *testing.T) {
	t.Setenv("UIAPI_AUTH", "")
	t.Setenv("UIAPI_FAKE_TOKENS", "t=u:user")
	if _, err := verifierFromEnv(); err == nil {
		t.Fatal("sin UIAPI_AUTH el servicio no debe arrancar")
	}
	t.Setenv("UIAPI_AUTH", "open")
	if _, err := verifierFromEnv(); err == nil {
		t.Fatal("un modo desconocido no debe arrancar")
	}
	t.Setenv("UIAPI_AUTH", "fake")
	t.Setenv("UIAPI_FAKE_TOKENS", "")
	if _, err := verifierFromEnv(); err == nil {
		t.Fatal("fake sin tokens no debe arrancar")
	}
	t.Setenv("UIAPI_FAKE_TOKENS", "t=u:user")
	if _, err := verifierFromEnv(); err != nil {
		t.Fatal(err)
	}
}
