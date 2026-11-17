package cli

import (
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// reorderArgs moves flags before positionals so "show SOURCE --page 2" works.
// valueFlags names the flags that consume the following argument.
func reorderArgs(args []string, valueFlags map[string]bool) []string {
	var flags, positionals []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positionals = append(positionals, args[i+1:]...)
			break
		}
		if len(arg) > 1 && strings.HasPrefix(arg, "-") {
			flags = append(flags, arg)
			name := strings.TrimLeft(arg, "-")
			if strings.Contains(name, "=") {
				continue
			}
			if valueFlags[name] && i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
			continue
		}
		positionals = append(positionals, arg)
	}
	return append(flags, positionals...)
}

func newFlagSet(name string, opt Options, wantJSON bool) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	if wantJSON {
		fs.SetOutput(io.Discard)
	} else {
		fs.SetOutput(opt.Stderr)
	}
	return fs
}

func parseArgs(fs *flag.FlagSet, args []string, opt Options, wantJSON bool, positionals int, usage string) int {
	if err := fs.Parse(args); err != nil {
		if wantJSON {
			return usageError(opt, true, "invalid_flags", err.Error())
		}
		return 2
	}
	if positionals == 0 {
		if fs.NArg() > 0 {
			return usageError(opt, wantJSON, "unexpected_argument", fmt.Sprintf("unexpected argument %q", fs.Arg(0)))
		}
		return 0
	}
	if fs.NArg() != positionals {
		return usageError(opt, wantJSON, "invalid_input", usage)
	}
	return 0
}

func parsePageArg(raw string, opt Options, jsonMode bool) (int, int) {
	page, err := strconv.Atoi(raw)
	if err != nil || page < 1 {
		return 0, usageError(opt, jsonMode, "invalid_input", fmt.Sprintf("invalid page %q", raw))
	}
	return page, 0
}
