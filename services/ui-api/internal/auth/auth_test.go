package auth

import (
	"context"
	"testing"
)

func TestFakeVerifier(t *testing.T) {
	v, err := NewFakeTokenVerifier("tok-user=u1:user, tok-admin=a1:admin")
	if err != nil {
		t.Fatal(err)
	}
	p, err := v.Verify(context.Background(), "tok-admin")
	if err != nil || p != (Principal{ID: "a1", Role: "admin"}) {
		t.Fatalf("%+v %v", p, err)
	}
	for _, bad := range []string{"", "nope", "tok-use", "tok-user "} {
		if _, err := v.Verify(context.Background(), bad); err == nil {
			t.Fatalf("%q debio fallar", bad)
		}
	}
}

func TestFakeVerifierConfigErrors(t *testing.T) {
	for _, spec := range []string{"", "x", "x=y", "x=y:root", "=y:user", "x=:user"} {
		if _, err := NewFakeTokenVerifier(spec); err == nil {
			t.Fatalf("%q debio fallar", spec)
		}
	}
}

func TestBearerToken(t *testing.T) {
	for h, want := range map[string]string{"Bearer abc": "abc", "bearer abc": "abc"} {
		if got, ok := BearerToken(h); !ok || got != want {
			t.Fatalf("%q -> %q %v", h, got, ok)
		}
	}
	for _, h := range []string{"", "Bearer", "Bearer ", "Basic dG9r", "Bearer a b", "abc"} {
		if _, ok := BearerToken(h); ok {
			t.Fatalf("%q no debio aceptarse", h)
		}
	}
}
