package serdser

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRESTResponseLocalizationContract scans the complete Gin REST adapter and
// enforces the boundary between human-facing prose and canonical API values.
//
// Human-facing response fields must explicitly opt into localization via the
// i18n:"localized" struct tag. Machine-readable fields such as status, result,
// outcome, and reason_codes must never opt into localization.
func TestRESTResponseLocalizationContract(t *testing.T) {
	t.Parallel()

	root := ginRESTRoot(t)
	fset := token.NewFileSet()
	var violations []string

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
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
			typeSpec, ok := node.(*ast.TypeSpec)
			if !ok || !isRESTResponseStructName(typeSpec.Name.Name) {
				return true
			}

			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				return true
			}

			for _, field := range structType.Fields.List {
				if field.Tag == nil {
					continue
				}

				tagValue := strings.Trim(field.Tag.Value, "`")
				tags := reflect.StructTag(tagValue)
				jsonName := strings.Split(tags.Get("json"), ",")[0]
				if jsonName == "" || jsonName == "-" {
					continue
				}

				i18nMode := tags.Get("i18n")
				position := fset.Position(field.Pos())

				if isHumanFacingProseField(jsonName) && i18nMode != "localized" {
					violations = append(violations,
						position.String()+": human-facing response field "+jsonName+" must declare i18n:\"localized\"",
					)
				}

				if isCanonicalResponseField(jsonName) && i18nMode == "localized" {
					violations = append(violations,
						position.String()+": canonical response field "+jsonName+" must not be localized",
					)
				}
			}

			return true
		})

		return nil
	})
	require.NoError(t, err)
	assert.Empty(t, violations, strings.Join(violations, "\n"))
}

// TestRESTDirectJSONWritesStayBehindResponseBoundaries prevents new handlers
// from bypassing the serializer/localization boundary. Health probes are the
// deliberate exception because their UP response is a canonical machine
// contract rather than user-facing prose.
func TestRESTDirectJSONWritesStayBehindResponseBoundaries(t *testing.T) {
	t.Parallel()

	root := ginRESTRoot(t)
	fset := token.NewFileSet()
	var violations []string

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}

		if strings.HasPrefix(rel, "serdser"+string(filepath.Separator)) ||
			strings.HasPrefix(rel, "healthrs"+string(filepath.Separator)) {
			return nil
		}

		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return parseErr
		}

		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}

			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "JSON" {
				return true
			}

			violations = append(violations,
				fset.Position(call.Pos()).String()+": direct JSON response bypasses serdser localization boundary",
			)
			return true
		})

		return nil
	})
	require.NoError(t, err)
	assert.Empty(t, violations, strings.Join(violations, "\n"))
}

func ginRESTRoot(t *testing.T) string {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	require.True(t, ok)

	return filepath.Dir(filepath.Dir(currentFile))
}

func isRESTResponseStructName(name string) bool {
	lower := strings.ToLower(name)
	if strings.Contains(lower, "response") || strings.Contains(lower, "schema") {
		return true
	}

	switch name {
	case "SuccessMeta", "ErrorDetails", "ErrorViolation":
		return true

	default:
		return false
	}
}

func isHumanFacingProseField(name string) bool {
	for _, candidate := range []string{
		"message",
		"title",
		"description",
		"label",
		"hint",
		"text",
		"explanation",
		"notice",
	} {
		if name == candidate || strings.HasSuffix(name, "_"+candidate) {
			return true
		}
	}

	return false
}

func isCanonicalResponseField(name string) bool {
	switch name {
	case "status",
		"state",
		"code",
		"status_code",
		"result",
		"outcome",
		"reason_codes",
		"type",
		"currency",
		"currencies",
		"channel",
		"purpose",
		"funding_models",
		"settlement_models",
		"jurisdiction":
		return true

	default:
		return false
	}
}

// TestPublicMessageLiteralsAreSemanticKeys prevents raw user-facing prose from
// being introduced where the localization pipeline expects catalog keys.
func TestPublicMessageLiteralsAreSemanticKeys(t *testing.T) {
	t.Parallel()

	root := repositoryRootFromGin(t)
	fset := token.NewFileSet()
	var violations []string

	err := filepath.WalkDir(filepath.Join(root, "pkg"), func(path string, entry os.DirEntry, walkErr error) error {
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
			switch typed := node.(type) {
			case *ast.CallExpr:
				selector, ok := typed.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}

				switch selector.Sel.Name {
				case "NewPublic", "NewPublicWithParams":
					if len(typed.Args) > 1 {
						checkSemanticKeyLiteral(fset, typed.Args[1], []string{"error.", "validation."}, &violations)
					}

				case "SerValidationErr":
					if len(typed.Args) > 2 {
						checkSemanticKeyLiteral(fset, typed.Args[2], []string{"validation."}, &violations)
					}
				}

			case *ast.KeyValueExpr:
				ident, ok := typed.Key.(*ast.Ident)
				if !ok || ident.Name != "MessageKey" {
					return true
				}

				checkSemanticKeyLiteral(fset, typed.Value, []string{"error.", "validation."}, &violations)
			}

			return true
		})

		return nil
	})
	require.NoError(t, err)
	assert.Empty(t, violations, strings.Join(violations, "\n"))
}

func checkSemanticKeyLiteral(
	fset *token.FileSet,
	expr ast.Expr,
	allowedPrefixes []string,
	violations *[]string,
) {
	literal, ok := expr.(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return
	}

	value, err := strconv.Unquote(literal.Value)
	if err != nil {
		return
	}

	for _, prefix := range allowedPrefixes {
		if strings.HasPrefix(value, prefix) {
			return
		}
	}

	*violations = append(*violations,
		fset.Position(literal.Pos()).String()+": public message literal must be a semantic i18n key, got "+value,
	)
}

func repositoryRootFromGin(t *testing.T) string {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	require.True(t, ok)

	dir := filepath.Dir(currentFile)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		require.NotEqual(t, dir, parent, "could not locate repository root from %s", currentFile)
		dir = parent
	}
}
