// Command extract_go_funcs walks one or more directories and emits a JSON
// array of every top-level function and method declaration found in the
// .go files under them, for use by code_similarity.py.
//
// Usage:
//
//	go run extract_go_funcs.go [--include-tests] <root> [<root2> ...]
package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// FuncRecord is one extracted function/method.
type FuncRecord struct {
	Path      string `json:"path"`
	Name      string `json:"name"`
	Receiver  string `json:"receiver,omitempty"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Source    string `json:"source"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: extract_go_funcs [--include-tests] <root> [<root2> ...]")
		os.Exit(1)
	}

	includeTests := false
	var roots []string
	for _, a := range os.Args[1:] {
		if a == "--include-tests" {
			includeTests = true
			continue
		}
		roots = append(roots, a)
	}

	var records []FuncRecord
	for _, root := range roots {
		walkErr := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: %v\n", err)
				return nil
			}
			if info.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".go") {
				return nil
			}
			if !includeTests && strings.HasSuffix(path, "_test.go") {
				return nil
			}
			records = append(records, extractFile(path)...)
			return nil
		})
		if walkErr != nil {
			fmt.Fprintf(os.Stderr, "warning: walking %s: %v\n", root, walkErr)
		}
	}

	if records == nil {
		records = []FuncRecord{}
	}
	enc := json.NewEncoder(os.Stdout)
	if err := enc.Encode(records); err != nil {
		fmt.Fprintf(os.Stderr, "error encoding output: %v\n", err)
		os.Exit(1)
	}
}

func extractFile(path string) []FuncRecord {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: parse error in %s: %v\n", path, err)
		return nil
	}
	src, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not read %s: %v\n", path, err)
		return nil
	}

	var out []FuncRecord
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		start := fset.Position(fn.Pos())
		end := fset.Position(fn.End())
		out = append(out, FuncRecord{
			Path:      path,
			Name:      fn.Name.Name,
			Receiver:  receiverType(fn),
			StartLine: start.Line,
			EndLine:   end.Line,
			Source:    string(src[start.Offset:end.Offset]),
		})
	}
	return out
}

func receiverType(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return ""
	}
	return exprString(fn.Recv.List[0].Type)
}

func exprString(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + exprString(t.X)
	default:
		return ""
	}
}
