package artifact

import (
	"fmt"
	"sync"
	"testing"

	"github.com/hupe1980/agentmesh/core"
)

// Interface compliance (compile-time assertions)
var _ core.ArtifactStore = (*InMemoryStore)(nil)

func TestInMemoryArtifactStore_SaveGetIsolation(t *testing.T) { // keep test name for history
	svc := NewInMemoryStore()
	data := []byte("hello")
	if err := svc.Save("s1", "a1", data); err != nil {
		t.Fatalf("save: %v", err)
	}
	// mutate original slice
	data[0] = 'H'
	out, err := svc.Get("s1", "a1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if string(out) != "hello" { // should not reflect mutation
		t.Fatalf("expected 'hello', got %q", string(out))
	}
	// mutate returned slice
	out[0] = 'x'
	out2, _ := svc.Get("s1", "a1")
	if string(out2) != "hello" { // original stored should be unchanged
		t.Fatalf("expected isolation, got %q", string(out2))
	}
}

func TestInMemoryArtifactStore_ListAndDelete(t *testing.T) { // name retained
	svc := NewInMemoryStore()
	if err := svc.Save("s1", "a1", []byte("1")); err != nil {
		t.Fatal(err)
	}
	if err := svc.Save("s1", "a2", []byte("2")); err != nil {
		t.Fatal(err)
	}
	ids, err := svc.List("s1")
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 ids, got %d", len(ids))
	}
	if err := svc.Delete("s1", "a1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := svc.Get("s1", "a1"); err == nil {
		t.Fatalf("expected error for deleted artifact")
	}
	ids, _ = svc.List("s1")
	if len(ids) != 1 {
		t.Fatalf("expected 1 id after delete, got %d", len(ids))
	}
}

func TestInMemoryArtifactStore_Concurrency(t *testing.T) { // name retained
	svc := NewInMemoryStore()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			id := i % 10
			if err := svc.Save("s1", fmt.Sprintf("a%d", id), []byte("data")); err != nil {
				t.Errorf("save err: %v", err)
			}
			_, _ = svc.List("s1")
		}()
	}
	wg.Wait()
	ids, err := svc.List("s1")
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) == 0 {
		t.Fatalf("expected some artifacts, got 0")
	}
}
