package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// schemaV13 adds the search surface: a transient reservation ledger for the
// daily query budget, an append-only query audit and append-only classified
// candidates. None of these tables can change an attempt, a grade, an answer
// key or a source. Fetched text is stored only for a local-usable candidate;
// a CHECK makes the other two classes text-free.
var schemaV13 = []string{
	`CREATE TABLE IF NOT EXISTS find_query_reservations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		day TEXT NOT NULL CHECK (length(day) = 10),
		created_at TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS find_query_reservations_day ON find_query_reservations(day, id)`,
	`CREATE TABLE IF NOT EXISTS find_queries (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		day TEXT NOT NULL CHECK (length(day) = 10),
		created_at TEXT NOT NULL,
		intent_json TEXT NOT NULL CHECK (length(intent_json) BETWEEN 2 AND 30000),
		skill_id TEXT NOT NULL DEFAULT '' CHECK (length(skill_id) <= 100),
		tactic_id TEXT NOT NULL DEFAULT '' CHECK (length(tactic_id) <= 100),
		exam_section TEXT NOT NULL DEFAULT '' CHECK (length(exam_section) <= 100),
		learner_level TEXT NOT NULL DEFAULT '' CHECK (length(learner_level) <= 40),
		query TEXT NOT NULL CHECK (length(query) BETWEEN 1 AND 300),
		provider TEXT NOT NULL DEFAULT '' CHECK (length(provider) <= 200),
		endpoint_host TEXT NOT NULL DEFAULT '' CHECK (length(endpoint_host) <= 300),
		status TEXT NOT NULL CHECK (status IN (
			'ok', 'empty', 'all_link_only', 'candidate_limit_exceeded',
			'rate_limited', 'auth_failed', 'provider_error', 'unreachable',
			'invalid_response', 'timeout')),
		error_code TEXT NOT NULL DEFAULT '' CHECK (length(error_code) <= 100),
		error_message TEXT NOT NULL DEFAULT '' CHECK (length(error_message) <= 2000),
		http_status INTEGER NOT NULL DEFAULT 0 CHECK (http_status BETWEEN 0 AND 599),
		candidate_count INTEGER NOT NULL DEFAULT 0 CHECK (candidate_count >= 0),
		max_candidates INTEGER NOT NULL CHECK (max_candidates BETWEEN 1 AND 100),
		request_sha256 TEXT NOT NULL DEFAULT ''
			CHECK (request_sha256 = '' OR length(request_sha256) = 64),
		request_bytes INTEGER NOT NULL DEFAULT 0 CHECK (request_bytes >= 0),
		latency_ms INTEGER NOT NULL DEFAULT 0 CHECK (latency_ms >= 0),
		CHECK (status IN ('ok', 'empty', 'all_link_only') OR error_code <> '')
	)`,
	`CREATE INDEX IF NOT EXISTS find_queries_day ON find_queries(day, id)`,
	`CREATE TRIGGER IF NOT EXISTS find_queries_append_only_update BEFORE UPDATE ON find_queries
		BEGIN SELECT RAISE(ABORT, 'find queries are append-only'); END`,
	`CREATE TRIGGER IF NOT EXISTS find_queries_append_only_delete BEFORE DELETE ON find_queries
		BEGIN SELECT RAISE(ABORT, 'find queries are append-only'); END`,
	`CREATE TABLE IF NOT EXISTS find_candidates (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		query_id INTEGER NOT NULL REFERENCES find_queries(id),
		provider TEXT NOT NULL CHECK (length(provider) BETWEEN 1 AND 200),
		query TEXT NOT NULL CHECK (length(query) BETWEEN 1 AND 300),
		retrieved_at TEXT NOT NULL,
		title TEXT NOT NULL CHECK (length(title) BETWEEN 1 AND 300),
		url TEXT NOT NULL CHECK (length(url) BETWEEN 1 AND 2048),
		classification TEXT NOT NULL CHECK (classification IN (
			'link-only', 'local-usable', 'generated-unverified')),
		license TEXT NOT NULL DEFAULT '' CHECK (length(license) <= 100),
		reason TEXT NOT NULL DEFAULT '' CHECK (length(reason) <= 100),
		content TEXT NOT NULL DEFAULT '' CHECK (length(content) <= 20000),
		content_sha256 TEXT NOT NULL CHECK (length(content_sha256) = 64),
		displayable INTEGER NOT NULL CHECK (displayable IN (0, 1)),
		CHECK (
			(classification = 'local-usable' AND content <> '' AND displayable = 1)
			OR (classification IN ('link-only', 'generated-unverified')
				AND content = '' AND displayable = 0)
		)
	)`,
	`CREATE INDEX IF NOT EXISTS find_candidates_query ON find_candidates(query_id, id)`,
	`CREATE TRIGGER IF NOT EXISTS find_candidates_append_only_update BEFORE UPDATE ON find_candidates
		BEGIN SELECT RAISE(ABORT, 'find candidates are append-only'); END`,
	`CREATE TRIGGER IF NOT EXISTS find_candidates_append_only_delete BEFORE DELETE ON find_candidates
		BEGIN SELECT RAISE(ABORT, 'find candidates are append-only'); END`,
}

// FindQuery is one attempted search. A query is appended only after the host
// returns, so the row records the final outcome; the reservation covers the
// window before that.
type FindQuery struct {
	ID             int64
	Day            string
	CreatedAt      time.Time
	IntentJSON     string
	SkillID        string
	TacticID       string
	ExamSection    string
	LearnerLevel   string
	Query          string
	Provider       string
	EndpointHost   string
	Status         string
	ErrorCode      string
	ErrorMessage   string
	HTTPStatus     int
	CandidateCount int
	MaxCandidates  int
	RequestSHA256  string
	RequestBytes   int
	LatencyMS      int64
}

// FindCandidate is one classified provider result with direct provenance.
type FindCandidate struct {
	ID             int64
	QueryID        int64
	Provider       string
	Query          string
	RetrievedAt    time.Time
	Title          string
	URL            string
	Classification string
	License        string
	Reason         string
	Content        string
	ContentSHA256  string
	Displayable    bool
}

// FindUsage is the persisted budget state for one UTC day. Reservations count
// as used: a process that dies after claiming a slot can only under-spend.
type FindUsage struct {
	Day          string
	Queries      int
	Reservations int
}

// Used returns the number of slots spent or claimed on the day.
func (u FindUsage) Used() int { return u.Queries + u.Reservations }

// FindQueryUsage counts stored queries and outstanding reservations for a day.
func (s *Store) FindQueryUsage(day string) (FindUsage, error) {
	if err := validateFindDay(day); err != nil {
		return FindUsage{}, err
	}
	usage := FindUsage{Day: day}
	err := s.db.QueryRow(`SELECT
			(SELECT COUNT(*) FROM find_queries WHERE day = ?),
			(SELECT COUNT(*) FROM find_query_reservations WHERE day = ?)`,
		day, day).Scan(&usage.Queries, &usage.Reservations)
	if err != nil {
		return FindUsage{}, fmt.Errorf("store: read find budget: %w", err)
	}
	return usage, nil
}

// ReserveFindQuery claims one daily slot inside an immediate transaction. The
// returned usage includes the claim when ok is true.
func (s *Store) ReserveFindQuery(day string, max int, at time.Time) (int64, FindUsage, bool, error) {
	if err := validateFindDay(day); err != nil {
		return 0, FindUsage{}, false, err
	}
	if max < 1 {
		return 0, FindUsage{}, false, errors.New("store: find query budget must be positive")
	}
	if at.IsZero() {
		return 0, FindUsage{}, false, errors.New("store: find reservation timestamp is required")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, FindUsage{}, false, fmt.Errorf("store: begin find reservation: %w", err)
	}
	defer tx.Rollback()

	usage := FindUsage{Day: day}
	if err := tx.QueryRow(`SELECT
			(SELECT COUNT(*) FROM find_queries WHERE day = ?),
			(SELECT COUNT(*) FROM find_query_reservations WHERE day = ?)`,
		day, day).Scan(&usage.Queries, &usage.Reservations); err != nil {
		return 0, FindUsage{}, false, fmt.Errorf("store: read find budget: %w", err)
	}
	if usage.Used() >= max {
		return 0, usage, false, nil
	}
	res, err := tx.Exec(`INSERT INTO find_query_reservations (day, created_at) VALUES (?, ?)`,
		day, at.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, FindUsage{}, false, fmt.Errorf("store: reserve find query: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, FindUsage{}, false, fmt.Errorf("store: read find reservation id: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, FindUsage{}, false, fmt.Errorf("store: commit find reservation: %w", err)
	}
	usage.Reservations++
	return id, usage, true, nil
}

// ReleaseFindQueryReservation removes a claim made for a request that provably
// never left the machine. A sent-but-failed request keeps its slot.
func (s *Store) ReleaseFindQueryReservation(id int64) error {
	if id <= 0 {
		return nil
	}
	if _, err := s.db.Exec(`DELETE FROM find_query_reservations WHERE id = ?`, id); err != nil {
		return fmt.Errorf("store: release find reservation: %w", err)
	}
	return nil
}

// AppendFindQuery stores one final query and its candidates in the transaction
// that releases the reservation. CandidateCount is derived from candidates, so
// a caller cannot store a truncated list while claiming a larger count.
func (s *Store) AppendFindQuery(query FindQuery, candidates []FindCandidate, reservationID int64) (FindQuery, error) {
	query.CandidateCount = len(candidates)
	if err := validateFindQuery(query, candidates); err != nil {
		return FindQuery{}, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return FindQuery{}, fmt.Errorf("store: begin find append: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.Exec(`INSERT INTO find_queries (
			day, created_at, intent_json, skill_id, tactic_id, exam_section, learner_level,
			query, provider, endpoint_host, status, error_code, error_message,
			http_status, candidate_count, max_candidates, request_sha256, request_bytes, latency_ms
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		query.Day, query.CreatedAt.UTC().Format(time.RFC3339Nano), query.IntentJSON,
		query.SkillID, query.TacticID, query.ExamSection, query.LearnerLevel,
		query.Query, query.Provider, query.EndpointHost, query.Status, query.ErrorCode,
		query.ErrorMessage, query.HTTPStatus, query.CandidateCount, query.MaxCandidates,
		query.RequestSHA256, query.RequestBytes, query.LatencyMS)
	if err != nil {
		return FindQuery{}, fmt.Errorf("store: append find query: %w", err)
	}
	queryID, err := res.LastInsertId()
	if err != nil {
		return FindQuery{}, fmt.Errorf("store: read find query id: %w", err)
	}
	query.ID = queryID

	for index := range candidates {
		candidate := &candidates[index]
		candidate.QueryID = queryID
		res, err := tx.Exec(`INSERT INTO find_candidates (
				query_id, provider, query, retrieved_at, title, url, classification,
				license, reason, content, content_sha256, displayable
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			candidate.QueryID, candidate.Provider, candidate.Query,
			candidate.RetrievedAt.UTC().Format(time.RFC3339Nano), candidate.Title,
			candidate.URL, candidate.Classification, candidate.License, candidate.Reason,
			candidate.Content, candidate.ContentSHA256, boolInt(candidate.Displayable))
		if err != nil {
			return FindQuery{}, fmt.Errorf("store: append find candidate %d: %w", index+1, err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			return FindQuery{}, fmt.Errorf("store: read find candidate id: %w", err)
		}
		candidate.ID = id
	}

	if reservationID > 0 {
		if _, err := tx.Exec(`DELETE FROM find_query_reservations WHERE id = ?`, reservationID); err != nil {
			return FindQuery{}, fmt.Errorf("store: release find reservation: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return FindQuery{}, fmt.Errorf("store: commit find append: %w", err)
	}
	return query, nil
}

// FindQueries returns the most recent stored queries, newest first.
func (s *Store) FindQueries(limit int) ([]FindQuery, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	rows, err := s.db.Query(findQuerySelect+` ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list find queries: %w", err)
	}
	defer rows.Close()
	var out []FindQuery
	for rows.Next() {
		query, err := scanFindQuery(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan find query: %w", err)
		}
		out = append(out, query)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate find queries: %w", err)
	}
	return out, nil
}

// FindQuery returns one stored query and its candidates.
func (s *Store) FindQuery(id int64) (FindQuery, []FindCandidate, bool, error) {
	query, err := scanFindQuery(s.db.QueryRow(findQuerySelect+` WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return FindQuery{}, nil, false, nil
	}
	if err != nil {
		return FindQuery{}, nil, false, fmt.Errorf("store: read find query: %w", err)
	}
	rows, err := s.db.Query(`SELECT
			id, query_id, provider, query, retrieved_at, title, url, classification,
			license, reason, content, content_sha256, displayable
		FROM find_candidates WHERE query_id = ? ORDER BY id`, id)
	if err != nil {
		return FindQuery{}, nil, false, fmt.Errorf("store: list find candidates: %w", err)
	}
	defer rows.Close()
	var candidates []FindCandidate
	for rows.Next() {
		candidate, err := scanFindCandidate(rows)
		if err != nil {
			return FindQuery{}, nil, false, fmt.Errorf("store: scan find candidate: %w", err)
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return FindQuery{}, nil, false, fmt.Errorf("store: iterate find candidates: %w", err)
	}
	return query, candidates, true, nil
}

const findQuerySelect = `SELECT
		id, day, created_at, intent_json, skill_id, tactic_id, exam_section, learner_level,
		query, provider, endpoint_host, status, error_code, error_message, http_status,
		candidate_count, max_candidates, request_sha256, request_bytes, latency_ms
	FROM find_queries`

func scanFindQuery(row rowScanner) (FindQuery, error) {
	var query FindQuery
	var created string
	err := row.Scan(
		&query.ID, &query.Day, &created, &query.IntentJSON, &query.SkillID, &query.TacticID,
		&query.ExamSection, &query.LearnerLevel, &query.Query, &query.Provider,
		&query.EndpointHost, &query.Status, &query.ErrorCode, &query.ErrorMessage,
		&query.HTTPStatus, &query.CandidateCount, &query.MaxCandidates,
		&query.RequestSHA256, &query.RequestBytes, &query.LatencyMS,
	)
	if err != nil {
		return FindQuery{}, err
	}
	query.CreatedAt, err = parseTime(created)
	if err != nil {
		return FindQuery{}, err
	}
	return query, nil
}

func scanFindCandidate(row rowScanner) (FindCandidate, error) {
	var candidate FindCandidate
	var retrieved string
	var displayable int
	err := row.Scan(
		&candidate.ID, &candidate.QueryID, &candidate.Provider, &candidate.Query,
		&retrieved, &candidate.Title, &candidate.URL, &candidate.Classification,
		&candidate.License, &candidate.Reason, &candidate.Content,
		&candidate.ContentSHA256, &displayable,
	)
	if err != nil {
		return FindCandidate{}, err
	}
	candidate.RetrievedAt, err = parseTime(retrieved)
	if err != nil {
		return FindCandidate{}, err
	}
	candidate.Displayable = displayable != 0
	return candidate, nil
}

var findStatuses = map[string]bool{
	"ok": true, "empty": true, "all_link_only": true, "candidate_limit_exceeded": true,
	"rate_limited": true, "auth_failed": true, "provider_error": true,
	"unreachable": true, "invalid_response": true, "timeout": true,
}

func validateFindQuery(query FindQuery, candidates []FindCandidate) error {
	var problems []string
	add := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}
	if err := validateFindDay(query.Day); err != nil {
		add("%v", err)
	}
	if query.CreatedAt.IsZero() {
		add("created_at is required")
	}
	if strings.TrimSpace(query.IntentJSON) == "" {
		add("intent_json is required")
	}
	if query.Query == "" || utf8.RuneCountInString(query.Query) > 300 {
		add("query must be 1..300 characters")
	}
	if !findStatuses[query.Status] {
		add("status %q is invalid", query.Status)
	} else if query.Status != "ok" && query.Status != "empty" && query.Status != "all_link_only" && query.ErrorCode == "" {
		add("status %q requires an error code", query.Status)
	}
	if query.MaxCandidates < 1 || query.MaxCandidates > 100 {
		add("max_candidates %d is outside 1..100", query.MaxCandidates)
	}
	if query.RequestSHA256 != "" && (len(query.RequestSHA256) != 64 || !isLowerHex(query.RequestSHA256)) {
		add("request_sha256 is not a lowercase SHA-256")
	}
	switch query.Status {
	case "ok":
		if len(candidates) == 0 {
			add("status ok requires at least one candidate")
		}
	case "empty", "candidate_limit_exceeded":
		if len(candidates) != 0 {
			add("status %s must store no candidate", query.Status)
		}
	case "all_link_only":
		if len(candidates) == 0 {
			add("status all_link_only requires candidates")
		}
		for _, candidate := range candidates {
			if candidate.Classification != "link-only" {
				add("status all_link_only contains %s", candidate.Classification)
			}
		}
	default:
		if len(candidates) != 0 {
			add("failed query %s must store no candidate", query.Status)
		}
	}
	for index, candidate := range candidates {
		if err := validateFindCandidate(candidate); err != nil {
			add("candidate %d: %v", index+1, err)
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("store: invalid find query: %s", strings.Join(problems, "; "))
	}
	return nil
}

func validateFindCandidate(candidate FindCandidate) error {
	var problems []string
	add := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}
	if candidate.Provider == "" || utf8.RuneCountInString(candidate.Provider) > 200 {
		add("provider must be 1..200 characters")
	}
	if candidate.Query == "" || utf8.RuneCountInString(candidate.Query) > 300 {
		add("query must be 1..300 characters")
	}
	if candidate.RetrievedAt.IsZero() {
		add("retrieved_at is required")
	}
	if candidate.Title == "" || utf8.RuneCountInString(candidate.Title) > 300 {
		add("title must be 1..300 characters")
	}
	if candidate.URL == "" || utf8.RuneCountInString(candidate.URL) > 2048 {
		add("url must be 1..2048 characters")
	}
	if utf8.RuneCountInString(candidate.License) > 100 {
		add("license is longer than 100 characters")
	}
	if utf8.RuneCountInString(candidate.Reason) > 100 {
		add("reason is longer than 100 characters")
	}
	if len(candidate.ContentSHA256) != 64 || !isLowerHex(candidate.ContentSHA256) {
		add("content_sha256 is not a lowercase SHA-256")
	}
	switch candidate.Classification {
	case "local-usable":
		if candidate.Content == "" || !candidate.Displayable {
			add("local-usable candidate must carry displayable text")
		}
	case "link-only", "generated-unverified":
		if candidate.Content != "" || candidate.Displayable {
			add("%s candidate must not carry text", candidate.Classification)
		}
	default:
		add("classification %q is invalid", candidate.Classification)
	}
	if utf8.RuneCountInString(candidate.Content) > 20000 {
		add("content is longer than 20000 characters")
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

func validateFindDay(day string) error {
	if len(day) != 10 {
		return errors.New("store: find day must be YYYY-MM-DD")
	}
	if _, err := time.Parse("2006-01-02", day); err != nil {
		return fmt.Errorf("store: find day %q: %w", day, err)
	}
	return nil
}

func isLowerHex(value string) bool {
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}
