//go:build minio

package objstore_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"

	wm "github.com/ogaston/agentic-qa-swarm/services/go-warm-manager"
	"github.com/ogaston/agentic-qa-swarm/services/go-warm-manager/adapters/objstore"
)

// Misma imagen (por digest) que scripts/test/minio-local.sh.
const minioImage = "cgr.dev/chainguard/minio@sha256:9dcc028b309030afa86fc1fc8d93907ae373ea3fb75277cca3fc77e4645932b7"

func TestObjectStoreS3(t *testing.T) {
	const ak, sk = "testaccesskey", "testsecretkey123"
	name := fmt.Sprintf("aqs-wm-minio-%d", time.Now().UnixNano())
	out, err := exec.Command("docker", "run", "-d", "--rm", "--name", name, "--security-opt", "label=disable", "-p", "127.0.0.1::9000",
		"-e", "MINIO_ROOT_USER="+ak, "-e", "MINIO_ROOT_PASSWORD="+sk, minioImage, "server", "/data", "--address", ":9000").CombinedOutput()
	if err != nil {
		t.Fatalf("no se pudo levantar MinIO: %v %s", err, out)
	}
	defer exec.Command("docker", "rm", "-f", name).Run()
	pb, err := exec.Command("docker", "port", name, "9000/tcp").Output()
	if err != nil {
		t.Fatal(err)
	}
	ep := strings.TrimSpace(strings.Split(string(pb), "\n")[0])
	s, err := objstore.NewS3(ep, ak, sk, "evidence", false)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	ready := false
	for i := 0; i < 60 && !ready; i++ {
		if _, err := s.Client.ListBuckets(ctx); err == nil {
			ready = true
		} else {
			time.Sleep(time.Second)
		}
	}
	if !ready {
		t.Fatal("MinIO no respondio")
	}
	if err := s.Client.MakeBucket(ctx, "evidence", minio.MakeBucketOptions{}); err != nil {
		t.Fatal(err)
	}

	// Superficie real generada por el servicio y subida a MinIO.
	pub := &wm.MemPublisher{}
	svc := &wm.Service{Surface: &wm.FakeProber{Base: "http://warm-app.aqs-test.svc", Routes: map[string]wm.FakeResponse{
		"/openapi.json": {Status: 200, Body: `{"paths":{"/orders":{"get":{},"post":{}}}}`}}},
		Objects: s, Pub: pub, Clock: wm.RealClock{}}
	sa, err := svc.InferSurface(ctx, "r-1", "t")
	if err != nil {
		t.Fatal(err)
	}
	uri := pub.Events[0].Data["surface_uri"].(string)
	if uri != "s3://evidence/r-1/surface.json" {
		t.Fatalf("uri=%s", uri)
	}
	got, err := s.Get(ctx, uri)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := json.Marshal(sa)
	if !bytes.Equal(got, want) {
		t.Fatalf("lectura distinta:\n%s\n%s", got, want)
	}
	var back wm.SurfaceArtifact
	if err := json.Unmarshal(got, &back); err != nil || len(back.Endpoints) != 2 {
		t.Fatal("contenido invalido")
	}
	if err := validateSurface(t, got); err != nil {
		t.Fatal(err)
	}
}
