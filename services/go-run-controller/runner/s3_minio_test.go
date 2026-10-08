//go:build minio

package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"

	"github.com/ogaston/agentic-qa-swarm/services/go-run-controller/plan"
)

// Misma imagen (por digest) que scripts/test/minio-local.sh.
const minioImage = "cgr.dev/chainguard/minio@sha256:9dcc028b309030afa86fc1fc8d93907ae373ea3fb75277cca3fc77e4645932b7"

func startMinio(t *testing.T, bucket string) *S3 {
	t.Helper()
	const ak, sk = "testaccesskey", "testsecretkey123" // valores de prueba generados aquí
	name := fmt.Sprintf("aqs-rc-minio-%d", time.Now().UnixNano())
	out, err := exec.Command("docker", "run", "-d", "--rm", "--name", name, "--security-opt", "label=disable", "-p", "127.0.0.1::9000",
		"-e", "MINIO_ROOT_USER="+ak, "-e", "MINIO_ROOT_PASSWORD="+sk, minioImage, "server", "/data", "--address", ":9000").CombinedOutput()
	if err != nil {
		t.Fatalf("no se pudo levantar MinIO: %v %s", err, out)
	}
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", name).Run() })
	pb, err := exec.Command("docker", "port", name, "9000/tcp").Output()
	if err != nil {
		t.Fatal(err)
	}
	ep := strings.TrimSpace(strings.Split(string(pb), "\n")[0])
	d := t.TempDir()
	_ = os.WriteFile(filepath.Join(d, "ak"), []byte(ak+"\n"), 0o600)
	_ = os.WriteFile(filepath.Join(d, "sk"), []byte(sk+"\n"), 0o600)
	s, err := NewS3("http://"+ep, bucket, filepath.Join(d, "ak"), filepath.Join(d, "sk"))
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
		t.Fatal("MinIO no respondió")
	}
	return s
}

func TestEvidenceS3RoundTripAndMissingBucket(t *testing.T) {
	s := startMinio(t, "evidence")
	ctx := context.Background()
	if err := s.Client.MakeBucket(ctx, "evidence", minio.MakeBucketOptions{}); err != nil {
		t.Fatal(err)
	}
	// Dos flujos con el Launcher real sobre el S3 real.
	r := newRig("a", "b")
	r.l.Evidence = s
	run := passedRun("r-1")
	_, _ = r.l.Progress(ctx, run)
	for _, j := range r.jobs(t) {
		r.finish(t, j.Name, true)
	}
	out, err := r.l.Progress(ctx, run)
	if err != nil || !out.Done || out.FailReason != "" || len(out.URIs) != 4 {
		t.Fatalf("%+v %v", out, err)
	}
	for _, u := range out.URIs {
		if !strings.HasPrefix(u, "s3://evidence/runs/r-1/") {
			t.Fatalf("uri %s", u)
		}
	}
	// Se lee de vuelta y coincide por hash; result.json es el JSON esperado.
	b, err := s.Get(ctx, "runs/r-1/a/result.json")
	if err != nil {
		t.Fatal(err)
	}
	var fr flowResult
	if json.Unmarshal(b, &fr) != nil || fr.Status != "passed" || fr.FlowID != "a" || fr.LogsSHA256 != sha256Hex(nil) {
		t.Fatalf("result.json %s", b)
	}
	if _, err := PutVerified(ctx, s, "runs/r-1/x/y", []byte("hola")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, "runs/r-1/no-existe"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("objeto inexistente: %v", err)
	}
	raw, _ := json.Marshal(plan.EvidenceURIs{RunID: "r-1", URIs: out.URIs})
	var v any
	_ = json.Unmarshal(raw, &v)
	if err := eviSchema(t).Validate(v); err != nil {
		t.Fatalf("EvidenceURIs inválidas: %v", err)
	}

	// Bucket inexistente: la escritura falla, el flujo cuenta como fallido y no se declara evidencia.
	bad := *s
	bad.Bucket = "no-existe"
	r2 := newRig("a")
	r2.l.Evidence = &bad
	_, err = r2.l.Progress(ctx, run)
	if err == nil {
		// con un bucket inexistente ni siquiera se puede leer el resultado: nada se lanza ni se declara
		t.Fatal("avanzó con el bucket inexistente")
	}
	if len(r2.jobs(t)) != 0 {
		t.Fatal("Jobs creados con el almacén inutilizable")
	}
	// Con el bucket borrado a mitad de corrida: el flujo queda fallido (sin URIs).
	r3 := newRig("a")
	r3.l.Evidence = &flaky{S3: s, failPut: true}
	run2 := passedRun("r-2")
	_, _ = r3.l.Progress(ctx, run2)
	r3.finish(t, "runner-r-2-a", true)
	out, err = r3.l.Progress(ctx, run2)
	if err != nil || !out.Done || out.FailReason == "" || len(out.URIs) != 0 {
		t.Fatalf("%+v %v", out, err)
	}
}

// flaky delega en el S3 real pero escribe a un bucket inexistente.
type flaky struct {
	*S3
	failPut bool
}

func (f *flaky) Put(ctx context.Context, key string, data []byte) (string, error) {
	if strings.HasSuffix(key, "/started-at") { // el marcador de inicio se escribe antes de lanzar y sí sube
		return f.S3.Put(ctx, key, data)
	}
	bad := *f.S3
	bad.Bucket = "no-existe"
	return bad.Put(ctx, key, data)
}
