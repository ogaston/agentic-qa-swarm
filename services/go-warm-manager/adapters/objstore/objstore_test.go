package objstore_test

import (
	"context"
	"testing"

	"github.com/ogaston/agentic-qa-swarm/services/go-warm-manager/adapters/objstore"
)

func TestObjectStoreFileRoundTripAndTraversal(t *testing.T) {
	f := &objstore.File{Dir: t.TempDir()}
	uri, err := f.Put(context.Background(), "r-1/surface.json", []byte(`{"a":1}`))
	if err != nil {
		t.Fatal(err)
	}
	got, err := f.Get(context.Background(), uri)
	if err != nil || string(got) != `{"a":1}` {
		t.Fatalf("%q %v", got, err)
	}
	for _, bad := range []string{"../x", "a/../../x", ""} {
		if _, err := f.Put(context.Background(), bad, nil); err == nil {
			t.Errorf("clave %q aceptada", bad)
		}
	}
}
