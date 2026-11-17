package cli

import (
	"path/filepath"
	"strings"

	"github.com/wxxb789/passquest/internal/note"
	"github.com/wxxb789/passquest/internal/study/outbox"
)

func runNoteExport(args []string, opt Options) int {
	wantJSON := jsonFlagRequested(args)
	args = reorderArgs(args, map[string]bool{"out": true, "db": true})
	fs := newFlagSet("note export", opt, wantJSON)
	outFlag := fs.String("out", "", "output path; default <db dir>/passquest-notes.tsv")
	dbFlag := fs.String("db", "", "SQLite database path")
	jsonFlag := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if code := parseArgs(fs, args, opt, wantJSON, 0, ""); code != 0 {
		return code
	}
	st, err := openDatabase(*dbFlag, opt)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	defer st.Close()
	notes, err := st.Notes()
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	if len(notes) == 0 {
		return usageError(opt, *jsonFlag, "no_notes", "no accepted note is recorded; run passquest note propose and passquest note accept first")
	}
	if message, denied := outbox.ConsentRefusal(st, notes, "exported"); denied {
		return usageError(opt, *jsonFlag, "consent_required", message)
	}
	rows := make([]note.Fields, 0, len(notes))
	for _, n := range notes {
		rows = append(rows, note.HTML(n.Front, n.Back, n.Tactic, n.SourceLabel))
	}
	path := strings.TrimSpace(*outFlag)
	if path == "" {
		path = filepath.Join(filepath.Dir(st.Path()), "passquest-notes.tsv")
	}
	result, err := note.WriteExport(rows, path)
	if err != nil {
		return internalError(opt, *jsonFlag, err)
	}
	doc := noteExportDoc{
		SchemaVersion: ProtocolVersion,
		State:         "exported",
		Path:          result.Path,
		Notes:         result.Notes,
		Bytes:         result.Bytes,
		SHA256:        result.SHA256,
		Header:        result.Header,
		Deck:          note.Deck,
		Model:         note.ModelName,
		Limits:        noteLimits,
	}
	if *jsonFlag {
		return writeJSON(opt, doc)
	}
	writeNoteExport(opt.Stdout, doc)
	return 0
}
