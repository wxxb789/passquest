// Command pathtypo reports platform-bound path literals in Go source.
//
// The gate used to scan text and grew a hand-written lexer for raw strings, runes
// and comments; every review round found another escape. Go already ships the
// answer: go/parser gives the positions of real string literals, so a rune holding
// a backtick, a comment quoting a path, or SQL inside a raw string cannot be
// mistaken for a path.
//
// Output, one line per hit: <file>:<line>: <literal>
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

var posixRoots = []string{
	"/tmp/",     // portable-paths: allow the rule table this scan is built from
	"/home/",    // portable-paths: allow the rule table this scan is built from
	"/Users/",   // portable-paths: allow the rule table this scan is built from
	"/usr/",     // portable-paths: allow the rule table this scan is built from
	"/var/",     // portable-paths: allow the rule table this scan is built from
	"/etc/",     // portable-paths: allow the rule table this scan is built from
	"/opt/",     // portable-paths: allow the rule table this scan is built from
	"/private/", // portable-paths: allow the rule table this scan is built from
	"/root/",    // portable-paths: allow the rule table this scan is built from
	"/mnt/",     // portable-paths: allow the rule table this scan is built from
}

const allowMarker = "portable-paths: allow"

// platformBound reports whether a literal value only resolves on POSIX.
func platformBound(value string) bool {
	if len(value) > 2 && value[1] == ':' && (value[2] == '\\' || value[2] == '/') {
		if (value[0] >= 'A' && value[0] <= 'Z') || (value[0] >= 'a' && value[0] <= 'z') {
			return true
		}
	}
	if strings.HasPrefix(value, `\\`) && len(value) > 2 && value[2] != '\\' { // portable-paths: allow the UNC rule, not a path
		return true
	}
	for _, root := range posixRoots {
		if strings.HasPrefix(value, root) {
			return true
		}
	}
	return false
}

// allowed reports whether the literal's own line carries the marker.
func allowed(line string) bool {
	return strings.Contains(line, allowMarker)
}

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
	hits := 0
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
		lines := strings.Split(string(src), "\n")
		ast.Inspect(file, func(node ast.Node) bool {
			lit, ok := node.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			value, err := strconv.Unquote(lit.Value)
			if err != nil || !platformBound(value) {
				return true
			}
			position := fset.Position(lit.Pos())
			if position.Line <= len(lines) && allowed(lines[position.Line-1]) {
				return true
			}
			fmt.Printf("%s:%d: %s\n", position.Filename, position.Line, strconv.Quote(value))
			hits++
			return true
		})
	}
	if hits > 0 {
		os.Exit(1)
	}
}
