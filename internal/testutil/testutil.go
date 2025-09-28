package testutil

import (
	"encoding/json"
	"testing"
)

func MustJSON(t *testing.T, v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	return string(b)
}
