package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/wxxb789/passquest/internal/tutor"
)

// ReserveNoteRefinement claims one slot in the shared daily provider budget.
// It uses the tutor reservation table, so a tutor ask and a note refinement
// cannot both pass the same limit.
func (s *Store) ReserveNoteRefinement(providerID string, since, at time.Time, limit int) (int64, int, error) {
	if limit <= 0 {
		return 0, 0, errors.New("store: provider daily call limit must be positive")
	}
	providerID = strings.TrimSpace(providerID)
	boundary := since.UTC().Format(time.RFC3339Nano)
	tx, err := s.db.Begin()
	if err != nil {
		return 0, 0, fmt.Errorf("store: begin refinement reservation: %w", err)
	}
	defer tx.Rollback()
	var used int
	if err := tx.QueryRow(providerCallCountSQL(true), providerCallCountArgs(providerID, boundary, true)...).Scan(&used); err != nil {
		return 0, 0, fmt.Errorf("store: count provider budget: %w", err)
	}
	if used >= limit {
		return 0, used, nil
	}
	res, err := tx.Exec(`INSERT INTO tutor_call_reservations (provider, created_at) VALUES (?, ?)`,
		providerID, at.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, 0, fmt.Errorf("store: reserve refinement: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, 0, fmt.Errorf("store: read refinement reservation: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, 0, fmt.Errorf("store: commit refinement reservation: %w", err)
	}
	return id, used + 1, nil
}

// FinalizeNoteRefinement stores the refinement outcome and releases its
// reservation in one transaction.
func (s *Store) FinalizeNoteRefinement(r NoteRefinement, reservationID int64) (NoteRefinement, error) {
	if err := validateNoteRefinement(r); err != nil {
		return NoteRefinement{}, err
	}
	if reservationID <= 0 {
		return NoteRefinement{}, errors.New("store: refinement requires a budget reservation")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return NoteRefinement{}, fmt.Errorf("store: begin refinement: %w", err)
	}
	defer tx.Rollback()
	released, err := tx.Exec(`DELETE FROM tutor_call_reservations WHERE id = ?`, reservationID)
	if err != nil {
		return NoteRefinement{}, fmt.Errorf("store: release refinement reservation: %w", err)
	}
	held, err := released.RowsAffected()
	if err != nil {
		return NoteRefinement{}, fmt.Errorf("store: read refinement release: %w", err)
	}
	if held != 1 {
		return NoteRefinement{}, fmt.Errorf("store: refinement reservation %d is not held", reservationID)
	}
	res, err := tx.Exec(`INSERT INTO note_refinements (
			draft_id, created_at, status, error_code, error_message, provider, provider_host, model, prompt_version,
			request_sha256, request_bytes, output_front, output_back, prompt_tokens, completion_tokens,
			http_status, latency_ms
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.DraftID, r.CreatedAt.UTC().Format(time.RFC3339Nano), r.Status, r.ErrorCode, r.ErrorMessage,
		r.Provider, r.ProviderHost, r.Model, r.PromptVersion, r.RequestSHA256, r.RequestBytes,
		r.OutputFront, r.OutputBack, nullableInt(r.PromptTokens), nullableInt(r.CompletionTokens),
		r.HTTPStatus, r.LatencyMS)
	if err != nil {
		return NoteRefinement{}, fmt.Errorf("store: append refinement: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return NoteRefinement{}, fmt.Errorf("store: read refinement id: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return NoteRefinement{}, fmt.Errorf("store: commit refinement: %w", err)
	}
	r.ID = id
	return r, nil
}

func validateNoteRefinement(r NoteRefinement) error {
	if r.CreatedAt.IsZero() {
		return errors.New("store: refinement timestamp is required")
	}
	switch r.Status {
	case "ok":
		if r.ErrorCode != "" {
			return errors.New("store: successful refinement carries an error code")
		}
		if strings.TrimSpace(r.OutputFront) == "" || strings.TrimSpace(r.OutputBack) == "" {
			return errors.New("store: successful refinement has no output")
		}
	case "error":
		if !tutor.ValidHostErrorCode(r.ErrorCode) {
			return fmt.Errorf("store: refinement error code %q is not documented", r.ErrorCode)
		}
		if strings.TrimSpace(r.ErrorMessage) == "" {
			return errors.New("store: failed refinement has no message")
		}
		if r.OutputFront != "" || r.OutputBack != "" {
			return errors.New("store: failed refinement carries output")
		}
	default:
		return fmt.Errorf("store: refinement status %q is invalid", r.Status)
	}
	if strings.TrimSpace(r.ProviderHost) == "" || strings.TrimSpace(r.Model) == "" {
		return errors.New("store: refinement requires provider host and model")
	}
	if strings.TrimSpace(r.PromptVersion) == "" {
		return errors.New("store: refinement prompt version is required")
	}
	if len(r.RequestSHA256) != 64 || strings.ToLower(r.RequestSHA256) != r.RequestSHA256 {
		return errors.New("store: refinement request hash must be 64 lowercase hex characters")
	}
	for _, char := range r.RequestSHA256 {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f')) {
			return errors.New("store: refinement request hash must be 64 lowercase hex characters")
		}
	}
	if r.RequestBytes <= 0 {
		return errors.New("store: refinement request size must be positive")
	}
	if r.HTTPStatus < 0 || r.HTTPStatus > 599 || r.LatencyMS < 0 {
		return errors.New("store: refinement transport numbers are invalid")
	}
	return nil
}

// NoteRefinements returns recorded refinements in insertion order.
func (s *Store) NoteRefinements() ([]NoteRefinement, error) {
	rows, err := s.db.Query(`SELECT id, draft_id, created_at, status, error_code, error_message,
			provider, provider_host, model, prompt_version, request_sha256, request_bytes, output_front,
			output_back, prompt_tokens, completion_tokens, http_status, latency_ms
		FROM note_refinements ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("store: list refinements: %w", err)
	}
	defer rows.Close()
	var out []NoteRefinement
	for rows.Next() {
		var (
			r                NoteRefinement
			created          string
			promptTokens     sql.NullInt64
			completionTokens sql.NullInt64
		)
		if err := rows.Scan(&r.ID, &r.DraftID, &created, &r.Status, &r.ErrorCode, &r.ErrorMessage,
			&r.Provider, &r.ProviderHost, &r.Model, &r.PromptVersion, &r.RequestSHA256, &r.RequestBytes,
			&r.OutputFront, &r.OutputBack, &promptTokens, &completionTokens, &r.HTTPStatus, &r.LatencyMS); err != nil {
			return nil, fmt.Errorf("store: scan refinement: %w", err)
		}
		if r.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
			return nil, fmt.Errorf("store: parse refinement time: %w", err)
		}
		if promptTokens.Valid {
			value := int(promptTokens.Int64)
			r.PromptTokens = &value
		}
		if completionTokens.Valid {
			value := int(completionTokens.Int64)
			r.CompletionTokens = &value
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate refinements: %w", err)
	}
	return out, nil
}
