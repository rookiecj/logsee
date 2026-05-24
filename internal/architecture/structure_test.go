package architecture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRequiredProjectStructureExists(t *testing.T) {
	root := repoRoot(t)

	requiredDirs := []string{
		"cmd/logsee",
		"internal/domain",
		"internal/usecase",
		"internal/port",
		"internal/adapter",
		"internal/adapter/cli",
		"internal/adapter/config",
		"internal/adapter/filesystem",
		"internal/adapter/tui",
		"internal/adapter/clipboard",
		"configs",
		"testdata",
	}

	for _, dir := range requiredDirs {
		info, err := os.Stat(filepath.Join(root, dir))
		if err != nil {
			t.Errorf("required directory %q is missing: %v", dir, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("required path %q must be a directory", dir)
		}
	}
}

func TestRequiredGoPackagesExist(t *testing.T) {
	root := repoRoot(t)

	requiredPackages := []string{
		"cmd/logsee",
		"internal/domain",
		"internal/usecase",
		"internal/port",
		"internal/adapter",
		"internal/adapter/cli",
		"internal/adapter/config",
		"internal/adapter/filesystem",
		"internal/adapter/tui",
		"internal/adapter/clipboard",
	}

	for _, dir := range requiredPackages {
		if !hasProductionGoFile(t, filepath.Join(root, dir)) {
			t.Errorf("required Go package %q has no production .go file", dir)
		}
	}
}

func TestEntrypointStaysInBootstrapLayer(t *testing.T) {
	root := repoRoot(t)
	mainPath := filepath.Join(root, "cmd/logsee/main.go")
	file := parseGoFile(t, mainPath)

	if file.Name.Name != "main" {
		t.Fatalf("cmd/logsee/main.go package = %q, want main", file.Name.Name)
	}

	for _, spec := range file.Imports {
		importPath := unquoteImportPath(spec.Path.Value)
		if strings.Contains(importPath, "internal/domain") || strings.Contains(importPath, "internal/usecase") {
			t.Errorf("entrypoint must stay in bootstrap/wiring layer; unexpected import %q", importPath)
		}
	}
}

func TestCleanArchitectureImportBoundaries(t *testing.T) {
	root := repoRoot(t)

	assertNoForbiddenImports(t, filepath.Join(root, "internal/domain"), []string{
		"internal/adapter",
		"internal/adapter/cli",
		"internal/adapter/config",
		"internal/adapter/filesystem",
		"internal/adapter/tui",
		"internal/adapter/clipboard",
		"github.com/charmbracelet",
		"github.com/rivo/tview",
		"github.com/atotto/clipboard",
		"golang.org/x/term",
		"os",
		"io/fs",
		"path/filepath",
	})

	assertNoForbiddenImports(t, filepath.Join(root, "internal/usecase"), []string{
		"internal/adapter",
	})
}

func repoRoot(t *testing.T) string {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}

	for {
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			return wd
		}

		parent := filepath.Dir(wd)
		if parent == wd {
			t.Fatal("could not find repository root containing go.mod")
		}
		wd = parent
	}
}

func hasProductionGoFile(t *testing.T, dir string) bool {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") {
			return true
		}
	}
	return false
}

func assertNoForbiddenImports(t *testing.T, dir string, forbidden []string) {
	t.Helper()

	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		file := parseGoFile(t, path)
		for _, spec := range file.Imports {
			importPath := unquoteImportPath(spec.Path.Value)
			for _, blocked := range forbidden {
				if importPath == blocked || strings.Contains(importPath, blocked) {
					t.Errorf("%s imports forbidden dependency %q", filepath.ToSlash(path), importPath)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
}

func parseGoFile(t *testing.T, path string) *ast.File {
	t.Helper()

	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return file
}

func unquoteImportPath(value string) string {
	return strings.Trim(value, `"`)
}
