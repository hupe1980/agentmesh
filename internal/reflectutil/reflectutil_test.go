package reflectutil

import "testing"

func TestIsNilOrZero(t *testing.T) {
	tests := []struct {
		name string
		v    any
		exp  bool
	}{
		{"nil interface", any(nil), true},
		{"zero int", 0, true},
		{"nonzero int", 1, false},
		{"zero bool", false, true},
		{"nonzero bool", true, false},
		{"nil slice", []string(nil), true},
		{"empty slice", []string{}, true},
		{"non-empty slice", []string{"a"}, false},
		{"nil map", map[string]int(nil), true},
		{"empty map", map[string]int{}, true},
		{"non-empty map", map[string]int{"a": 1}, false},
		{"pointer nil", (*int)(nil), true},
	}

	for _, tt := range tests {
		if got := IsNilOrZero(tt.v); got != tt.exp {
			t.Errorf("%s: expected %v, got %v", tt.name, tt.exp, got)
		}
	}
}

func TestIsNil(t *testing.T) {
	tests := []struct {
		name string
		v    any
		exp  bool
	}{
		{"nil interface", any(nil), true},
		{"zero int", 0, false},
		{"zero bool", false, false},
		{"nil slice", []string(nil), true},
		{"empty slice", []string{}, false},
		{"nil map", map[string]int(nil), true},
		{"empty map", map[string]int{}, false},
		{"pointer nil", (*int)(nil), true},
	}

	for _, tt := range tests {
		if got := IsNil(tt.v); got != tt.exp {
			t.Errorf("%s: expected %v, got %v", tt.name, tt.exp, got)
		}
	}
}
