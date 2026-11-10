package source

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	ErrInvalidInput = errors.New("invalid import input")
	ErrToolMissing  = errors.New("required local tool is missing")
)

// ImportOptions carries injected dependencies so tests never run poppler or
// depend on the wall clock. A nil PDFText/PDFPageCount uses the local tools.
type ImportOptions struct {
	Title        string
	Now          time.Time
	PDFText      func(path string) (string, error)
	PDFPageCount func(path string) (int, error)
}

// ImportPath reads a local TXT directory, single TXT or PDF into a page-linked
// Source. Imported pages always start unreviewed.
func ImportPath(path string, opts ImportOptions) (Source, error) {
	if strings.TrimSpace(path) == "" {
		return Source{}, fmt.Errorf("%w: empty path", ErrInvalidInput)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return Source{}, fmt.Errorf("%w: resolve %q: %v", ErrInvalidInput, path, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return Source{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	var src Source
	switch {
	case info.IsDir():
		src, err = importTXTDir(abs)
	case strings.EqualFold(filepath.Ext(abs), ".pdf"):
		src, err = importPDF(abs, opts)
	case strings.EqualFold(filepath.Ext(abs), ".txt"):
		src, err = importTXTFile(abs)
	default:
		return Source{}, fmt.Errorf("%w: %s is not a TXT directory, .txt file or .pdf file", ErrInvalidInput, path)
	}
	if err != nil {
		return Source{}, err
	}

	if title := strings.TrimSpace(opts.Title); title != "" {
		src.Title = title
	} else if strings.TrimSpace(src.Title) == "" {
		src.Title = DefaultTitle(abs)
	}
	src.Path = abs
	src.CreatedAt = nowUTC(opts.Now)
	src.Rights = DefaultRights
	src.ExamRelation = DefaultExamRelation
	src.Notice = NoticeFor(src)
	src.ID, src.SHA256 = Identity(src)
	return src, nil
}

var pageFilePattern = regexp.MustCompile(`^page-([0-9]+)\.txt$`)

type discoveredPage struct {
	number int
	path   string
}

func importTXTDir(dir string) (Source, error) {
	pagesDir := filepath.Join(dir, "pages")
	entries, err := os.ReadDir(pagesDir)
	if err != nil {
		if !os.IsNotExist(err) {
			return Source{}, fmt.Errorf("%w: read %s: %v", ErrInvalidInput, pagesDir, err)
		}
		pagesDir = dir
		entries, err = os.ReadDir(pagesDir)
		if err != nil {
			return Source{}, fmt.Errorf("%w: read %s: %v", ErrInvalidInput, pagesDir, err)
		}
	}

	seen := make(map[int]bool)
	var files []discoveredPage
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		match := pageFilePattern.FindStringSubmatch(entry.Name())
		if match == nil {
			continue
		}
		number, err := strconv.Atoi(match[1])
		if err != nil || number < 1 {
			return Source{}, fmt.Errorf("%w: bad page number in %s", ErrInvalidInput, entry.Name())
		}
		if seen[number] {
			return Source{}, fmt.Errorf("%w: page %d appears twice", ErrInvalidInput, number)
		}
		seen[number] = true
		files = append(files, discoveredPage{number: number, path: filepath.Join(pagesDir, entry.Name())})
	}
	if len(files) == 0 {
		return Source{}, fmt.Errorf("%w: %s has no pages/page-NNN.txt files", ErrInvalidInput, dir)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].number < files[j].number })

	manifestPath := filepath.Join(dir, "manifest.tsv")
	manifest, err := readManifest(manifestPath)
	if err != nil {
		return Source{}, err
	}

	total := files[len(files)-1].number
	for page := range manifest {
		if page > total {
			total = page
		}
	}

	byNumber := make(map[int]string, len(files))
	for _, file := range files {
		byNumber[file.number] = file.path
	}

	src := Source{Title: DefaultTitle(dir), Import: ImportTXTDir}
	for number := 1; number <= total; number++ {
		page, ok := manifest[number]
		if path, present := byNumber[number]; present {
			data, err := os.ReadFile(path)
			if err != nil {
				return Source{}, fmt.Errorf("%w: read page %d: %v", ErrInvalidInput, number, err)
			}
			text := string(data)
			if !ValidUTF8(text) {
				return Source{}, fmt.Errorf("%w: page %d is not valid UTF-8", ErrInvalidInput, number)
			}
			origin := PageUnknown
			if ok {
				origin = mapOrigin(page.origin)
			}
			flags := flagsForText(origin, text)
			if ok && (len(data) != page.bytes || CountNonSpace(text) != page.chars) {
				flags = appendFlag(flags, FlagManifestMismatch)
			}
			if ok {
				page.section = strings.TrimSpace(page.section)
			}
			src.Pages = append(src.Pages, Page{
				Number:   number,
				Origin:   origin,
				Section:  page.section,
				Review:   Unreviewed,
				Flags:    flags,
				Text:     text,
				Revision: 1,
			})
			continue
		}
		src.Pages = append(src.Pages, Page{
			Number:   number,
			Origin:   PageNone,
			Section:  strings.TrimSpace(page.section),
			Review:   Unreviewed,
			Flags:    []Flag{FlagPageMissing, FlagEmpty},
			Text:     "",
			Revision: 1,
		})
	}
	return src, nil
}

func importTXTFile(path string) (Source, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Source{}, fmt.Errorf("%w: read %s: %v", ErrInvalidInput, path, err)
	}
	text := string(data)
	if !ValidUTF8(text) {
		return Source{}, fmt.Errorf("%w: %s is not valid UTF-8", ErrInvalidInput, path)
	}
	src := Source{Title: DefaultTitle(path), Import: ImportTXTFile}

	switch {
	case strings.Contains(text, "\f"):
		for i, chunk := range splitFormFeedPages(text) {
			src.Pages = append(src.Pages, Page{
				Number: i + 1, Origin: PageUnknown, Review: Unreviewed,
				Flags: flagsForText(PageUnknown, chunk), Text: chunk, Revision: 1,
			})
		}
	case hasPageMarkers(text):
		pages, total, err := splitPageMarkers(text)
		if err != nil {
			return Source{}, err
		}
		byNumber := make(map[int]markedPage, len(pages))
		for _, page := range pages {
			byNumber[page.number] = page
		}
		if total < len(pages) {
			return Source{}, fmt.Errorf("%w: %s declares %d pages but contains page %d", ErrInvalidInput, path, total, pages[len(pages)-1].number)
		}
		for number := 1; number <= total; number++ {
			page, ok := byNumber[number]
			if !ok {
				src.Pages = append(src.Pages, Page{
					Number: number, Origin: PageNone, Review: Unreviewed,
					Flags: []Flag{FlagPageMissing, FlagEmpty}, Text: "", Revision: 1,
				})
				continue
			}
			src.Pages = append(src.Pages, Page{
				Number: number, Origin: page.origin, Review: Unreviewed,
				Flags: flagsForText(page.origin, page.text), Text: page.text, Revision: 1,
			})
		}
	default:
		src.Pages = []Page{{
			Number: 1, Origin: PageUnknown, Review: Unreviewed,
			Flags: flagsForText(PageUnknown, text),
			Text:  text, Revision: 1,
		}}
	}
	return src, nil
}

func importPDF(path string, opts ImportOptions) (Source, error) {
	pageCountFn := opts.PDFPageCount
	if pageCountFn == nil {
		pageCountFn = defaultPDFPageCount
	}
	textFn := opts.PDFText
	if textFn == nil {
		textFn = defaultPDFText
	}

	count, err := pageCountFn(path)
	if err != nil {
		return Source{}, err
	}
	if count < 1 {
		return Source{}, fmt.Errorf("%w: %s reports %d pages", ErrInvalidInput, path, count)
	}
	text, err := textFn(path)
	if err != nil {
		return Source{}, err
	}
	if !ValidUTF8(text) {
		return Source{}, fmt.Errorf("%w: extracted PDF text is not valid UTF-8", ErrInvalidInput)
	}
	chunks := splitFormFeedPages(text)
	if len(chunks) != count {
		// The extractor did not emit one chunk per page, so position-based page
		// numbers would silently mislabel later pages. Refuse the import.
		return Source{}, fmt.Errorf("%w: extracted %d page chunks but the PDF reports %d pages", ErrInvalidInput, len(chunks), count)
	}

	src := Source{Title: DefaultTitle(path), Import: ImportPDF}
	for i, chunk := range chunks {
		number := i + 1
		chars := CountNonSpace(chunk)
		origin := PageNative
		var flags []Flag
		if chars < ScanThreshold {
			origin = PageNone
			flags = appendFlag(flags, FlagScanUnextracted)
			if chars == 0 {
				flags = appendFlag(flags, FlagEmpty)
			}
		} else {
			flags = appendFlag(flags, FlagReadingOrderUnverified)
		}
		src.Pages = append(src.Pages, Page{
			Number: number, Origin: origin, Review: Unreviewed,
			Flags: flags, Text: chunk, Revision: 1,
		})
	}
	return src, nil
}

func flagsForText(origin PageOrigin, text string) []Flag {
	var flags []Flag
	switch origin {
	case PageOCR:
		flags = appendFlag(flags, FlagOCRUnverified)
		flags = appendFlag(flags, FlagColumnOrderUnverified)
	case PageNative:
		flags = appendFlag(flags, FlagReadingOrderUnverified)
	case PageUnknown:
		flags = appendFlag(flags, FlagProvenanceUnknown)
	}
	return append(flags, textDoubtFlags(text)...)
}

// textDoubtFlags is the single empty/low_text decision shared by import and
// correction.
func textDoubtFlags(text string) []Flag {
	switch {
	case strings.TrimSpace(text) == "":
		return []Flag{FlagEmpty}
	case CountNonSpace(text) < LowTextThreshold:
		return []Flag{FlagLowText}
	default:
		return nil
	}
}

// splitFormFeedPages splits extractor or TXT output into page chunks. A form feed
// normally ends a page, so an immediately following line break belongs to that
// terminator; only a single trailing empty chunk is dropped. An empty chunk
// before the terminator is a legitimate empty page.
func splitFormFeedPages(text string) []string {
	text = strings.ReplaceAll(text, "\f\r\n", "\f")
	text = strings.ReplaceAll(text, "\f\n", "\f")
	chunks := strings.Split(text, "\f")
	if n := len(chunks); n > 1 && chunks[n-1] == "" {
		return chunks[:n-1]
	}
	return chunks
}

func appendFlag(flags []Flag, flag Flag) []Flag {
	for _, existing := range flags {
		if existing == flag {
			return flags
		}
	}
	return append(flags, flag)
}

// FlagsAfterCorrection carries import facts forward, drops doubts that the new
// text resolves, and marks the page corrected. A corrected page is still
// unreviewed until the user reviews it again.
func FlagsAfterCorrection(existing []Flag, text string) []Flag {
	var flags []Flag
	for _, flag := range existing {
		switch flag {
		case FlagEmpty, FlagLowText, FlagScanUnextracted:
			continue
		default:
			flags = appendFlag(flags, flag)
		}
	}
	flags = appendFlag(flags, FlagCorrected)
	return append(flags, textDoubtFlags(text)...)
}

func mapOrigin(raw string) PageOrigin {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "native", "text", "text-layer", "text_layer":
		return PageNative
	case "ocr":
		return PageOCR
	case "none", "scan", "empty", "missing":
		return PageNone
	default:
		return PageUnknown
	}
}

type manifestRow struct {
	section string
	origin  string
	bytes   int
	chars   int
}

func readManifest(path string) (map[int]manifestRow, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("%w: read manifest: %v", ErrInvalidInput, err)
	}
	defer file.Close()

	rows := make(map[int]manifestRow)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	line := 0
	for scanner.Scan() {
		line++
		raw := strings.TrimRight(scanner.Text(), "\r")
		if strings.TrimSpace(raw) == "" {
			continue
		}
		fields := strings.Split(raw, "\t")
		if line == 1 && len(fields) >= 5 && strings.EqualFold(strings.TrimSpace(fields[0]), "page") {
			continue
		}
		if len(fields) < 5 {
			return nil, fmt.Errorf("%w: manifest line %d has %d fields, want 5", ErrInvalidInput, line, len(fields))
		}
		page, err := strconv.Atoi(strings.TrimSpace(fields[0]))
		if err != nil || page < 1 {
			return nil, fmt.Errorf("%w: manifest line %d has bad page %q", ErrInvalidInput, line, fields[0])
		}
		size, err := strconv.Atoi(strings.TrimSpace(fields[3]))
		if err != nil || size < 0 {
			return nil, fmt.Errorf("%w: manifest line %d has bad bytes %q", ErrInvalidInput, line, fields[3])
		}
		chars, err := strconv.Atoi(strings.TrimSpace(fields[4]))
		if err != nil || chars < 0 {
			return nil, fmt.Errorf("%w: manifest line %d has bad chars_no_ws %q", ErrInvalidInput, line, fields[4])
		}
		if _, exists := rows[page]; exists {
			return nil, fmt.Errorf("%w: manifest page %d appears twice", ErrInvalidInput, page)
		}
		rows[page] = manifestRow{
			section: strings.TrimSpace(fields[1]), origin: strings.TrimSpace(fields[2]),
			bytes: size, chars: chars,
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("%w: read manifest: %v", ErrInvalidInput, err)
	}
	return rows, nil
}

var pageMarker = regexp.MustCompile(`(?m)^=== PDF PAGE ([0-9]+) / ([0-9]+)[^\n]*===[ \t]*\r?\n?`)

func hasPageMarkers(text string) bool {
	return pageMarker.MatchString(text)
}

type markedPage struct {
	number int
	origin PageOrigin
	text   string
}

func splitPageMarkers(text string) ([]markedPage, int, error) {
	matches := pageMarker.FindAllStringSubmatchIndex(text, -1)
	if len(matches) == 0 {
		return nil, 0, fmt.Errorf("%w: no page markers", ErrInvalidInput)
	}
	var pages []markedPage
	total := 0
	seen := make(map[int]bool)
	for i, match := range matches {
		header := text[match[0]:match[1]]
		number, _ := strconv.Atoi(text[match[2]:match[3]])
		declared, _ := strconv.Atoi(text[match[4]:match[5]])
		if declared > total {
			total = declared
		}
		if number < 1 || (declared > 0 && number > declared) {
			return nil, 0, fmt.Errorf("%w: malformed page marker %q", ErrInvalidInput, strings.TrimSpace(header))
		}
		if seen[number] {
			return nil, 0, fmt.Errorf("%w: page %d appears twice", ErrInvalidInput, number)
		}
		seen[number] = true

		end := len(text)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}
		origin := PageUnknown
		upper := strings.ToUpper(header)
		switch {
		case strings.Contains(upper, "OCR"):
			origin = PageOCR
		case strings.Contains(upper, "TEXT LAYER"), strings.Contains(upper, "TEXT_LAYER"):
			origin = PageNative
		}
		pages = append(pages, markedPage{number: number, origin: origin, text: text[match[1]:end]})
	}
	if total == 0 {
		total = pages[len(pages)-1].number
	}
	return pages, total, nil
}

func defaultPDFPageCount(path string) (int, error) {
	bin := os.Getenv("PASSQUEST_PDFINFO")
	if bin == "" {
		bin = "pdfinfo"
	}
	out, err := runLocalTool(bin, path)
	if err != nil {
		return 0, err
	}
	match := regexp.MustCompile(`(?m)^Pages:[ \t]+([0-9]+)[ \t]*$`).FindSubmatch(out)
	if match == nil {
		return 0, fmt.Errorf("%w: %s did not report a page count", ErrInvalidInput, bin)
	}
	count, err := strconv.Atoi(string(match[1]))
	if err != nil || count < 1 {
		return 0, fmt.Errorf("%w: %s reported an invalid page count", ErrInvalidInput, bin)
	}
	return count, nil
}

func defaultPDFText(path string) (string, error) {
	bin := os.Getenv("PASSQUEST_PDFTOTEXT")
	if bin == "" {
		bin = "pdftotext"
	}
	out, err := runLocalTool(bin, "-layout", "-enc", "UTF-8", path, "-")
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func runLocalTool(bin string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err == nil {
		return out, nil
	}
	if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s; install poppler or import a UTF-8 TXT baseline instead", ErrToolMissing, bin)
	}
	if ctx.Err() != nil {
		return nil, fmt.Errorf("%w: %s timed out", ErrInvalidInput, bin)
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		message := strings.TrimSpace(stderr.String())
		if len(message) > 400 {
			message = message[:400]
		}
		return nil, fmt.Errorf("%w: %s exited %d: %s", ErrInvalidInput, bin, exitErr.ExitCode(), message)
	}
	return nil, fmt.Errorf("%w: run %s: %v", ErrInvalidInput, bin, err)
}
