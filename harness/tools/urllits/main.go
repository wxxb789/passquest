// Command urllits reports http(s) URL literals in Go source.
//
// The local-only gate (goal 0015) pins the network exception's endpoint with
// it: the update package's non-test Go files may carry the constant release
// URL and the release-page prefix as literals and nothing else, so the check
// has to read real string literals, not text - a comment quoting a URL, a
// rune holding one or a doc example is not a URL the program can request. Go already
// ships the answer: go/parser gives the positions of real string literals,
// the same reason harness/tools/pathtypo exists.
//
// Output, one line per hit: <file>:<line>: <quoted literal>. With -files it
// reads the paths from stdin, one per line: a .go path after the tool file
// would be compiled by go run as a second source file.
package main

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
)

func main() {
	paths := os.Args[1:]
	if len(paths) > 0 && paths[0] == "-files" {
		paths = nil
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			if file := strings.TrimSpace(scanner.Text()); file != "" {
				paths = append(paths, file)
			}
		}
		if err := scanner.Err(); err != nil {
			fmt.Fprintf(os.Stderr, "stdin: %v\n", err)
			os.Exit(2)
		}
	}

	fset := token.NewFileSet()
	for _, path := range paths {
		src, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
			os.Exit(2)
		}
		file, err := parser.ParseFile(fset, path, src, parser.SkipObjectResolution)
		if err != nil {
			// A file that does not parse is go-build's finding, not this gate's.
			continue
		}
		ast.Inspect(file, func(node ast.Node) bool {
			lit, ok := node.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			value, err := strconv.Unquote(lit.Value)
			if err != nil || !httpURL(value) {
				return true
			}
			position := fset.Position(lit.Pos())
			fmt.Printf("%s:%d: %s\n", position.Filename, position.Line, strconv.Quote(value))
			return true
		})
	}
}

// httpURL reports whether value starts with the http or https scheme, in any
// spelling: net/url lower-cases the scheme, so an upper-case one still
// requests.
func httpURL(value string) bool {
	scheme := strings.ToLower(value)
	return strings.HasPrefix(scheme, "http://") || strings.HasPrefix(scheme, "https://")
}
