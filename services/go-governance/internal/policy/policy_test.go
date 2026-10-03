package policy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestValidate(t *testing.T) {
	ok := map[string]string{
		Events:          `{"enabled_events":["tag","commit"]}`,
		"events_vacio":  `{"enabled_events":[]}`,
		ConfirmRequired: `{"required":true}`,
		WarmQuotas:      `{"max_runs_per_day":1000,"rebuild_cadence_hours":720,"idle_scale_down_minutes":1440,"housekeeping_grace_hours":168}`,
		Workflows:       `{"workflows":[{"name":"checkout","complexity":"low","max_runs_per_day":2,"timeout_seconds":600,"requires_approval":false}]}`,
	}
	for k, v := range ok {
		name := strings.TrimSuffix(k, "_vacio")
		if _, err := Validate(name, json.RawMessage(v)); err != nil {
			t.Errorf("%s válido rechazado: %v", k, err)
		}
	}
	q := func(a, b, c, d int) string {
		return fmt.Sprintf(`{"max_runs_per_day":%d,"rebuild_cadence_hours":%d,"idle_scale_down_minutes":%d,"housekeeping_grace_hours":%d}`, a, b, c, d)
	}
	wf := func(n, c string, m, to int, ap string) string {
		return fmt.Sprintf(`{"workflows":[{"name":%q,"complexity":%q,"max_runs_per_day":%d,"timeout_seconds":%d,"requires_approval":%s}]}`, n, c, m, to, ap)
	}
	bad := map[string][2]string{
		"evento fuera de enum":      {Events, `{"enabled_events":["push"]}`},
		"evento repetido":           {Events, `{"enabled_events":["tag","tag"]}`},
		"events sin campo":          {Events, `{}`},
		"events null":               {Events, `{"enabled_events":null}`},
		"events campo extra":        {Events, `{"enabled_events":[],"x":1}`},
		"events tipo":               {Events, `{"enabled_events":"tag"}`},
		"basura tras el valor":      {Events, `{"enabled_events":[]} {}`},
		"confirm false":             {ConfirmRequired, `{"required":false}`},
		"confirm sin campo":         {ConfirmRequired, `{}`},
		"confirm string":            {ConfirmRequired, `{"required":"true"}`},
		"cuota max 0":               {WarmQuotas, q(0, 24, 10, 24)},
		"cuota max 1001":            {WarmQuotas, q(1001, 24, 10, 24)},
		"cuota cadencia 0":          {WarmQuotas, q(1, 0, 10, 24)},
		"cuota cadencia 721":        {WarmQuotas, q(1, 721, 10, 24)},
		"cuota idle 0":              {WarmQuotas, q(1, 1, 0, 24)},
		"cuota idle 1441":           {WarmQuotas, q(1, 1, 1441, 24)},
		"cuota grace 0":             {WarmQuotas, q(1, 1, 1, 0)},
		"cuota grace 169":           {WarmQuotas, q(1, 1, 1, 169)},
		"cuota campo ausente":       {WarmQuotas, `{"max_runs_per_day":1}`},
		"workflow nombre mayúscula": {Workflows, wf("Checkout", "low", 1, 1, "false")},
		"workflow nombre dígito":    {Workflows, wf("1a", "low", 1, 1, "false")},
		"workflow nombre largo":     {Workflows, wf("a"+strings.Repeat("b", 63), "low", 1, 1, "false")},
		"workflow complexity":       {Workflows, wf("a", "extreme", 1, 1, "false")},
		"workflow max 0":            {Workflows, wf("a", "low", 0, 1, "false")},
		"workflow timeout 3601":     {Workflows, wf("a", "low", 1, 3601, "false")},
		"workflow sin approval":     {Workflows, `{"workflows":[{"name":"a","complexity":"low","max_runs_per_day":1,"timeout_seconds":1}]}`},
		"workflow repetido": {Workflows, `{"workflows":[` +
			`{"name":"a","complexity":"low","max_runs_per_day":1,"timeout_seconds":1,"requires_approval":false},` +
			`{"name":"a","complexity":"low","max_runs_per_day":1,"timeout_seconds":1,"requires_approval":false}]}`},
		"workflows ausente": {Workflows, `{}`},
	}
	for name, c := range bad {
		if _, err := Validate(c[0], json.RawMessage(c[1])); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: esperaba ErrInvalid, obtuve %v", name, err)
		}
	}
	if _, err := Validate("inventada", json.RawMessage(`{}`)); err == nil || errors.Is(err, ErrInvalid) {
		t.Errorf("política desconocida debe fallar distinto de ErrInvalid: %v", err)
	}
}

func TestValidateNormalizes(t *testing.T) {
	a, _ := Validate(Events, json.RawMessage(`{ "enabled_events" : [ "tag" ] }`))
	b, _ := Validate(Events, json.RawMessage(`{"enabled_events":["tag"]}`))
	if string(a) != string(b) || string(a) != `{"enabled_events":["tag"]}` {
		t.Fatalf("%s %s", a, b)
	}
}

func open(t *testing.T) (*FileStore, string) {
	p := filepath.Join(t.TempDir(), "policies.jsonl")
	s, err := OpenFileStore(p, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, p
}

func TestStoreVersioningAndIdempotence(t *testing.T) {
	ctx := context.Background()
	s, p := open(t)
	if _, found, _ := s.Get(ctx, Events); found {
		t.Fatal("sin valores por defecto implícitos")
	}
	v1, n1, _ := s.Put(ctx, Events, json.RawMessage(`{"enabled_events":["tag"]}`), "a1")
	v1b, n1b, _ := s.Put(ctx, Events, json.RawMessage(`{"enabled_events":["tag"]}`), "a1")
	v2, n2, _ := s.Put(ctx, Events, json.RawMessage(`{"enabled_events":[]}`), "a1")
	if v1 != 1 || !n1 || v1b != 1 || n1b || v2 != 2 || !n2 {
		t.Fatalf("%d%v %d%v %d%v", v1, n1, v1b, n1b, v2, n2)
	}
	if pv, isNew, _ := s.Plan(ctx, Events, json.RawMessage(`{"enabled_events":["commit"]}`)); pv != 3 || !isNew {
		t.Fatal("Plan")
	}
	if b, _ := os.ReadFile(p); strings.Count(string(b), "\n") != 2 {
		t.Fatal("solo deben guardarse 2 versiones")
	}
	s.Close()
	s2, err := OpenFileStore(p, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if v, found, _ := s2.Get(ctx, Events); !found || v.Version != 2 || v.Actor != "a1" {
		t.Fatalf("reapertura: %+v", v)
	}
}

func TestStoreConcurrentPutsConsecutive(t *testing.T) {
	ctx := context.Background()
	s, _ := open(t)
	var wg sync.WaitGroup
	got := make(chan int, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			v, _, err := s.Put(ctx, WarmQuotas, json.RawMessage(fmt.Sprintf(`{"max_runs_per_day":%d}`, i+1)), "a")
			if err != nil {
				t.Error(err)
			}
			got <- v
		}(i)
	}
	wg.Wait()
	close(got)
	seen := map[int]bool{}
	for v := range got {
		seen[v] = true
	}
	for i := 1; i <= 20; i++ {
		if !seen[i] {
			t.Fatalf("falta la versión %d: %v", i, seen)
		}
	}
}

func TestStoreCorruptFileRefuses(t *testing.T) {
	for name, content := range map[string]string{
		"json roto":        "{no\n",
		"truncada":         `{"name":"events","version":1,"value":{}}`,
		"nombre":           `{"name":"x","version":1,"value":{}}` + "\n",
		"salto de versión": `{"name":"events","version":2,"value":{}}` + "\n",
	} {
		p := filepath.Join(t.TempDir(), "p.jsonl")
		os.WriteFile(p, []byte(content), 0o600)
		if s, err := OpenFileStore(p, nil); err == nil {
			s.Close()
			t.Errorf("%s: debía negarse a abrir", name)
		}
	}
}

func TestStoreCancelledContext(t *testing.T) {
	s, _ := open(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := s.Get(ctx, Events); err == nil {
		t.Fatal("Get con contexto cancelado")
	}
	if _, _, err := s.Put(ctx, Events, json.RawMessage(`{}`), "a"); err == nil {
		t.Fatal("Put con contexto cancelado")
	}
}
