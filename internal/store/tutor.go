package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/wxxb789/passquest/internal/tutor"
)

// schemaV7 adds the tutor surfaces: non-secret provider configuration, an
// append-only per-source consent trail, and append-only advisory call records.
// None of these tables can change an attempt, a grade or an answer key.
var schemaV7 = []string{
	`CREATE TABLE IF NOT EXISTS tutor_config (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		endpoint TEXT NOT NULL,
		model TEXT NOT NULL,
		timeout_ms INTEGER NOT NULL,
		max_output_tokens INTEGER NOT NULL,
		daily_calls INTEGER NOT NULL,
		updated_at TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS tutor_consent_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		source_id TEXT NOT NULL,
		granted INTEGER NOT NULL CHECK (granted IN (0, 1)),
		reviewer TEXT NOT NULL,
		at TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS tutor_consent_source ON tutor_consent_events(source_id, id)`,
	`CREATE TRIGGER IF NOT EXISTS tutor_consent_append_only_update BEFORE UPDATE ON tutor_consent_events
		BEGIN SELECT RAISE(ABORT, 'tutor consent events are append-only'); END`,
	`CREATE TRIGGER IF NOT EXISTS tutor_consent_append_only_delete BEFORE DELETE ON tutor_consent_events
		BEGIN SELECT RAISE(ABORT, 'tutor consent events are append-only'); END`,
	`CREATE TABLE IF NOT EXISTS tutor_calls (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		created_at TEXT NOT NULL,
		attempt_id INTEGER NOT NULL,
		mode TEXT NOT NULL,
		status TEXT NOT NULL CHECK (status IN ('ok', 'error')),
		error_code TEXT NOT NULL DEFAULT '',
		error_message TEXT NOT NULL DEFAULT '',
		provider_host TEXT NOT NULL,
		model TEXT NOT NULL,
		prompt_version TEXT NOT NULL,
		source_id TEXT NOT NULL DEFAULT '',
		content_id TEXT NOT NULL,
		question_id TEXT NOT NULL,
		source_text_shared INTEGER NOT NULL CHECK (source_text_shared IN (0, 1)),
		request_sha256 TEXT NOT NULL CHECK (length(request_sha256) = 64),
		request_bytes INTEGER NOT NULL CHECK (request_bytes > 0),
		output_text TEXT NOT NULL DEFAULT '',
		output_chars INTEGER NOT NULL CHECK (output_chars >= 0),
		prompt_tokens INTEGER CHECK (prompt_tokens IS NULL OR prompt_tokens >= 0),
		completion_tokens INTEGER CHECK (completion_tokens IS NULL OR completion_tokens >= 0),
		http_status INTEGER NOT NULL DEFAULT 0 CHECK (http_status BETWEEN 0 AND 599),
		latency_ms INTEGER NOT NULL CHECK (latency_ms >= 0),
		CHECK (status = 'ok' OR error_code <> ''),
		CHECK (status = 'error' OR error_code = ''),
		CHECK (status = 'ok' OR output_text = '')
	)`,
	`CREATE INDEX IF NOT EXISTS tutor_calls_attempt ON tutor_calls(attempt_id, id)`,
	`CREATE TRIGGER IF NOT EXISTS tutor_calls_append_only_update BEFORE UPDATE ON tutor_calls
		BEGIN SELECT RAISE(ABORT, 'tutor calls are append-only'); END`,
	`CREATE TRIGGER IF NOT EXISTS tutor_calls_append_only_delete BEFORE DELETE ON tutor_calls
		BEGIN SELECT RAISE(ABORT, 'tutor calls are append-only'); END`,
}

// TutorConsentEvent records one grant or revoke decision for an imported
// source. The effective consent is the latest event; history is retained so a
// later decision cannot silently erase an earlier authorization.
type TutorConsentEvent struct {
	ID       int64
	SourceID string
	Granted  bool
	Reviewer string
	At       time.Time
}

// TutorCall is one append-only advisory record. OutputText is model output,
// not verified content; it is empty for a failed call. Raw prompt text is not
// stored: RequestSHA256 and RequestBytes describe the exact serialized
// provider request body, which the host posts verbatim, so a call that reached
// the provider can be matched against its logs. A call that failed before the
// request was sent still records the prepared body's identity.
type TutorCall struct {
	ID               int64
	CreatedAt        time.Time
	AttemptID        int64
	Mode             string
	Status           string
	ErrorCode        string
	ErrorMessage     string
	ProviderHost     string
	Model            string
	PromptVersion    string
	SourceID         string
	ContentID        string
	QuestionID       string
	SourceTextShared bool
	RequestSHA256    string
	RequestBytes     int
	OutputText       string
	OutputChars      int
	PromptTokens     *int
	CompletionTokens *int
	HTTPStatus       int
	LatencyMS        int64
}

// TutorConfig returns the saved provider configuration. configured is false
// when the learner has not saved one. A saved credential is never part of it.
func (s *Store) TutorConfig() (tutor.Config, bool, error) {
	var cfg tutor.Config
	err := s.db.QueryRow(`SELECT endpoint, model, timeout_ms, max_output_tokens, daily_calls
		FROM tutor_config WHERE id = 1`).
		Scan(&cfg.Endpoint, &cfg.Model, &cfg.TimeoutMS, &cfg.MaxOutputTokens, &cfg.DailyCalls)
	if errors.Is(err, sql.ErrNoRows) {
		return tutor.Config{}, false, nil
	}
	if err != nil {
		return tutor.Config{}, false, fmt.Errorf("store: read tutor config: %w", err)
	}
	return cfg, true, nil
}

// SaveTutorConfig validates and replaces the non-secret provider
// configuration. This is mutable settings, not learner evidence.
func (s *Store) SaveTutorConfig(cfg tutor.Config, at time.Time) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	if at.IsZero() {
		return errors.New("store: tutor config timestamp is required")
	}
	_, err := s.db.Exec(`INSERT INTO tutor_config (
			id, endpoint, model, timeout_ms, max_output_tokens, daily_calls, updated_at
		) VALUES (1, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			endpoint = excluded.endpoint,
			model = excluded.model,
			timeout_ms = excluded.timeout_ms,
			max_output_tokens = excluded.max_output_tokens,
			daily_calls = excluded.daily_calls,
			updated_at = excluded.updated_at`,
		strings.TrimSpace(cfg.Endpoint), strings.TrimSpace(cfg.Model), cfg.TimeoutMS,
		cfg.MaxOutputTokens, cfg.DailyCalls, at.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("store: save tutor config: %w", err)
	}
	return nil
}

// AppendTutorConsent records one grant or revoke event. The caller checks that
// the source exists; this method validates the event shape.
func (s *Store) AppendTutorConsent(event TutorConsentEvent) (TutorConsentEvent, error) {
	event.SourceID = strings.TrimSpace(event.SourceID)
	event.Reviewer = strings.TrimSpace(event.Reviewer)
	if event.SourceID == "" {
		return TutorConsentEvent{}, errors.New("store: tutor consent source id is required")
	}
	if event.Reviewer == "" {
		return TutorConsentEvent{}, errors.New("store: tutor consent reviewer is required")
	}
	if event.At.IsZero() {
		return TutorConsentEvent{}, errors.New("store: tutor consent timestamp is required")
	}
	res, err := s.db.Exec(`INSERT INTO tutor_consent_events (source_id, granted, reviewer, at)
		VALUES (?, ?, ?, ?)`,
		event.SourceID, boolInt(event.Granted), event.Reviewer, event.At.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return TutorConsentEvent{}, fmt.Errorf("store: append tutor consent: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return TutorConsentEvent{}, fmt.Errorf("store: read tutor consent id: %w", err)
	}
	event.ID = id
	event.At = event.At.UTC()
	return event, nil
}

// TutorConsent returns the latest consent event for one source.
func (s *Store) TutorConsent(sourceID string) (TutorConsentEvent, bool, error) {
	row := s.db.QueryRow(`SELECT id, source_id, granted, reviewer, at
		FROM tutor_consent_events WHERE source_id = ? ORDER BY id DESC LIMIT 1`, sourceID)
	event, err := scanTutorConsent(row)
	if errors.Is(err, sql.ErrNoRows) {
		return TutorConsentEvent{}, false, nil
	}
	if err != nil {
		return TutorConsentEvent{}, false, fmt.Errorf("store: read tutor consent: %w", err)
	}
	return event, true, nil
}

// TutorConsents returns the latest event for every source that has one. A
// source with no event is absent, which means private text is withheld.
func (s *Store) TutorConsents() ([]TutorConsentEvent, error) {
	rows, err := s.db.Query(`SELECT e.id, e.source_id, e.granted, e.reviewer, e.at
		FROM tutor_consent_events e
		WHERE e.id = (SELECT MAX(latest.id) FROM tutor_consent_events latest WHERE latest.source_id = e.source_id)
		ORDER BY e.source_id`)
	if err != nil {
		return nil, fmt.Errorf("store: list tutor consents: %w", err)
	}
	defer rows.Close()
	var out []TutorConsentEvent
	for rows.Next() {
		event, err := scanTutorConsent(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan tutor consent: %w", err)
		}
		out = append(out, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate tutor consents: %w", err)
	}
	return out, nil
}

func scanTutorConsent(row rowScanner) (TutorConsentEvent, error) {
	var (
		event   TutorConsentEvent
		granted int
		at      string
	)
	if err := row.Scan(&event.ID, &event.SourceID, &granted, &event.Reviewer, &at); err != nil {
		return TutorConsentEvent{}, err
	}
	event.Granted = granted == 1
	parsed, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		return TutorConsentEvent{}, fmt.Errorf("parse tutor consent time: %w", err)
	}
	event.At = parsed
	return event, nil
}

// AppendTutorCall stores one advisory call attempt and returns it with its id.
// A failed call is recorded too: usage and failure visibility is part of the
// contract, while the attempt table remains untouched.
func (s *Store) AppendTutorCall(call TutorCall) (TutorCall, error) {
	if err := validateTutorCall(call); err != nil {
		return TutorCall{}, err
	}
	res, err := s.db.Exec(`INSERT INTO tutor_calls (
			created_at, attempt_id, mode, status, error_code, error_message,
			provider_host, model, prompt_version, source_id, content_id, question_id,
			source_text_shared, request_sha256, request_bytes, output_text, output_chars,
			prompt_tokens, completion_tokens, http_status, latency_ms
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		call.CreatedAt.UTC().Format(time.RFC3339Nano), call.AttemptID, call.Mode, call.Status,
		call.ErrorCode, call.ErrorMessage, call.ProviderHost, call.Model, call.PromptVersion,
		call.SourceID, call.ContentID, call.QuestionID, boolInt(call.SourceTextShared),
		call.RequestSHA256, call.RequestBytes, call.OutputText, call.OutputChars,
		nullableInt(call.PromptTokens), nullableInt(call.CompletionTokens), call.HTTPStatus, call.LatencyMS)
	if err != nil {
		return TutorCall{}, fmt.Errorf("store: append tutor call: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return TutorCall{}, fmt.Errorf("store: read tutor call id: %w", err)
	}
	call.ID = id
	call.CreatedAt = call.CreatedAt.UTC()
	return call, nil
}

// tutorCallColumns is the one column order both call queries scan.
const tutorCallColumns = `id, created_at, attempt_id, mode, status, error_code, error_message,
			provider_host, model, prompt_version, source_id, content_id, question_id,
			source_text_shared, request_sha256, request_bytes, output_text, output_chars,
			prompt_tokens, completion_tokens, http_status, latency_ms`

// TutorCalls lists advisory calls in insertion order. attemptID 0 lists all.
func (s *Store) TutorCalls(attemptID int64) ([]TutorCall, error) {
	query := `SELECT ` + tutorCallColumns + `
		FROM tutor_calls`
	args := []any{}
	if attemptID != 0 {
		query += ` WHERE attempt_id = ?`
		args = append(args, attemptID)
	}
	query += ` ORDER BY id`
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list tutor calls: %w", err)
	}
	defer rows.Close()
	var out []TutorCall
	for rows.Next() {
		call, err := scanTutorCall(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan tutor call: %w", err)
		}
		out = append(out, call)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate tutor calls: %w", err)
	}
	return out, nil
}

// TutorCall returns one advisory call by id.
func (s *Store) TutorCall(id int64) (TutorCall, error) {
	row := s.db.QueryRow(`SELECT `+tutorCallColumns+`
		FROM tutor_calls WHERE id = ?`, id)
	call, err := scanTutorCall(row)
	if errors.Is(err, sql.ErrNoRows) {
		return TutorCall{}, ErrNotFound
	}
	if err != nil {
		return TutorCall{}, fmt.Errorf("store: read tutor call: %w", err)
	}
	return call, nil
}

// TutorCallCountSince counts calls at or after at, including failures, for the
// daily budget. SQLite parses the stored RFC3339 timestamps.
func (s *Store) TutorCallCountSince(at time.Time) (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM tutor_calls WHERE julianday(created_at) >= julianday(?)`,
		at.UTC().Format(time.RFC3339Nano)).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("store: count tutor calls: %w", err)
	}
	return count, nil
}

func scanTutorCall(row rowScanner) (TutorCall, error) {
	var (
		call             TutorCall
		created          string
		sourceTextShared int
		promptTokens     sql.NullInt64
		completionTokens sql.NullInt64
	)
	if err := row.Scan(
		&call.ID, &created, &call.AttemptID, &call.Mode, &call.Status, &call.ErrorCode, &call.ErrorMessage,
		&call.ProviderHost, &call.Model, &call.PromptVersion, &call.SourceID, &call.ContentID, &call.QuestionID,
		&sourceTextShared, &call.RequestSHA256, &call.RequestBytes, &call.OutputText, &call.OutputChars,
		&promptTokens, &completionTokens, &call.HTTPStatus, &call.LatencyMS,
	); err != nil {
		return TutorCall{}, err
	}
	parsed, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return TutorCall{}, fmt.Errorf("parse tutor call time: %w", err)
	}
	call.CreatedAt = parsed
	call.SourceTextShared = sourceTextShared == 1
	if promptTokens.Valid {
		value := int(promptTokens.Int64)
		call.PromptTokens = &value
	}
	if completionTokens.Valid {
		value := int(completionTokens.Int64)
		call.CompletionTokens = &value
	}
	return call, nil
}

func validateTutorCall(call TutorCall) error {
	var problems []string
	add := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}
	if call.CreatedAt.IsZero() {
		add("created_at is required")
	}
	if call.AttemptID <= 0 {
		add("attempt_id must be positive")
	}
	if !tutor.Mode(call.Mode).Valid() {
		add("mode %q is invalid", call.Mode)
	}
	switch call.Status {
	case "ok":
		if call.ErrorCode != "" {
			add("ok call has error code %q", call.ErrorCode)
		}
		if strings.TrimSpace(call.OutputText) == "" {
			add("ok call has no output text")
		}
		if call.OutputChars != len([]rune(call.OutputText)) {
			add("output_chars %d does not match %d runes", call.OutputChars, len([]rune(call.OutputText)))
		}
	case "error":
		if !tutor.ValidHostErrorCode(call.ErrorCode) {
			add("error code %q is not documented", call.ErrorCode)
		}
		if strings.TrimSpace(call.ErrorMessage) == "" {
			add("error call has no message")
		}
		if len([]rune(call.ErrorMessage)) > 1000 {
			add("error message exceeds 1000 characters")
		}
		if call.OutputText != "" || call.OutputChars != 0 {
			add("error call carries output text")
		}
	default:
		add("status %q is invalid", call.Status)
	}
	if call.OutputChars > tutor.MaxResponseChars {
		add("output_chars %d exceeds %d", call.OutputChars, tutor.MaxResponseChars)
	}
	if strings.TrimSpace(call.ProviderHost) == "" {
		add("provider_host is required")
	}
	if strings.TrimSpace(call.Model) == "" {
		add("model is required")
	}
	if strings.TrimSpace(call.PromptVersion) == "" {
		add("prompt_version is required")
	}
	if strings.TrimSpace(call.ContentID) == "" || strings.TrimSpace(call.QuestionID) == "" {
		add("content_id and question_id are required")
	}
	if len(call.RequestSHA256) != 64 || strings.ToLower(call.RequestSHA256) != call.RequestSHA256 {
		add("request_sha256 must be 64 lowercase hex characters")
	}
	for _, r := range call.RequestSHA256 {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			add("request_sha256 must be lowercase hex")
			break
		}
	}
	if call.RequestBytes <= 0 {
		add("request_bytes must be positive")
	}
	if call.PromptTokens != nil && *call.PromptTokens < 0 {
		add("prompt_tokens is negative")
	}
	if call.CompletionTokens != nil && *call.CompletionTokens < 0 {
		add("completion_tokens is negative")
	}
	if call.HTTPStatus < 0 || call.HTTPStatus > 599 {
		add("http_status %d is outside 0..599", call.HTTPStatus)
	}
	if call.LatencyMS < 0 {
		add("latency_ms is negative")
	}
	if len(problems) > 0 {
		return fmt.Errorf("store: invalid tutor call: %s", strings.Join(problems, "; "))
	}
	return nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func nullableInt(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}
