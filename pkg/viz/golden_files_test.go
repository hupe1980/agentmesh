package viz

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGoldenFileManager_NewGoldenFileManager(t *testing.T) {
	gfm := NewGoldenFileManager("/tmp/golden")

	require.NotNil(t, gfm)
	require.Equal(t, "/tmp/golden", gfm.baseDir)
}

func TestGoldenFileManager_SaveAndLoad(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()
	gfm := NewGoldenFileManager(tmpDir)

	suiteID := "test-suite"
	testName := "test-case"

	output := map[string]any{
		"result": "success",
		"count":  42,
		"data":   []any{"a", "b", "c"},
	}

	// Save golden file
	err := gfm.Save(suiteID, testName, output)
	require.NoError(t, err)

	// Verify file exists
	require.True(t, gfm.Exists(suiteID, testName))

	// Load golden file
	loaded, err := gfm.Load(suiteID, testName)
	require.NoError(t, err)
	require.NotNil(t, loaded)

	// Verify content
	require.Equal(t, "success", loaded["result"])
	require.Equal(t, float64(42), loaded["count"]) // JSON unmarshals numbers as float64
	require.Len(t, loaded["data"], 3)
}

func TestGoldenFileManager_LoadNonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	gfm := NewGoldenFileManager(tmpDir)

	// Try to load non-existent file
	_, err := gfm.Load("suite", "nonexistent")
	require.Error(t, err)
	require.Contains(t, err.Error(), "golden file does not exist")
}

func TestGoldenFileManager_Exists(t *testing.T) {
	tmpDir := t.TempDir()
	gfm := NewGoldenFileManager(tmpDir)

	suiteID := "suite1"
	testName := "test1"

	// Should not exist initially
	require.False(t, gfm.Exists(suiteID, testName))

	// Save file
	err := gfm.Save(suiteID, testName, map[string]any{"key": "value"})
	require.NoError(t, err)

	// Should exist now
	require.True(t, gfm.Exists(suiteID, testName))
}

func TestGoldenFileManager_Delete(t *testing.T) {
	tmpDir := t.TempDir()
	gfm := NewGoldenFileManager(tmpDir)

	suiteID := "suite1"
	testName := "test1"

	// Create golden file
	err := gfm.Save(suiteID, testName, map[string]any{"key": "value"})
	require.NoError(t, err)
	require.True(t, gfm.Exists(suiteID, testName))

	// Delete it
	err = gfm.Delete(suiteID, testName)
	require.NoError(t, err)
	require.False(t, gfm.Exists(suiteID, testName))

	// Deleting again should not error
	err = gfm.Delete(suiteID, testName)
	require.NoError(t, err)
}

func TestGoldenFileManager_List(t *testing.T) {
	tmpDir := t.TempDir()
	gfm := NewGoldenFileManager(tmpDir)

	suiteID := "suite1"

	// Empty suite
	tests, err := gfm.List(suiteID)
	require.NoError(t, err)
	require.Empty(t, tests)

	// Create some golden files
	err = gfm.Save(suiteID, "test1", map[string]any{"a": 1})
	require.NoError(t, err)

	err = gfm.Save(suiteID, "test2", map[string]any{"b": 2})
	require.NoError(t, err)

	err = gfm.Save(suiteID, "test3", map[string]any{"c": 3})
	require.NoError(t, err)

	// List them
	tests, err = gfm.List(suiteID)
	require.NoError(t, err)
	require.Len(t, tests, 3)
	require.Contains(t, tests, "test1")
	require.Contains(t, tests, "test2")
	require.Contains(t, tests, "test3")
}

func TestGoldenFileManager_SanitizeName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "simple"},
		{"with space", "with_space"},
		{"with/slash", "with_slash"},
		{"with\\backslash", "with_backslash"},
		{"with:colon", "with_colon"},
		{"with*asterisk", "with_asterisk"},
		{"with?question", "with_question"},
		{"with\"quote", "with_quote"},
		{"with<less", "with_less"},
		{"with>greater", "with_greater"},
		{"with|pipe", "with_pipe"},
		{"complex/path:name*test", "complex_path_name_test"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := sanitizeName(tt.input)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestGoldenFileManager_GetPath(t *testing.T) {
	tmpDir := t.TempDir()
	gfm := NewGoldenFileManager(tmpDir)

	path := gfm.GetPath("suite1", "test1")

	expected := filepath.Join(tmpDir, "suite1", "test1.golden.json")
	require.Equal(t, expected, path)
}

func TestGoldenFileManager_LoadOrCreate(t *testing.T) {
	tmpDir := t.TempDir()
	gfm := NewGoldenFileManager(tmpDir)

	suiteID := "suite1"
	testName := "test1"
	output := map[string]any{"result": "success"}

	// First call should create the file
	loaded, created, err := gfm.LoadOrCreate(suiteID, testName, output)
	require.NoError(t, err)
	require.True(t, created)
	require.Equal(t, output["result"], loaded["result"])

	// Second call should load existing
	loaded, created, err = gfm.LoadOrCreate(suiteID, testName, map[string]any{"different": "data"})
	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, "success", loaded["result"])
	require.NotContains(t, loaded, "different")
}

func TestGoldenFileManager_Compare(t *testing.T) {
	tmpDir := t.TempDir()
	gfm := NewGoldenFileManager(tmpDir)

	suiteID := "suite1"
	testName := "test1"

	// Save golden file
	expected := map[string]any{
		"result": "success",
		"count":  10,
	}
	err := gfm.Save(suiteID, testName, expected)
	require.NoError(t, err)

	// Compare with identical output
	actual := map[string]any{
		"result": "success",
		"count":  float64(10), // JSON unmarshals as float64
	}
	diffs, err := gfm.Compare(suiteID, testName, actual)
	require.NoError(t, err)
	require.Empty(t, diffs)

	// Compare with different output
	actual = map[string]any{
		"result": "failure",
		"count":  float64(20),
	}
	diffs, err = gfm.Compare(suiteID, testName, actual)
	require.NoError(t, err)
	require.NotEmpty(t, diffs)
	require.GreaterOrEqual(t, len(diffs), 2) // result and count changed
}

func TestGoldenFileManager_Update(t *testing.T) {
	tmpDir := t.TempDir()
	gfm := NewGoldenFileManager(tmpDir)

	suiteID := "suite1"
	testName := "test1"

	// Create initial golden file
	initial := map[string]any{"version": "1.0"}
	err := gfm.Save(suiteID, testName, initial)
	require.NoError(t, err)

	// Load and verify
	loaded, err := gfm.Load(suiteID, testName)
	require.NoError(t, err)
	require.Equal(t, "1.0", loaded["version"])

	// Update golden file
	updated := map[string]any{"version": "2.0"}
	err = gfm.Update(suiteID, testName, updated)
	require.NoError(t, err)

	// Load and verify update
	loaded, err = gfm.Load(suiteID, testName)
	require.NoError(t, err)
	require.Equal(t, "2.0", loaded["version"])
}

func TestGoldenFileManager_UpdateAll(t *testing.T) {
	tmpDir := t.TempDir()
	gfm := NewGoldenFileManager(tmpDir)

	results := []TestRun{
		{
			SuiteID:  "suite1",
			TestName: "test1",
			Actual:   map[string]any{"result": "a"},
		},
		{
			SuiteID:  "suite1",
			TestName: "test2",
			Actual:   map[string]any{"result": "b"},
		},
		{
			SuiteID:  "suite2",
			TestName: "test1",
			Actual:   map[string]any{"result": "c"},
		},
	}

	// Update all
	err := gfm.UpdateAll(results)
	require.NoError(t, err)

	// Verify all were created
	loaded, err := gfm.Load("suite1", "test1")
	require.NoError(t, err)
	require.Equal(t, "a", loaded["result"])

	loaded, err = gfm.Load("suite1", "test2")
	require.NoError(t, err)
	require.Equal(t, "b", loaded["result"])

	loaded, err = gfm.Load("suite2", "test1")
	require.NoError(t, err)
	require.Equal(t, "c", loaded["result"])
}

func TestGoldenFileManager_DirectoryCreation(t *testing.T) {
	tmpDir := t.TempDir()
	gfm := NewGoldenFileManager(tmpDir)

	// Save to nested suite (should create directories)
	err := gfm.Save("suite1", "test1", map[string]any{"data": "test"})
	require.NoError(t, err)

	// Verify directory was created
	suiteDir := filepath.Join(tmpDir, "suite1")
	info, err := os.Stat(suiteDir)
	require.NoError(t, err)
	require.True(t, info.IsDir())
}

func TestGoldenFileManager_JSONFormatting(t *testing.T) {
	tmpDir := t.TempDir()
	gfm := NewGoldenFileManager(tmpDir)

	suiteID := "suite1"
	testName := "test1"

	output := map[string]any{
		"nested": map[string]any{
			"key": "value",
		},
	}

	err := gfm.Save(suiteID, testName, output)
	require.NoError(t, err)

	// Read raw file content
	path := gfm.GetPath(suiteID, testName)
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	// Verify it's pretty-printed (has newlines and indentation)
	content := string(data)
	require.Contains(t, content, "\n")
	require.Contains(t, content, "  ") // Indentation
}
