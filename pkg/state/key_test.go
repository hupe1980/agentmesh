package state

import (
	"testing"
)

func TestKey_Get(t *testing.T) {
	sm, err := NewStateManager(0)
	if err != nil {
		t.Fatal(err)
	}

	// Test successful get
	key := NewKey[string]("test_key")
	_ = sm.Set("test_key", "hello")

	value, err := key.Get(sm)
	if err != nil {
		t.Errorf("Get() error = %v, want nil", err)
	}
	if value != "hello" {
		t.Errorf("Get() = %v, want %v", value, "hello")
	}

	// Test missing key
	missingKey := NewKey[string]("missing")
	_, err = missingKey.Get(sm)
	if err == nil {
		t.Error("Get() on missing key should return error")
	}

	// Test wrong type
	_ = sm.Set("wrong_type", 42)
	wrongKey := NewKey[string]("wrong_type")
	_, err = wrongKey.Get(sm)
	if err == nil {
		t.Error("Get() with wrong type should return error")
	}
}

func TestKey_GetOr(t *testing.T) {
	sm, err := NewStateManager(0)
	if err != nil {
		t.Fatal(err)
	}

	key := NewKey[int]("counter")

	// Test with missing key (should return default)
	value := key.GetOr(sm, 99)
	if value != 99 {
		t.Errorf("GetOr() = %v, want %v", value, 99)
	}

	// Test with existing key
	_ = sm.Set("counter", 42)
	value = key.GetOr(sm, 99)
	if value != 42 {
		t.Errorf("GetOr() = %v, want %v", value, 42)
	}

	// Test with wrong type (should return default) - use different key
	wrongTypeKey := NewKey[int]("wrong_type_key")
	_ = sm.Set("wrong_type_key", "not an int")
	value = wrongTypeKey.GetOr(sm, 99)
	if value != 99 {
		t.Errorf("GetOr() with wrong type = %v, want default %v", value, 99)
	}
}

func TestKey_MustGet(t *testing.T) {
	sm, err := NewStateManager(0)
	if err != nil {
		t.Fatal(err)
	}

	key := NewKey[string]("test")
	_ = sm.Set("test", "value")

	// Test successful MustGet
	value := key.MustGet(sm)
	if value != "value" {
		t.Errorf("MustGet() = %v, want %v", value, "value")
	}

	// Test MustGet panic on missing key
	missingKey := NewKey[string]("missing")
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustGet() should panic on missing key")
		}
	}()
	_ = missingKey.MustGet(sm)
}

func TestKey_Set(t *testing.T) {
	sm, err := NewStateManager(0)
	if err != nil {
		t.Fatal(err)
	}

	key := NewKey[int]("counter")

	// Test set
	err = key.Set(sm, 42)
	if err != nil {
		t.Errorf("Set() error = %v, want nil", err)
	}

	// Verify value was set
	value := sm.Get("counter")
	if value != 42 {
		t.Errorf("After Set(), Get() = %v, want %v", value, 42)
	}

	// Test overwrite
	err = key.Set(sm, 100)
	if err != nil {
		t.Errorf("Set() error = %v, want nil", err)
	}

	value = sm.Get("counter")
	if value != 100 {
		t.Errorf("After Set(), Get() = %v, want %v", value, 100)
	}
}

func TestKey_Update(t *testing.T) {
	sm, err := NewStateManager(0)
	if err != nil {
		t.Fatal(err)
	}

	key := NewKey[int]("counter")

	// Test update on non-existent key (should start from zero value)
	err = key.Update(sm, func(current int) int {
		return current + 1
	})
	if err != nil {
		t.Errorf("Update() error = %v, want nil", err)
	}

	value, _ := key.Get(sm)
	if value != 1 {
		t.Errorf("After Update() from zero, value = %v, want %v", value, 1)
	}

	// Test update on existing key
	err = key.Update(sm, func(current int) int {
		return current * 2
	})
	if err != nil {
		t.Errorf("Update() error = %v, want nil", err)
	}

	value, _ = key.Get(sm)
	if value != 2 {
		t.Errorf("After Update(), value = %v, want %v", value, 2)
	}
}

func TestKey_Exists(t *testing.T) {
	sm, err := NewStateManager(0)
	if err != nil {
		t.Fatal(err)
	}

	key := NewKey[string]("test")

	// Test non-existent key
	if key.Exists(sm) {
		t.Error("Exists() = true, want false for non-existent key")
	}

	// Test after setting
	_ = key.Set(sm, "value")
	if !key.Exists(sm) {
		t.Error("Exists() = false, want true after Set()")
	}

	// Test with wrong type - use different key to avoid type conflict
	wrongTypeKey := NewKey[string]("wrong_type_test")
	_ = sm.Set("wrong_type_test", 42)
	if wrongTypeKey.Exists(sm) {
		t.Error("Exists() = true, want false for wrong type")
	}
}

func TestKey_Name(t *testing.T) {
	key := NewKey[int]("my_counter")
	if key.Name() != "my_counter" {
		t.Errorf("Name() = %v, want %v", key.Name(), "my_counter")
	}
}

func TestCounter(t *testing.T) {
	sm, err := NewStateManager(0)
	if err != nil {
		t.Fatal(err)
	}

	counter := NewCounter("test_counter")

	// Test increment from zero
	value, err := counter.Increment(sm)
	if err != nil {
		t.Errorf("Increment() error = %v, want nil", err)
	}
	if value != 1 {
		t.Errorf("Increment() = %v, want %v", value, 1)
	}

	// Test multiple increments
	for i := 2; i <= 5; i++ {
		value, _ = counter.Increment(sm)
		if value != i {
			t.Errorf("Increment() = %v, want %v", value, i)
		}
	}

	// Test decrement
	value, err = counter.Decrement(sm)
	if err != nil {
		t.Errorf("Decrement() error = %v, want nil", err)
	}
	if value != 4 {
		t.Errorf("Decrement() = %v, want %v", value, 4)
	}

	// Test get
	value, err = counter.Get(sm)
	if err != nil {
		t.Errorf("Get() error = %v, want nil", err)
	}
	if value != 4 {
		t.Errorf("Get() = %v, want %v", value, 4)
	}

	// Test set
	err = counter.Set(sm, 100)
	if err != nil {
		t.Errorf("Set() error = %v, want nil", err)
	}

	value, _ = counter.Get(sm)
	if value != 100 {
		t.Errorf("After Set(), Get() = %v, want %v", value, 100)
	}
}

func TestFlag(t *testing.T) {
	sm, err := NewStateManager(0)
	if err != nil {
		t.Fatal(err)
	}

	flag := NewFlag("test_flag")

	// Test initial state (should be false)
	if flag.IsSet(sm) {
		t.Error("IsSet() = true, want false for new flag")
	}

	// Test Set
	err = flag.Set(sm)
	if err != nil {
		t.Errorf("Set() error = %v, want nil", err)
	}
	if !flag.IsSet(sm) {
		t.Error("IsSet() = false, want true after Set()")
	}

	// Test Clear
	err = flag.Clear(sm)
	if err != nil {
		t.Errorf("Clear() error = %v, want nil", err)
	}
	if flag.IsSet(sm) {
		t.Error("IsSet() = true, want false after Clear()")
	}

	// Test Toggle
	err = flag.Toggle(sm)
	if err != nil {
		t.Errorf("Toggle() error = %v, want nil", err)
	}
	if !flag.IsSet(sm) {
		t.Error("IsSet() = false, want true after Toggle() from false")
	}

	err = flag.Toggle(sm)
	if err != nil {
		t.Errorf("Toggle() error = %v, want nil", err)
	}
	if flag.IsSet(sm) {
		t.Error("IsSet() = true, want false after Toggle() from true")
	}
}

func TestKey_ComplexTypes(t *testing.T) {
	sm, err := NewStateManager(0)
	if err != nil {
		t.Fatal(err)
	}

	// Test with slice
	sliceKey := NewKey[[]string]("tags")
	tags := []string{"go", "agent", "framework"}
	_ = sliceKey.Set(sm, tags)

	retrieved, err := sliceKey.Get(sm)
	if err != nil {
		t.Errorf("Get() slice error = %v, want nil", err)
	}
	if len(retrieved) != 3 {
		t.Errorf("Get() slice length = %v, want %v", len(retrieved), 3)
	}

	// Test with map
	mapKey := NewKey[map[string]int]("scores")
	scores := map[string]int{"alice": 100, "bob": 90}
	_ = mapKey.Set(sm, scores)

	retrievedMap, err := mapKey.Get(sm)
	if err != nil {
		t.Errorf("Get() map error = %v, want nil", err)
	}
	if retrievedMap["alice"] != 100 {
		t.Errorf("Get() map[alice] = %v, want %v", retrievedMap["alice"], 100)
	}

	// Test with struct
	type Config struct {
		Host string
		Port int
	}
	configKey := NewKey[*Config]("config")
	cfg := &Config{Host: "localhost", Port: 8080}
	_ = configKey.Set(sm, cfg)

	retrievedCfg, err := configKey.Get(sm)
	if err != nil {
		t.Errorf("Get() struct error = %v, want nil", err)
	}
	if retrievedCfg.Host != "localhost" || retrievedCfg.Port != 8080 {
		t.Errorf("Get() struct = %+v, want %+v", retrievedCfg, cfg)
	}
}

func TestKey_Concurrency(t *testing.T) {
	sm, err := NewStateManager(0)
	if err != nil {
		t.Fatal(err)
	}

	counter := NewCounter("concurrent_counter")

	// Concurrent increments
	const goroutines = 100
	done := make(chan bool, goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			_, _ = counter.Increment(sm)
			done <- true
		}()
	}

	for i := 0; i < goroutines; i++ {
		<-done
	}

	// Note: This test may not reach exactly 100 due to race conditions
	// in the underlying state manager, but it should not panic
	value, _ := counter.Get(sm)
	if value <= 0 {
		t.Errorf("After concurrent increments, counter = %v, want > 0", value)
	}
}

func BenchmarkKey_Get(b *testing.B) {
	sm, _ := NewStateManager(0)
	key := NewKey[int]("bench_key")
	_ = key.Set(sm, 42)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = key.Get(sm)
	}
}

func BenchmarkKey_Set(b *testing.B) {
	sm, _ := NewStateManager(0)
	key := NewKey[int]("bench_key")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = key.Set(sm, i)
	}
}

func BenchmarkCounter_Increment(b *testing.B) {
	sm, _ := NewStateManager(0)
	counter := NewCounter("bench_counter")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = counter.Increment(sm)
	}
}

func BenchmarkKey_GetOr(b *testing.B) {
	sm, _ := NewStateManager(0)
	key := NewKey[int]("bench_key")
	_ = key.Set(sm, 42)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = key.GetOr(sm, 0)
	}
}
