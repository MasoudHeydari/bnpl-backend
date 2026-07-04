package paymentapplication

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPackageDoesNotImportRepo(t *testing.T) {
	t.Parallel()

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller failed")
	}

	packageDir := filepath.Dir(filename)
	entries, err := os.ReadDir(packageDir)
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}

		content, readErr := os.ReadFile(filepath.Join(packageDir, entry.Name()))
		if readErr != nil {
			t.Fatalf("read %s: %v", entry.Name(), readErr)
		}

		for line := range strings.SplitSeq(string(content), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "import ") && strings.Contains(trimmed, "pkg/core/repo") {
				t.Fatalf("%s imports repo", entry.Name())
			}
		}
	}
}

func TestApplicationResult_IsValid(t *testing.T) {
	t.Parallel()

	if !AppliedApplicationResult.IsValid() {
		t.Fatal("expected applied to be valid")
	}

	if ApplicationResult("unknown").IsValid() {
		t.Fatal("expected unknown to be invalid")
	}
}
