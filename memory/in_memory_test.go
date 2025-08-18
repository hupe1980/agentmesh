package memory

import (
	"sync"
	"testing"

	"github.com/hupe1980/agentmesh/core"
)

// Interface compliance (compile-time assertions)
var _ core.MemoryStore = (*InMemoryStore)(nil)

func TestInMemoryMemoryStore_GetAndPut(t *testing.T) { // name retained for history
	svc := NewInMemoryStore()
	m, err := svc.Get("s1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m) != 0 {
		t.Fatalf("expected empty memory, got %#v", m)
	}
	if err := svc.Put("s1", map[string]any{"k1": "v1", "k2": 2}); err != nil {
		t.Fatalf("update failed: %v", err)
	}
	m2, _ := svc.Get("s1")
	if len(m2) != 2 || m2["k1"] != "v1" || m2["k2"].(int) != 2 {
		t.Fatalf("unexpected memory contents: %#v", m2)
	}
	// mutation safety (returned map is a copy)
	m2["k1"] = "changed"
	m3, _ := svc.Get("s1")
	if m3["k1"] != "v1" {
		t.Fatalf("expected copy isolation, got %#v", m3["k1"])
	}
}

func TestInMemoryMemoryStore_StoreSearchDelete(t *testing.T) { // name retained
	svc := NewInMemoryStore()
	// store memories
	for i := 0; i < 5; i++ {
		if err := svc.Store("s2", "content"+string(rune('A'+i)), map[string]any{"idx": i}); err != nil {
			t.Fatalf("store failed: %v", err)
		}
	}
	// search all (empty query) limit larger than stored
	res, err := svc.Search("s2", "", 10)
	if err != nil {
		t.Fatalf("search all failed: %v", err)
	}
	if len(res) != 5 {
		t.Fatalf("expected 5 results, got %d", len(res))
	}
	// search with query substring
	res2, _ := svc.Search("s2", "contentA", 5)
	if len(res2) != 1 || res2[0].Content == "" {
		t.Fatalf("expected single match, got %#v", res2)
	}
	// limit test
	res3, _ := svc.Search("s2", "", 3)
	if len(res3) != 3 {
		t.Fatalf("expected 3 limited results, got %d", len(res3))
	}
	// delete existing id (take first)
	if len(res) > 0 {
		if err := svc.Delete("s2", res[0].ID); err != nil {
			t.Fatalf("delete existing failed: %v", err)
		}
	}
	res4, _ := svc.Search("s2", "", 10)
	if len(res4) != 4 {
		t.Fatalf("expected 4 after delete, got %d", len(res4))
	}
	// delete nonexistent
	if err := svc.Delete("s2", "does_not_exist"); err == nil {
		t.Fatalf("expected error deleting nonexistent memory")
	}
}

func TestInMemoryMemoryStore_ConcurrentAccess(t *testing.T) { // name retained
	svc := NewInMemoryStore()
	wg := sync.WaitGroup{}
	for i := 0; i < 25; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := svc.Put("s4", map[string]any{string(rune('A' + (i % 5))): i}); err != nil {
				t.Errorf("update error: %v", err)
			}
			if _, err := svc.Get("s4"); err != nil {
				t.Errorf("get error: %v", err)
			}
			if _, err := svc.Search("s4", "", 5); err != nil {
				t.Errorf("search mem error: %v", err)
			}
		}(i)
	}
	wg.Wait()
	// final read
	m, _ := svc.Get("s4")
	if len(m) == 0 {
		t.Fatalf("expected keys after concurrent updates")
	}
}
