package viz

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// GoldenFileManager manages golden file snapshots for test cases.
type GoldenFileManager struct {
	baseDir string // Base directory for golden files
}

// NewGoldenFileManager creates a new golden file manager.
func NewGoldenFileManager(baseDir string) *GoldenFileManager {
	return &GoldenFileManager{
		baseDir: baseDir,
	}
}

// Save saves test output as a golden file.
func (gfm *GoldenFileManager) Save(suiteID, testName string, output map[string]any) error {
	path := gfm.getPath(suiteID, testName)

	// Create directory if it doesn't exist
	dir := filepath.Dir(path)
	//nolint:gosec // G301: Test golden files need standard permissions for version control
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Marshal output to pretty JSON
	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal output: %w", err)
	}

	// Write to file
	//nolint:gosec // G306: Test golden files need standard permissions for version control
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write golden file: %w", err)
	}

	return nil
}

// Load loads expected output from a golden file.
func (gfm *GoldenFileManager) Load(suiteID, testName string) (map[string]any, error) {
	path := gfm.getPath(suiteID, testName)

	// Read file
	//nolint:gosec // G304: Path is constructed from controlled test inputs using filepath functions
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("golden file does not exist: %s", path)
		}
		return nil, fmt.Errorf("failed to read golden file: %w", err)
	}

	// Unmarshal JSON
	var output map[string]any
	if err := json.Unmarshal(data, &output); err != nil {
		return nil, fmt.Errorf("failed to unmarshal golden file: %w", err)
	}

	return output, nil
}

// Exists checks if a golden file exists for the test.
func (gfm *GoldenFileManager) Exists(suiteID, testName string) bool {
	path := gfm.getPath(suiteID, testName)
	_, err := os.Stat(path)
	return err == nil
}

// Delete removes a golden file.
func (gfm *GoldenFileManager) Delete(suiteID, testName string) error {
	path := gfm.getPath(suiteID, testName)

	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return nil // Already deleted
		}
		return fmt.Errorf("failed to delete golden file: %w", err)
	}

	return nil
}

// List lists all golden files for a suite.
func (gfm *GoldenFileManager) List(suiteID string) ([]string, error) {
	suiteDir := filepath.Join(gfm.baseDir, sanitizeName(suiteID))

	// Check if directory exists
	if _, err := os.Stat(suiteDir); os.IsNotExist(err) {
		return []string{}, nil
	}

	// Read directory
	entries, err := os.ReadDir(suiteDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read suite directory: %w", err)
	}

	// Collect test names
	testNames := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".golden.json") {
			continue
		}

		// Extract test name from filename
		name := strings.TrimSuffix(entry.Name(), ".golden.json")
		testNames = append(testNames, name)
	}

	return testNames, nil
}

// getPath generates the file path for a golden file.
func (gfm *GoldenFileManager) getPath(suiteID, testName string) string {
	sanitizedSuite := sanitizeName(suiteID)
	sanitizedTest := sanitizeName(testName)
	filename := fmt.Sprintf("%s.golden.json", sanitizedTest)
	return filepath.Join(gfm.baseDir, sanitizedSuite, filename)
}

// sanitizeName sanitizes a name for use in a file path.
func sanitizeName(name string) string {
	// Replace invalid characters with underscores
	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
		" ", "_",
	)
	return replacer.Replace(name)
}

// GetPath returns the file system path for a golden file.
func (gfm *GoldenFileManager) GetPath(suiteID, testName string) string {
	return gfm.getPath(suiteID, testName)
}

// LoadOrCreate loads a golden file if it exists, or creates it if it doesn't.
// Returns the loaded/created output, whether it was newly created, and any error.
func (gfm *GoldenFileManager) LoadOrCreate(suiteID, testName string, output map[string]any) (map[string]any, bool, error) {
	// Check if file exists first
	if gfm.Exists(suiteID, testName) {
		// Load existing golden file
		expected, err := gfm.Load(suiteID, testName)
		if err != nil {
			return nil, false, err
		}
		return expected, false, nil // Loaded existing
	}

	// Save as new golden file
	if err := gfm.Save(suiteID, testName, output); err != nil {
		return nil, false, err
	}

	return output, true, nil // Created new
}

// Compare compares actual output with golden file and returns differences.
func (gfm *GoldenFileManager) Compare(suiteID, testName string, actual map[string]any) ([]Diff, error) {
	expected, err := gfm.Load(suiteID, testName)
	if err != nil {
		return nil, err
	}

	// Use TestRunner's comparison logic
	runner := &TestRunner{}
	diffs := runner.compareOutputs(expected, actual)

	return diffs, nil
}

// Update updates a golden file with new output.
// This is the same as Save but more explicit for update operations.
func (gfm *GoldenFileManager) Update(suiteID, testName string, output map[string]any) error {
	return gfm.Save(suiteID, testName, output)
}

// UpdateAll updates all golden files for a suite based on test results.
func (gfm *GoldenFileManager) UpdateAll(results []TestRun) error {
	for i := range results {
		if err := gfm.Save(results[i].SuiteID, results[i].TestName, results[i].Actual); err != nil {
			return fmt.Errorf("failed to update golden file for %s: %w", results[i].TestName, err)
		}
	}
	return nil
}
