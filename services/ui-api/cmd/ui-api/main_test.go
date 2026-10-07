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
	t.Setenv("UIAPI_ALLOW_FAKE_AUTH", "true")
	t.Setenv("UIAPI_FAKE_TOKENS", "")
	if _, err := verifierFromEnv(); err == nil {
		t.Fatal("fake sin tokens no debe arrancar")
	}
	t.Setenv("UIAPI_FAKE_TOKENS", "t=u:user")
	if _, err := verifierFromEnv(); err != nil {
		t.Fatal(err)
	}
}

func TestFakeAuthIsFencedAndIdentityNeedsURL(t *testing.T) {
	t.Setenv("UIAPI_AUTH", "fake")
	t.Setenv("UIAPI_FAKE_TOKENS", "t=u:user")
	t.Setenv("UIAPI_ALLOW_FAKE_AUTH", "")
	if _, err := verifierFromEnv(); err == nil {
		t.Fatal("fake sin UIAPI_ALLOW_FAKE_AUTH=true no debe arrancar")
	}
	t.Setenv("UIAPI_ALLOW_FAKE_AUTH", "true")
	t.Setenv("UIAPI_ENV", "prod")
	if _, err := verifierFromEnv(); err == nil {
		t.Fatal("fake en prod no debe arrancar")
	}
	t.Setenv("UIAPI_AUTH", "identity")
	t.Setenv("IDENTITY_URL", "")
	if _, err := verifierFromEnv(); err == nil {
		t.Fatal("identity sin IDENTITY_URL no debe arrancar")
	}
	t.Setenv("IDENTITY_URL", "ftp://x")
	if _, err := verifierFromEnv(); err == nil {
		t.Fatal("IDENTITY_URL no http(s) no debe arrancar")
	}
	t.Setenv("IDENTITY_URL", "http://127.0.0.1:18200")
	if _, err := verifierFromEnv(); err != nil {
		t.Fatal(err)
	}
}
