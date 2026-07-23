// Command strip_comments blanks out every comment (line and block) in one
// or more .go files, using go/scanner so string/rune literals containing
// "//" or "/*" are never mistaken for comments. Non-comment bytes,
// including newlines inside multi-line block comments, are left
// untouched, so line numbers and non-comment line content are unchanged
// -- a line that was pure comment becomes blank (and is then excluded by
// the caller's normal blank-line filtering); a line with trailing code
// plus a comment keeps its code.
//
// Usage:
//
//	go run strip_comments.go <file1.go> [<file2.go> ...]
//
// Emits a JSON array of {"path": ..., "source": ...} in input order.
package main

import (
	"encoding/json"
	"fmt"
	"go/scanner"
	"go/token"
	"os"
)

type Result struct {
	Path   string `json:"path"`
	Source string `json:"source"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: strip_comments <file1.go> [<file2.go> ...]")
		os.Exit(1)
	}

	var results []Result
	for _, path := range os.Args[1:] {
		src, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not read %s: %v\n", path, err)
			continue
		}
		results = append(results, Result{Path: path, Source: string(stripComments(src))})
	}

	enc := json.NewEncoder(os.Stdout)
	if err := enc.Encode(results); err != nil {
		fmt.Fprintf(os.Stderr, "error encoding output: %v\n", err)
		os.Exit(1)
	}
}

func stripComments(src []byte) []byte {
	fset := token.NewFileSet()
	file := fset.AddFile("", fset.Base(), len(src))

	out := make([]byte, len(src))
	copy(out, src)

	var s scanner.Scanner
	// scanner.Scanner calls the error handler and keeps going for most
	// lexical errors (e.g. an unterminated literal); we don't need to
	// abort the whole file over that, just skip reporting.
	s.Init(file, src, func(pos token.Position, msg string) {}, scanner.ScanComments)

	for {
		pos, tok, lit := s.Scan()
		if tok == token.EOF {
			break
		}
		if tok != token.COMMENT {
			continue
		}
		start := file.Offset(pos)
		end := start + len(lit)
		if end > len(out) {
			end = len(out)
		}
		for i := start; i < end; i++ {
			if out[i] != '\n' {
				out[i] = ' '
			}
		}
	}
	return out
}
