package stubs

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const plansDir = "../../../contracts/plans"

func validate(t *testing.T, name string, v any) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(plansDir, name+".schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft2020)
	c.AssertFormat()
	if err := c.AddResource("mem:///s.json", doc); err != nil {
		t.Fatal(err)
	}
	s, err := c.Compile("mem:///s.json")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(v)
	var inst any
	_ = json.Unmarshal(raw, &inst)
	if err := s.Validate(inst); err != nil {
		t.Fatalf("%s: %v\n%s", name, err, raw)
	}
}

func TestFakeFlowSourceDefaultFails(t *testing.T) {
	if _, err := (&FakeFlowSource{}).Flows("r"); !errors.Is(err, ErrNotProgrammed) {
		t.Fatalf("err=%v", err)
	}
}
func TestFakeSurfaceDefaultFails(t *testing.T) {
	if _, err := (&FakeSurface{}).Surface("r"); !errors.Is(err, ErrNotProgrammed) {
		t.Fatalf("err=%v", err)
	}
}
func TestFakeEvidenceDefaultFails(t *testing.T) {
	if _, err := (&FakeEvidence{}).Evidence("r"); !errors.Is(err, ErrNotProgrammed) {
		t.Fatalf("err=%v", err)
	}
}

func TestStubsReturnValidPlans(t *testing.T) {
	var f FakeFlowSource
	f.Program()
	p, err := f.Flows("run-9")
	if err != nil || p.RunID != "run-9" {
		t.Fatal(err, p)
	}
	validate(t, "flow-plan", p)
	var s FakeSurface
	s.Program()
	sa, err := s.Surface("run-9")
	if err != nil {
		t.Fatal(err)
	}
	validate(t, "surface-artifact", sa)
	var e FakeEvidence
	e.Program()
	ev, err := e.Evidence("run-9")
	if err != nil {
		t.Fatal(err)
	}
	validate(t, "evidence-uris", ev)
}

func TestFakeProgrammedError(t *testing.T) {
	boom := errors.New("boom")
	var f FakeFlowSource
	f.ProgramError(boom)
	if _, err := f.Flows("r"); !errors.Is(err, boom) {
		t.Fatal(err)
	}
	var s FakeSurface
	s.ProgramError(boom)
	if _, err := s.Surface("r"); !errors.Is(err, boom) {
		t.Fatal(err)
	}
	var e FakeEvidence
	e.ProgramError(boom)
	if _, err := e.Evidence("r"); !errors.Is(err, boom) {
		t.Fatal(err)
	}
}

func TestFakeConcurrent(t *testing.T) {
	var f FakeFlowSource
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); f.Program(); _, _ = f.Flows("r") }()
	}
	wg.Wait()
}
