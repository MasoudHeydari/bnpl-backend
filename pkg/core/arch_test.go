package core_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCorePackagesDoNotImportAdapters(t *testing.T) {
	t.Parallel()

	root := "."
	forbidden := []string{
		"github.com/parstis/bnpl-backend/pkg/adapter/",
		"github.com/gin-gonic/gin",
		"github.com/jackc/pgx",
		"github.com/IBM/sarama",
		"github.com/redis/go-redis",
		"github.com/sony/gobreaker",
	}

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if entry.IsDir() {
			return nil
		}

		if strings.HasPrefix(filepath.ToSlash(path), "mock/") {
			return nil
		}

		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		fset := token.NewFileSet()
		file, parseErr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if parseErr != nil {
			return parseErr
		}

		for _, imp := range file.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			for _, prefix := range forbidden {
				if strings.HasPrefix(importPath, prefix) {
					return &forbiddenImportError{file: path, importPath: importPath}
				}
			}
		}

		return nil
	})
	require.NoError(t, err)
}

type forbiddenImportError struct {
	file       string
	importPath string
}

func (e *forbiddenImportError) Error() string {
	return "forbidden import " + e.importPath + " in " + e.file
}
