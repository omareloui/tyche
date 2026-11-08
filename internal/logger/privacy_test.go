package logger_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// anyKeys are the only keys zap.Any & co may log: validation errors (field
// names + tags), reset options, seeds, the admin's receipt limit, a panic's
// message and an items' owner IDs. Anything else is likely an entity.
var anyKeys = []string{"errors", "options", "seed", "kind", "curr", "limit", "message", "owner"}

// privateKeys never get logged: users' data or what identifies them.
var privateKeys = []string{"name", "email", "merchant", "body", "url", "user", "amount", "description", "note", "timezone", "hash", "csrfId"}

// TestLogsKeepPrivacy fails on a zap field logging an entity or users' data
// (see Logging in CLAUDE.md): only IDs, counts, kinds and errors are logged.
func TestLogsKeepPrivacy(t *testing.T) {
	root := filepath.Join("..", "..")
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "node_modules", "third_party", ".git", "tmp", "data", "logs":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, "_templ.go") {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != "zap" {
				return true
			}
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			key, _ := strconv.Unquote(lit.Value)
			pos := fset.Position(call.Pos())
			switch {
			case slices.Contains([]string{"Any", "Reflect", "Object", "Inline"}, sel.Sel.Name) && !slices.Contains(anyKeys, key):
				t.Errorf("%s: zap.%s(%q, …) logs a whole value: log its ID instead", pos, sel.Sel.Name, key)
			case slices.Contains(privateKeys, key) || strings.HasPrefix(strings.ToLower(key), "oauth"):
				t.Errorf("%s: zap.%s(%q, …) logs users' data", pos, sel.Sel.Name, key)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
