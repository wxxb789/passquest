package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// schemaV15 separates the provider identity from the endpoint and records the
// two append-only evidence trails the provider surface needs: capability
// verification and bounded failover decisions. The provider columns default to
// the empty legacy id so a database created before this increment still opens
// and its old calls count as unattributed rather than against a named provider.
var schemaV15 = []string{
	`ALTER TABLE tutor_config ADD COLUMN provider TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE tutor_consent_events ADD COLUMN provider TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE tutor_calls ADD COLUMN provider TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE tutor_calls ADD COLUMN failover_from TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE tutor_call_reservations ADD COLUMN provider TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE note_refinements ADD COLUMN provider TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE agent_runs ADD COLUMN provider TEXT NOT NULL DEFAULT ''`,
	`CREATE TABLE IF NOT EXISTS provider_verification_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		provider TEXT NOT NULL,
		model TEXT NOT NULL,
		capability TEXT NOT NULL,
		status TEXT NOT NULL CHECK (status IN ('verified', 'failed')),
		failure_type TEXT NOT NULL DEFAULT '',
		action TEXT NOT NULL DEFAULT '',
		call_made INTEGER NOT NULL DEFAULT 0 CHECK (call_made IN (0, 1)),
		latency_ms INTEGER NOT NULL DEFAULT 0 CHECK (latency_ms >= 0),
		http_status INTEGER NOT NULL DEFAULT 0 CHECK (http_status BETWEEN 0 AND 599),
		checked_at TEXT NOT NULL,
		CHECK (status = 'verified' OR failure_type <> ''),
		CHECK (status = 'failed' OR failure_type = '')
	)`,
	`CREATE INDEX IF NOT EXISTS provider_verification_lookup
		ON provider_verification_events(provider, model, capability, id)`,
	`CREATE TRIGGER IF NOT EXISTS provider_verification_append_only_update
		BEFORE UPDATE ON provider_verification_events
		BEGIN SELECT RAISE(ABORT, 'provider verifications are append-only'); END`,
	`CREATE TRIGGER IF NOT EXISTS provider_verification_append_only_delete
		BEFORE DELETE ON provider_verification_events
		BEGIN SELECT RAISE(ABORT, 'provider verifications are append-only'); END`,
	`CREATE TABLE IF NOT EXISTS provider_failover_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		at TEXT NOT NULL,
		from_provider TEXT NOT NULL,
		from_model TEXT NOT NULL,
		to_provider TEXT NOT NULL DEFAULT '',
		to_model TEXT NOT NULL DEFAULT '',
		reason TEXT NOT NULL,
		applied INTEGER NOT NULL CHECK (applied IN (0, 1)),
		refusal TEXT NOT NULL DEFAULT '',
		from_call_id INTEGER NOT NULL DEFAULT 0,
		to_call_id INTEGER NOT NULL DEFAULT 0,
		CHECK (applied = 0 OR refusal = '')
	)`,
	`CREATE INDEX IF NOT EXISTS provider_failover_at ON provider_failover_events(at, id)`,
	`CREATE TRIGGER IF NOT EXISTS provider_failover_append_only_update
		BEFORE UPDATE ON provider_failover_events
		BEGIN SELECT RAISE(ABORT, 'provider failover events are append-only'); END`,
	`CREATE TRIGGER IF NOT EXISTS provider_failover_append_only_delete
		BEFORE DELETE ON provider_failover_events
		BEGIN SELECT RAISE(ABORT, 'provider failover events are append-only'); END`,
}

// ProviderVerification is one append-only capability check. Status is
// "verified" or "failed"; a failure carries the typed failure and its action.
type ProviderVerification struct {
	ID          int64
	Provider    string
	Model       string
	Capability  string
	Status      string
	FailureType string
	Action      string
	CallMade    bool
	LatencyMS   int64
	HTTPStatus  int
	CheckedAt   time.Time
}

// ProviderFailover is one bounded retry decision. Applied is true only when
// the retry call was made; Refusal names why it was not.
type ProviderFailover struct {
	ID           int64
	At           time.Time
	FromProvider string
	FromModel    string
	ToProvider   string
	ToModel      string
	Reason       string
	Applied      bool
	Refusal      string
	FromCallID   int64
	ToCallID     int64
}

// ProviderCallCount is one provider's UTC-day call count.
type ProviderCallCount struct {
	Provider string
	Calls    int
}

// AppendProviderVerification records one capability check. It never changes
// an attempt, a call or a credential.
func (s *Store) AppendProviderVerification(v ProviderVerification) (ProviderVerification, error) {
	v.Provider = strings.TrimSpace(v.Provider)
	v.Model = strings.TrimSpace(v.Model)
	v.Capability = strings.TrimSpace(v.Capability)
	if v.Provider == "" || v.Model == "" || v.Capability == "" {
		return ProviderVerification{}, errors.New("store: provider verification requires provider, model and capability")
	}
	switch v.Status {
	case "verified":
		if v.FailureType != "" {
			return ProviderVerification{}, errors.New("store: a verified capability carries a failure type")
		}
	case "failed":
		if strings.TrimSpace(v.FailureType) == "" {
			return ProviderVerification{}, errors.New("store: a failed capability requires a failure type")
		}
	default:
		return ProviderVerification{}, fmt.Errorf("store: verification status %q is invalid", v.Status)
	}
	if v.CheckedAt.IsZero() {
		return ProviderVerification{}, errors.New("store: provider verification timestamp is required")
	}
	res, err := s.db.Exec(`INSERT INTO provider_verification_events (
			provider, model, capability, status, failure_type, action, call_made, latency_ms, http_status, checked_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		v.Provider, v.Model, v.Capability, v.Status, v.FailureType, v.Action,
		boolInt(v.CallMade), v.LatencyMS, v.HTTPStatus, v.CheckedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return ProviderVerification{}, fmt.Errorf("store: append provider verification: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return ProviderVerification{}, fmt.Errorf("store: read provider verification id: %w", err)
	}
	v.ID = id
	v.CheckedAt = v.CheckedAt.UTC()
	return v, nil
}

// LatestProviderVerification returns the newest check for one capability.
func (s *Store) LatestProviderVerification(providerID, modelID, capability string) (ProviderVerification, bool, error) {
	row := s.db.QueryRow(`SELECT id, provider, model, capability, status, failure_type, action,
			call_made, latency_ms, http_status, checked_at
		FROM provider_verification_events
		WHERE provider = ? AND model = ? AND capability = ?
		ORDER BY id DESC LIMIT 1`, providerID, modelID, capability)
	v, err := scanProviderVerification(row)
	if errors.Is(err, sql.ErrNoRows) {
		return ProviderVerification{}, false, nil
	}
	if err != nil {
		return ProviderVerification{}, false, fmt.Errorf("store: read provider verification: %w", err)
	}
	return v, true, nil
}

// ProviderVerifications returns the latest check per (provider, model,
// capability), in insertion order.
func (s *Store) ProviderVerifications() ([]ProviderVerification, error) {
	rows, err := s.db.Query(`SELECT e.id, e.provider, e.model, e.capability, e.status,
			e.failure_type, e.action, e.call_made, e.latency_ms, e.http_status, e.checked_at
		FROM provider_verification_events e
		WHERE e.id = (
			SELECT MAX(latest.id) FROM provider_verification_events latest
			WHERE latest.provider = e.provider AND latest.model = e.model AND latest.capability = e.capability
		)
		ORDER BY e.id`)
	if err != nil {
		return nil, fmt.Errorf("store: list provider verifications: %w", err)
	}
	defer rows.Close()
	var out []ProviderVerification
	for rows.Next() {
		v, err := scanProviderVerification(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan provider verification: %w", err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate provider verifications: %w", err)
	}
	return out, nil
}

func scanProviderVerification(row rowScanner) (ProviderVerification, error) {
	var (
		v        ProviderVerification
		callMade int
		checked  string
	)
	if err := row.Scan(&v.ID, &v.Provider, &v.Model, &v.Capability, &v.Status,
		&v.FailureType, &v.Action, &callMade, &v.LatencyMS, &v.HTTPStatus, &checked); err != nil {
		return ProviderVerification{}, err
	}
	v.CallMade = callMade == 1
	parsed, err := time.Parse(time.RFC3339Nano, checked)
	if err != nil {
		return ProviderVerification{}, fmt.Errorf("parse provider verification time: %w", err)
	}
	v.CheckedAt = parsed
	return v, nil
}

// AppendProviderFailover records one retry decision, applied or refused.
func (s *Store) AppendProviderFailover(f ProviderFailover) (ProviderFailover, error) {
	f.FromProvider = strings.TrimSpace(f.FromProvider)
	f.FromModel = strings.TrimSpace(f.FromModel)
	f.ToProvider = strings.TrimSpace(f.ToProvider)
	f.ToModel = strings.TrimSpace(f.ToModel)
	f.Reason = strings.TrimSpace(f.Reason)
	f.Refusal = strings.TrimSpace(f.Refusal)
	if f.At.IsZero() || f.FromProvider == "" || f.FromModel == "" || f.Reason == "" {
		return ProviderFailover{}, errors.New("store: failover event requires time, from provider/model and reason")
	}
	if f.Applied {
		if f.ToProvider == "" || f.ToModel == "" {
			return ProviderFailover{}, errors.New("store: applied failover requires a target provider/model")
		}
		if f.Refusal != "" {
			return ProviderFailover{}, errors.New("store: applied failover carries a refusal")
		}
	} else if f.Refusal == "" {
		return ProviderFailover{}, errors.New("store: refused failover requires a refusal reason")
	}
	res, err := s.db.Exec(`INSERT INTO provider_failover_events (
			at, from_provider, from_model, to_provider, to_model, reason, applied, refusal,
			from_call_id, to_call_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		f.At.UTC().Format(time.RFC3339Nano), f.FromProvider, f.FromModel, f.ToProvider, f.ToModel,
		f.Reason, boolInt(f.Applied), f.Refusal, f.FromCallID, f.ToCallID)
	if err != nil {
		return ProviderFailover{}, fmt.Errorf("store: append provider failover: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return ProviderFailover{}, fmt.Errorf("store: read provider failover id: %w", err)
	}
	f.ID = id
	f.At = f.At.UTC()
	return f, nil
}

// ProviderFailovers returns the recorded decisions in insertion order.
func (s *Store) ProviderFailovers() ([]ProviderFailover, error) {
	rows, err := s.db.Query(`SELECT id, at, from_provider, from_model, to_provider, to_model,
			reason, applied, refusal, from_call_id, to_call_id
		FROM provider_failover_events ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("store: list provider failovers: %w", err)
	}
	defer rows.Close()
	var out []ProviderFailover
	for rows.Next() {
		var (
			f       ProviderFailover
			at      string
			applied int
		)
		if err := rows.Scan(&f.ID, &at, &f.FromProvider, &f.FromModel, &f.ToProvider, &f.ToModel,
			&f.Reason, &applied, &f.Refusal, &f.FromCallID, &f.ToCallID); err != nil {
			return nil, fmt.Errorf("store: scan provider failover: %w", err)
		}
		parsed, err := time.Parse(time.RFC3339Nano, at)
		if err != nil {
			return nil, fmt.Errorf("store: parse provider failover time: %w", err)
		}
		f.At = parsed
		f.Applied = applied == 1
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate provider failovers: %w", err)
	}
	return out, nil
}

// ProviderCallCountsSince counts every provider call recorded at or after at:
// tutor calls (including failures), in-flight reservations and note
// refinements, grouped by provider. The legacy empty provider is returned as
// "", which the CLI renders as unattributed.
func (s *Store) ProviderCallCountsSince(at time.Time) ([]ProviderCallCount, error) {
	boundary := at.UTC().Format(time.RFC3339Nano)
	rows, err := s.db.Query(`SELECT provider, COUNT(*) FROM (
			SELECT provider FROM tutor_calls WHERE julianday(created_at) >= julianday(?)
			UNION ALL
			SELECT provider FROM tutor_call_reservations WHERE julianday(created_at) >= julianday(?)
			UNION ALL
			SELECT provider FROM note_refinements WHERE julianday(created_at) >= julianday(?)
			UNION ALL
			SELECT provider FROM provider_verification_events
				WHERE call_made = 1 AND julianday(checked_at) >= julianday(?)
		) GROUP BY provider ORDER BY provider`, boundary, boundary, boundary, boundary)
	if err != nil {
		return nil, fmt.Errorf("store: count provider calls: %w", err)
	}
	defer rows.Close()
	var out []ProviderCallCount
	for rows.Next() {
		var item ProviderCallCount
		if err := rows.Scan(&item.Provider, &item.Calls); err != nil {
			return nil, fmt.Errorf("store: scan provider call count: %w", err)
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate provider call counts: %w", err)
	}
	return out, nil
}
