// Package architecture guards the component conventions that the inspector
// cannot enforce on packages it does not recognize as components.
package architecture

import (
	"go/ast"
	"go/types"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarchlab/akita/v5/inspect"
	"golang.org/x/tools/go/packages"
)

const modelingPath = "github.com/sarchlab/akita/v5/sim/modeling/"

func TestComponentArchitecture(t *testing.T) {
	root := filepath.Join("..", "..")
	defs, diagnostics := inspect.Inspect(inspect.Options{Dir: root}, "./amd/...", "./nvidia/...")
	for _, diagnostic := range diagnostics {
		t.Error(diagnostic)
	}
	if len(defs) == 0 {
		t.Fatal("inspector found no components")
	}
	pkgs := load(t, root, "./amd/...", "./nvidia/...")
	for _, diagnostic := range checkPackages(pkgs) {
		t.Error(diagnostic)
	}
	t.Logf("inspected %d component packages without architecture diagnostics", len(defs))
}

func load(t *testing.T, dir string, patterns ...string) []*packages.Package {
	t.Helper()
	pkgs, err := packages.Load(&packages.Config{
		Dir: dir, Tests: true,
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo | packages.NeedImports | packages.NeedDeps,
		Env: append(os.Environ(), "GOTOOLCHAIN=local", "CGO_ENABLED=0", "GOPACKAGESDRIVER=off"),
	}, patterns...)
	if err != nil {
		t.Fatal(err)
	}
	for _, pkg := range pkgs {
		for _, err := range pkg.Errors {
			t.Error(err)
		}
	}
	if t.Failed() {
		t.FailNow()
	}
	return pkgs
}

func checkPackages(pkgs []*packages.Package) []string {
	components := make(map[string]map[string]bool)
	covered := make(map[string]bool)
	for _, pkg := range pkgs {
		for _, file := range pkg.Syntax {
			isTest := strings.HasSuffix(pkg.Fset.Position(file.Pos()).Filename, "_test.go")
			ast.Inspect(file, func(node ast.Node) bool {
				if declaration, ok := node.(*ast.TypeSpec); ok && !isTest && isComponent(pkg.TypesInfo.TypeOf(declaration.Type)) {
					if components[pkg.PkgPath] == nil {
						components[pkg.PkgPath] = make(map[string]bool)
					}
					components[pkg.PkgPath][declaration.Name.Name] = declaration.Assign.IsValid()
				}
				if call, ok := node.(*ast.CallExpr); ok && isTest && isModelCheck(pkg.TypesInfo, call.Fun) {
					// Both same-package and external-package tests count.
					covered[strings.TrimSuffix(pkg.PkgPath, "_test")] = true
				}
				return true
			})
		}
	}
	var diagnostics []string
	for path, declarations := range components {
		if len(declarations) != 1 || !declarations["Comp"] {
			diagnostics = append(diagnostics, path+": declare exactly one component, as type Comp = model.Component[...]")
		}
		if !covered[path] {
			diagnostics = append(diagnostics, path+": missing modelingtest.Check* call in tests")
		}
	}
	return diagnostics
}

func isComponent(typ types.Type) bool {
	if typ == nil {
		return false
	}
	named, ok := types.Unalias(typ).(*types.Named)
	if !ok || named.Obj().Pkg() == nil || named.Obj().Name() != "Component" {
		return false
	}
	switch named.Obj().Pkg().Path() {
	case modelingPath + "ticking", modelingPath + "wakeup", modelingPath + "event":
		return true
	}
	return false
}

func isModelCheck(info *types.Info, expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.IndexExpr:
		return isModelCheck(info, e.X)
	case *ast.IndexListExpr:
		return isModelCheck(info, e.X)
	case *ast.SelectorExpr:
		return isModelCheck(info, e.Sel)
	case *ast.Ident:
		fn, ok := info.Uses[e].(*types.Func)
		return ok && fn.Pkg() != nil && fn.Pkg().Path() == modelingPath+"modelingtest" &&
			strings.HasPrefix(fn.Name(), "Check")
	}
	return false
}

func TestGuardRejectsMissedComponents(t *testing.T) {
	for _, tc := range []struct{ name, want string }{
		{"valid", ""},
		{"hidden", "declare exactly one component"},
		{"multiple", "declare exactly one component"},
		{"defined", "declare exactly one component"},
		{"uncovered", "missing modelingtest.Check"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := strings.Join(checkPackages(load(t, ".", "./testdata/"+tc.name)), "\n")
			if tc.want == "" && got != "" || tc.want != "" && !strings.Contains(got, tc.want) {
				t.Fatalf("diagnostics = %q, want %q", got, tc.want)
			}
		})
	}
}
