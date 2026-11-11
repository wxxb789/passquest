package note

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ColumnsDirective declares the four note fields in model order. Anki treats a
// line beginning with '#' as an import option, so this line is not imported as
// a note; setting #columns also makes field count and order explicit.
const ColumnsDirective = "#columns:Front\tBack\tTactic\tSource"

// exportPrelude holds the importer directives. They declare the exact
// separator, HTML field content, note type, deck and tag so a manual import
// needs no guessing.
const exportPrelude = "#separator:tab\n#html:true\n#notetype:" + ModelName + "\n#deck:" + Deck + "\n#tags:" + Tag + "\n"

// ExportResult describes one written file.
type ExportResult struct {
	Path   string
	Notes  int
	Bytes  int
	SHA256 string
	Header string
}

// ExportBytes builds the exact Anki-importable text. Fields are already HTML,
// so a tab or newline in a value would corrupt the row; both are rejected.
func ExportBytes(rows []Fields) ([]byte, error) {
	var builder strings.Builder
	builder.WriteString(exportPrelude)
	builder.WriteString(ColumnsDirective)
	builder.WriteString("\n")
	for i, row := range rows {
		for name, value := range row.Map() {
			if strings.ContainsAny(value, "\t\r\n") {
				return nil, fmt.Errorf("note: field %s of row %d contains a tab or newline", name, i+1)
			}
		}
		// Anki's text importer reads a row starting with '#' as an option or
		// comment line. The entity keeps the learner's visible text while making
		// the row unambiguously data.
		front := row.Front
		if strings.HasPrefix(front, "#") {
			front = "&#35;" + front[1:]
		}
		fmt.Fprintf(&builder, "%s\t%s\t%s\t%s\n", front, row.Back, row.Tactic, row.Source)
	}
	return []byte(builder.String()), nil
}

// WriteExport writes the import file atomically: a temp file in the destination
// directory is synced and renamed over the target. A failure leaves an existing
// export untouched and removes the temp file.
func WriteExport(rows []Fields, path string) (ExportResult, error) {
	if strings.TrimSpace(path) == "" {
		return ExportResult{}, fmt.Errorf("note: export path is required")
	}
	data, err := ExportBytes(rows)
	if err != nil {
		return ExportResult{}, err
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return ExportResult{}, fmt.Errorf("note: resolve export path: %w", err)
	}
	dir := filepath.Dir(absolute)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return ExportResult{}, fmt.Errorf("note: create export directory: %w", err)
	}
	temp, err := os.CreateTemp(dir, "."+filepath.Base(absolute)+".tmp-*")
	if err != nil {
		return ExportResult{}, fmt.Errorf("note: create temporary export: %w", err)
	}
	tempPath := temp.Name()
	cleanup := func() {
		name := temp.Name()
		if name != "" {
			temp.Close()
			os.Remove(name)
		}
	}
	if err := temp.Chmod(0o600); err != nil {
		cleanup()
		return ExportResult{}, fmt.Errorf("note: protect temporary export: %w", err)
	}
	if _, err := temp.Write(data); err != nil {
		cleanup()
		return ExportResult{}, fmt.Errorf("note: write export: %w", err)
	}
	if err := temp.Sync(); err != nil {
		cleanup()
		return ExportResult{}, fmt.Errorf("note: sync export: %w", err)
	}
	if err := temp.Close(); err != nil {
		os.Remove(tempPath)
		return ExportResult{}, fmt.Errorf("note: close export: %w", err)
	}
	if err := os.Rename(tempPath, absolute); err != nil {
		os.Remove(tempPath)
		return ExportResult{}, fmt.Errorf("note: replace export: %w", err)
	}
	sum := sha256.Sum256(data)
	return ExportResult{
		Path:   absolute,
		Notes:  len(rows),
		Bytes:  len(data),
		SHA256: hex.EncodeToString(sum[:]),
		Header: ColumnsDirective,
	}, nil
}
