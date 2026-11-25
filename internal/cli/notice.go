package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/wxxb789/passquest/internal/cmdline"
	"github.com/wxxb789/passquest/internal/userdata"
)

// noticeState is the old-default notice's once-per-run state.
type noticeState struct{ shown bool }

// noticeOldDefault tells the learner, once per run, that a database from the
// old working-directory default still exists and is not moved: the command line
// reads the per-user database now, and moving or merging the old file is the
// learner's own action. It reads the filesystem and writes one stderr line
// before the command's own work; nothing is copied, moved or deleted, so --json
// stdout is untouched. The move command is printed only when the per-user
// database does not exist yet — with both files a plain mv would replace the
// database the window is using — and a shell whose family cannot be detected
// loses the move command, not the notice.
func (o Options) noticeOldDefault(newPath string) {
	if o.notice == nil || o.notice.shown {
		return
	}
	o.notice.shown = true
	workdir, err := os.Getwd()
	if err != nil {
		return
	}
	oldPath, err := filepath.Abs(filepath.Join(workdir, userdata.LegacyDatabasePath))
	if err != nil {
		return
	}
	info, err := os.Stat(oldPath)
	if err != nil || info.IsDir() {
		return
	}
	resolved, err := filepath.Abs(newPath)
	if err != nil {
		return
	}
	if samePath(oldPath, resolved) {
		return
	}
	_, statErr := os.Stat(resolved)
	// Any stat result but a proven absence counts as existing: a move command
	// must not be suggested for a target the notice cannot show is absent.
	newExists := !errors.Is(statErr, fs.ErrNotExist)
	fmt.Fprintln(o.Stderr, oldDefaultNoticeLine(oldPath, resolved, newExists, o.Getenv))
}

// oldDefaultNoticeLine renders the notice: both absolute paths and the promise,
// plus — when the per-user database does not exist yet and the shell family can
// be detected — the move command quoted for it with the family, its source and
// the override hint beside it, exactly as next prints them. The verb follows
// the family, so the pasted command exists in the shell the arguments were
// quoted for.
func oldDefaultNoticeLine(oldPath, newPath string, newExists bool, getenv func(string) string) string {
	line := fmt.Sprintf("passquest: a database from the old default location exists at %s; the command line now uses %s. Pass Quest will not move it; move or merge it yourself",
		oldPath, newPath)
	if newExists {
		return line + " (the per-user database already exists, so no move command is printed; merge instead of overwriting it)."
	}
	choice, err := cmdline.DetectShell(getenv, runtime.GOOS)
	if err != nil {
		return line + "."
	}
	if choice.Family == cmdline.ShellCmd && strings.Contains(oldPath+newPath, "%") {
		// cmd.exe expands %NAME% even inside double quotes (cf. next's
		// warning), so this command would not be copy-safe.
		return line + "."
	}
	verb := "mv"
	switch choice.Family {
	case cmdline.ShellCmd:
		verb = "move"
	case cmdline.ShellPowerShell:
		verb = "Move-Item"
	}
	quoter := cmdline.CommandQuoter{Shell: choice}
	return fmt.Sprintf("%s: %s %s %s [quoted for %s (%s); override with %s]", line, verb,
		quoter.Quote(oldPath), quoter.Quote(newPath), choice.Family,
		shellSourceDetail(choice.Source, choice.Detail), cmdline.ShellOverrideHint)
}

// samePath reports whether two absolute paths name the same file. Windows
// compares case-insensitively; every other platform compares bytes.
func samePath(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}
