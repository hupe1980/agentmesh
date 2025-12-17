package schema

import (
	"testing"
)

func TestNewOutputSchema_Struct(t *testing.T) {
	type Person struct {
		Name string `json:"name" jsonschema:"required,description=Person's name"`
		Age  int    `json:"age" jsonschema:"required,minimum=0,maximum=150"`
	}

	output, err := NewOutputSchema("person", Person{})
	if err != nil {
		t.Fatalf("NewOutputSchema failed: %v", err)
	}

	if output.Name != "person" {
		t.Errorf("Expected name 'person', got '%s'", output.Name)
	}

	if !output.Strict {
		t.Error("Expected Strict to be true by default")
	}

	if output.Schema == nil {
		t.Fatal("Schema should not be nil")
	}

	// Verify schema has required fields
	if _, ok := output.Schema["type"]; !ok {
		t.Error("Schema missing 'type' field")
	}
	if _, ok := output.Schema["properties"]; !ok {
		t.Error("Schema missing 'properties' field")
	}
	if _, ok := output.Schema["required"]; !ok {
		t.Error("Schema missing 'required' field")
	}
}

func TestNewOutputSchema_Map(t *testing.T) {
	schemaMap := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"type": "string"},
		},
		"required": []string{"name"},
	}

	output, err := NewOutputSchema("test", schemaMap)
	if err != nil {
		t.Fatalf("NewOutputSchema failed: %v", err)
	}

	if output.Name != "test" {
		t.Errorf("Expected name 'test', got '%s'", output.Name)
	}
}

func TestNewOutputSchema_WithOptions(t *testing.T) {
	type Simple struct {
		Value string `json:"value" jsonschema:"required"`
	}

	output, err := NewOutputSchema("test", Simple{},
		WithStrict(false),
		WithDescription("Test schema"),
		WithAllowAdditionalProperties(true),
	)
	if err != nil {
		t.Fatalf("NewOutputSchema failed: %v", err)
	}

	if output.Strict {
		t.Error("Expected Strict to be false")
	}

	if output.Description != "Test schema" {
		t.Errorf("Expected description 'Test schema', got '%s'", output.Description)
	}

	if allow, ok := output.Schema["additionalProperties"].(bool); !ok || !allow {
		t.Error("Expected additionalProperties to be true")
	}
}

// ValidationPolicy tests
func TestValidationDisabled(t *testing.T) {
	policy := ValidationDisabled()

	if policy.Enabled {
		t.Error("Expected Enabled to be false")
	}

	if policy.MaxRetries != 0 {
		t.Errorf("Expected MaxRetries to be 0, got %d", policy.MaxRetries)
	}

	// OnFailure doesn't matter when validation is disabled
}

func TestValidationStrict(t *testing.T) {
	policy := ValidationStrict()

	if !policy.Enabled {
		t.Error("Expected Enabled to be true")
	}

	if policy.MaxRetries != 0 {
		t.Errorf("Expected MaxRetries to be 0, got %d", policy.MaxRetries)
	}

	if policy.OnFailure != FailOnError {
		t.Errorf("Expected OnFailure to be FailOnError, got %v", policy.OnFailure)
	}
}

func TestValidationWithRetry(t *testing.T) {
	tests := []struct {
		name       string
		maxRetries int
	}{
		{"zero retries", 0},
		{"one retry", 1},
		{"three retries", 3},
		{"five retries", 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := ValidationWithRetry(tt.maxRetries)

			if !policy.Enabled {
				t.Error("Expected Enabled to be true")
			}

			if policy.MaxRetries != tt.maxRetries {
				t.Errorf("Expected MaxRetries to be %d, got %d", tt.maxRetries, policy.MaxRetries)
			}

			if policy.OnFailure != FailOnError {
				t.Errorf("Expected OnFailure to be FailOnError, got %v", policy.OnFailure)
			}
		})
	}
}

func TestValidationWarnOnly(t *testing.T) {
	policy := ValidationWarnOnly()

	if !policy.Enabled {
		t.Error("Expected Enabled to be true")
	}

	if policy.MaxRetries != 0 {
		t.Errorf("Expected MaxRetries to be 0, got %d", policy.MaxRetries)
	}

	if policy.OnFailure != WarnOnError {
		t.Errorf("Expected OnFailure to be WarnOnError, got %v", policy.OnFailure)
	}
}

func TestWithValidationPolicy(t *testing.T) {
	type Simple struct {
		Value string `json:"value" jsonschema:"required"`
	}

	t.Run("applies ValidationStrict policy", func(t *testing.T) {
		output, err := NewOutputSchema("test", Simple{},
			WithValidationPolicy(ValidationStrict()),
		)
		if err != nil {
			t.Fatalf("NewOutputSchema failed: %v", err)
		}

		if output.Validation == nil {
			t.Fatal("Expected Validation to be set")
		}

		if !output.Validation.Enabled {
			t.Error("Expected Validation.Enabled to be true")
		}

		if output.Validation.MaxRetries != 0 {
			t.Errorf("Expected MaxRetries to be 0, got %d", output.Validation.MaxRetries)
		}

		if output.Validation.OnFailure != FailOnError {
			t.Errorf("Expected OnFailure to be FailOnError, got %v", output.Validation.OnFailure)
		}
	})

	t.Run("applies ValidationWithRetry policy", func(t *testing.T) {
		output, err := NewOutputSchema("test", Simple{},
			WithValidationPolicy(ValidationWithRetry(3)),
		)
		if err != nil {
			t.Fatalf("NewOutputSchema failed: %v", err)
		}

		if output.Validation == nil {
			t.Fatal("Expected Validation to be set")
		}

		if !output.Validation.Enabled {
			t.Error("Expected Validation.Enabled to be true")
		}

		if output.Validation.MaxRetries != 3 {
			t.Errorf("Expected MaxRetries to be 3, got %d", output.Validation.MaxRetries)
		}
	})

	t.Run("applies ValidationWarnOnly policy", func(t *testing.T) {
		output, err := NewOutputSchema("test", Simple{},
			WithValidationPolicy(ValidationWarnOnly()),
		)
		if err != nil {
			t.Fatalf("NewOutputSchema failed: %v", err)
		}

		if output.Validation == nil {
			t.Fatal("Expected Validation to be set")
		}

		if output.Validation.OnFailure != WarnOnError {
			t.Errorf("Expected OnFailure to be WarnOnError, got %v", output.Validation.OnFailure)
		}
	})

	t.Run("applies ValidationDisabled policy", func(t *testing.T) {
		output, err := NewOutputSchema("test", Simple{},
			WithValidationPolicy(ValidationDisabled()),
		)
		if err != nil {
			t.Fatalf("NewOutputSchema failed: %v", err)
		}

		if output.Validation == nil {
			t.Fatal("Expected Validation to be set")
		}

		if output.Validation.Enabled {
			t.Error("Expected Validation.Enabled to be false")
		}
	})

	t.Run("nil Validation when no policy set", func(t *testing.T) {
		output, err := NewOutputSchema("test", Simple{})
		if err != nil {
			t.Fatalf("NewOutputSchema failed: %v", err)
		}

		if output.Validation != nil {
			t.Error("Expected Validation to be nil when no policy set")
		}
	})
}

func TestFailureAction_String(t *testing.T) {
	tests := []struct {
		action   FailureAction
		expected string
	}{
		{FailOnError, "fail"},
		{WarnOnError, "warn"},
		{IgnoreOnError, "ignore"},
		{FailureAction("unknown"), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if string(tt.action) != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, tt.action)
			}
		})
	}
}

func TestValidationPolicyCustom(t *testing.T) {
	// Test creating custom ValidationPolicy directly
	policy := &ValidationPolicy{
		Enabled:    true,
		MaxRetries: 5,
		OnFailure:  IgnoreOnError,
	}

	if !policy.Enabled {
		t.Error("Expected Enabled to be true")
	}

	if policy.MaxRetries != 5 {
		t.Errorf("Expected MaxRetries to be 5, got %d", policy.MaxRetries)
	}

	if policy.OnFailure != IgnoreOnError {
		t.Errorf("Expected OnFailure to be IgnoreOnError, got %v", policy.OnFailure)
	}
}

func TestWithValidationPolicy_CombinedWithOtherOptions(t *testing.T) {
	type Simple struct {
		Value string `json:"value" jsonschema:"required"`
	}

	output, err := NewOutputSchema("test", Simple{},
		WithStrict(false),
		WithDescription("Test schema"),
		WithValidationPolicy(ValidationWithRetry(2)),
	)
	if err != nil {
		t.Fatalf("NewOutputSchema failed: %v", err)
	}

	// Verify all options are applied
	if output.Strict {
		t.Error("Expected Strict to be false")
	}

	if output.Description != "Test schema" {
		t.Errorf("Expected description 'Test schema', got '%s'", output.Description)
	}

	if output.Validation == nil {
		t.Fatal("Expected Validation to be set")
	}

	if output.Validation.MaxRetries != 2 {
		t.Errorf("Expected MaxRetries to be 2, got %d", output.Validation.MaxRetries)
	}
}
