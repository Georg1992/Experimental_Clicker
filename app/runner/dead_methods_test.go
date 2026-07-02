// dead_methods_test.go is a guard against accidentally pruning exported
// methods that still have external callers. For each method in
// `methodsToCheck`, it walks the module (skipping the runner package and
// this test file) and verifies at least one call site of `.Method(`
// exists anywhere outside the runner package.
//
// The test catches "listed method has become dead" — it does NOT catch
// "new method added but not listed". Use code review to add new methods
// to the list when they appear on the runner types.
//
// Limitation: the check is name-based, not type-based, so a method with
// a common name could in principle pass via an unrelated call site. The
// known collisions in this codebase are `cmd.Start` and `cmd.Process.Wait`
// (`*exec.Cmd`, in gui/server.go) — they would falsely count as callers
// of `Runner.Start` / `Runner.Wait`. The unique-name methods
// (`UpdateSettings`, `SetOnPauseChanged`, `StartPauseWatcher`) are
// inherently safe — no other type defines them.
//
// Interface-implementation methods (e.g. ViiperSession's InputSession
// methods) are intentionally excluded — they're called through the
// interface from within the runner package, so they wouldn't have
// external callers.
package runner_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var methodsToCheck = []struct{ Type, Method string }{
	// Runner (clicker)
	{"Runner", "Running"},
	{"Runner", "UpdateSettings"},
	{"Runner", "Start"},
	{"Runner", "Stop"},
	{"Runner", "Wait"},
	// KeyChainRunner
	{"KeyChainRunner", "Running"},
	{"KeyChainRunner", "UpdateSettings"},
	{"KeyChainRunner", "Start"},
	{"KeyChainRunner", "Stop"},
	{"KeyChainRunner", "Wait"},
	// TimerKeyRunner
	{"TimerKeyRunner", "Running"},
	{"TimerKeyRunner", "UpdateSettings"},
	{"TimerKeyRunner", "Start"},
	{"TimerKeyRunner", "Stop"},
	{"TimerKeyRunner", "Wait"},
	// ViiperSession (excluding InputSession interface impls, which are
	// called through the interface from inside the runner package)
	{"ViiperSession", "Close"},
}

func TestNoDeadMethods(t *testing.T) {
	moduleRoot := findModuleRoot(t)
	for _, m := range methodsToCheck {
		if !hasExternalCaller(t, moduleRoot, m.Method) {
			t.Errorf("dead method: %s.%s has no external callers — prune it from methodsToCheck or add a test that exercises it", m.Type, m.Method)
		}
	}
}

// findModuleRoot walks up from this test file's directory until it finds
// go.mod, returning the directory containing it. Anchored on the test
// file (not the working directory) so it works under any test runner.
func findModuleRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(thisFile)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

// hasExternalCaller walks the module looking for any `.methodName(`
// SelectorExpr. Skips the runner package directory (where the methods
// are defined) and this test file. Name-based (not type-based), so see
// the file-level comment for the false-positive caveat.
func hasExternalCaller(t *testing.T, moduleRoot, methodName string) bool {
	found := false
	runnerDir := filepath.Join(moduleRoot, "runner")
	filepath.Walk(moduleRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || found {
			return nil
		}
		if info.IsDir() {
			if path == runnerDir {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "dead_methods_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Logf("parse error in %s: %v", path, err)
			return nil
		}
		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if sel.Sel.Name != methodName {
				return true
			}
			found = true
			return false
		})
		return nil
	})
	return found
}
