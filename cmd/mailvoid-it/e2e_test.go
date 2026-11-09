package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestIntegrationSuiteE2E builds the real server, launches it on free ports,
// and runs every JTD suite file through the same harness used by the binary.
// It is skipped with -short.
func TestIntegrationSuiteE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping end-to-end integration suite in short mode")
	}
	root := filepath.Clean(filepath.Join("..", ".."))
	binary := filepath.Join(t.TempDir(), "mailvoid")
	build := exec.Command("go", "build", "-o", binary, "./cmd/mailvoid")
	build.Dir = root
	build.Env = append(os.Environ(), "GOTOOLCHAIN=local")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build server: %v\n%s", err, output)
	}

	suite, err := LoadSuite(filepath.Join(root, "testdata", "integration"))
	if err != nil {
		t.Fatal(err)
	}
	opts := Options{ServerPath: binary, AdminUser: "admin", AdminPassword: "mailvoid-it-password", SuiteDir: suite.Dir}
	server, err := StartServer(opts)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = server.Stop()
		server.Cleanup()
	}()

	for _, test := range suite.Tests {
		t.Run(test.Definition.Name, func(t *testing.T) {
			harness, err := NewHarness(server, opts)
			if err != nil {
				t.Fatal(err)
			}
			defer harness.Close()
			result := harness.RunTest(t.Context(), test)
			if !result.Passed {
				t.Fatalf("%s: %v\n%s", result.Name, result.Err, joinLogs(result))
			}
		})
	}
}

func joinLogs(result TestResult) string {
	var out string
	for _, step := range result.Steps {
		out += step.Name + ":\n"
		for _, line := range step.Logs {
			out += "  " + line + "\n"
		}
		if step.Err != nil {
			out += "  ERROR: " + step.Err.Error() + "\n"
		}
	}
	return out
}
