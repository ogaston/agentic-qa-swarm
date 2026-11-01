package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	wm "github.com/ogaston/agentic-qa-swarm/services/go-warm-manager"
	"github.com/ogaston/agentic-qa-swarm/services/go-warm-manager/provision"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestSyncRepositories(t *testing.T) {
	body := `[{"repo":"acme/shop","image_repo":"ghcr.io/acme/shop","enabled":true}]`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Error("missing service token")
		}
		w.Write([]byte(body))
	}))
	defer server.Close()
	cs := fake.NewClientset()
	rec := &provision.Reconciler{Kube: cs}
	svc := &wm.Service{}
	sync := func() error {
		return syncRepositories(context.Background(), server.Client(), server.URL, "token", rec, svc)
	}
	if err := sync(); err != nil {
		t.Fatal(err)
	}
	if _, err := cs.CoreV1().Namespaces().Get(context.Background(), "aqs-w-acme-shop", metav1.GetOptions{}); err != nil {
		t.Fatal(err)
	}
	body = `null`
	if err := sync(); err == nil {
		t.Fatal("null registry accepted")
	}
	if _, err := cs.CoreV1().Namespaces().Get(context.Background(), "aqs-w-acme-shop", metav1.GetOptions{}); err != nil {
		t.Fatal("namespace removed on invalid response", err)
	}
	body = `[]`
	if err := sync(); err != nil {
		t.Fatal(err)
	}
	if _, err := cs.CoreV1().Namespaces().Get(context.Background(), "aqs-w-acme-shop", metav1.GetOptions{}); err == nil {
		t.Fatal("removed repository retained")
	}
}
