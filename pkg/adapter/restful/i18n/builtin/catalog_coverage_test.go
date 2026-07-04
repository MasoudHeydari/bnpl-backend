package builtin

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	resti18n "github.com/parstis/bnpl-backend/pkg/adapter/restful/i18n"
)

// TestCatalogsCoverAllProductionMessageKeys scans production Go source for
// semantic REST message keys and verifies that every key exists in both
// supported catalogs. This catches a new public message that was wired into an
// endpoint but not translated into en-US or fa-IR.
func TestCatalogsCoverAllProductionMessageKeys(t *testing.T) {
	t.Parallel()

	enUS, err := loadCatalog(resti18n.LocaleENUS)
	require.NoError(t, err)
	faIR, err := loadCatalog(resti18n.LocaleFAIR)
	require.NoError(t, err)

	root := repositoryRoot(t)
	keys := collectProductionMessageKeys(t, root)
	require.NotEmpty(t, keys)

	for key := range keys {
		_, ok := enUS[key]
		require.True(t, ok, "production message key %q is missing from en-US catalog", key)
		_, ok = faIR[key]
		require.True(t, ok, "production message key %q is missing from fa-IR catalog", key)
	}
}

func collectProductionMessageKeys(t *testing.T, root string) map[string]struct{} {
	t.Helper()

	keys := make(map[string]struct{})
	fset := token.NewFileSet()

	for _, subtree := range []string{"pkg", "cmd"} {
		base := filepath.Join(root, subtree)
		if _, err := os.Stat(base); os.IsNotExist(err) {
			continue
		}

		err := filepath.WalkDir(base, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}

			if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}

			file, parseErr := parser.ParseFile(fset, path, nil, 0)
			if parseErr != nil {
				return parseErr
			}

			ast.Inspect(file, func(node ast.Node) bool {
				literal, ok := node.(*ast.BasicLit)
				if !ok || literal.Kind != token.STRING {
					return true
				}

				value, unquoteErr := strconv.Unquote(literal.Value)
				if unquoteErr != nil {
					return true
				}

				if isMessageKey(value) {
					keys[value] = struct{}{}
				}

				return true
			})

			return nil
		})
		require.NoError(t, err)
	}

	return keys
}

func isMessageKey(value string) bool {
	return strings.HasPrefix(value, "error.") ||
		strings.HasPrefix(value, "validation.") ||
		strings.HasPrefix(value, "success.")
}

func repositoryRoot(t *testing.T) string {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	require.True(t, ok)

	dir := filepath.Dir(currentFile)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		require.NotEqual(
			t,
			dir,
			parent,
			"could not locate repository root from %s",
			currentFile,
		)
		dir = parent
	}
}
