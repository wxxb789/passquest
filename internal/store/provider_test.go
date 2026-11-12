package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestProviderBudgetIsCountedPerProvider(t *testing.T) {
	st := mustOpen(t, filepath.Join(t.TempDir(), "provider-budget.sqlite"))
	day := time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)
	at := day.Add(9 * time.Hour)

	alpha, used, err := st.ReserveTutorCall("alpha", day, at, 1)
	if err != nil || alpha == 0 || used != 1 {
		t.Fatalf("alpha reservation = %d, %d, %v", alpha, used, err)
	}
	if refused, used, err := st.ReserveTutorCall("alpha", day, at, 1); err != nil || refused != 0 || used != 1 {
		t.Fatalf("alpha second reservation = %d, %d, %v; want its own refusal", refused, used, err)
	}
	beta, used, err := st.ReserveTutorCall("beta", day, at, 1)
	if err != nil || beta == 0 || used != 1 {
		t.Fatalf("beta reservation = %d, %d, %v; alpha's limit blocked beta", beta, used, err)
	}
	counts, err := st.ProviderCallCountsSince(day)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int{}
	for _, item := range counts {
		got[item.Provider] = item.Calls
	}
	if got["alpha"] != 1 || got["beta"] != 1 {
		t.Fatalf("per-provider counts = %+v", got)
	}
}

func TestProviderVerificationLatestWins(t *testing.T) {
	st := mustOpen(t, filepath.Join(t.TempDir(), "verification.sqlite"))
	at := time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)
	if _, err := st.AppendProviderVerification(ProviderVerification{
		Provider: "alpha", Model: "alpha-small", Capability: "text.chat", Status: "failed",
		FailureType: "endpoint_unreachable", Action: "check the endpoint", CheckedAt: at,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AppendProviderVerification(ProviderVerification{
		Provider: "alpha", Model: "alpha-small", Capability: "text.chat", Status: "verified",
		CheckedAt: at.Add(time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	latest, found, err := st.LatestProviderVerification("alpha", "alpha-small", "text.chat")
	if err != nil || !found || latest.Status != "verified" {
		t.Fatalf("latest = %+v, %v, %v", latest, found, err)
	}
	all, err := st.ProviderVerifications()
	if err != nil || len(all) != 1 || all[0].Status != "verified" {
		t.Fatalf("list = %+v, %v", all, err)
	}
}

func TestProviderConsentOverridesOnlyItsOwnProvider(t *testing.T) {
	st := mustOpen(t, filepath.Join(t.TempDir(), "provider-consent.sqlite"))
	at := time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)
	if _, err := st.AppendTutorConsent(TutorConsentEvent{
		SourceID: "src-a", Granted: true, Reviewer: "owner", At: at,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AppendTutorConsent(TutorConsentEvent{
		SourceID: "src-a", Provider: "alpha", Granted: false, Reviewer: "owner", At: at.Add(time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	if event, ok, err := st.TutorConsent("src-a", "alpha"); err != nil || !ok || event.Granted {
		t.Fatalf("alpha consent = %+v, %v, %v; want its provider-specific denial", event, ok, err)
	}
	if event, ok, err := st.TutorConsent("src-a", "beta"); err != nil || !ok || !event.Granted {
		t.Fatalf("beta consent = %+v, %v, %v; want the global grant", event, ok, err)
	}
	if event, ok, err := st.TutorConsent("src-a", ""); err != nil || !ok || !event.Granted {
		t.Fatalf("legacy consent = %+v, %v, %v; want the global grant", event, ok, err)
	}
}

func TestProviderFailoverEventsAreAppendOnly(t *testing.T) {
	st := mustOpen(t, filepath.Join(t.TempDir(), "failover.sqlite"))
	at := time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)
	if _, err := st.AppendProviderFailover(ProviderFailover{
		At: at, FromProvider: "alpha", FromModel: "alpha-small",
		ToProvider: "beta", ToModel: "beta-small", Reason: "rate_limited", Applied: true,
		FromCallID: 1, ToCallID: 2,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AppendProviderFailover(ProviderFailover{
		At: at, FromProvider: "alpha", FromModel: "alpha-small",
		ToProvider: "beta", Reason: "rate_limited", Refusal: "consent_scope_differs",
	}); err != nil {
		t.Fatal(err)
	}
	events, err := st.ProviderFailovers()
	if err != nil || len(events) != 2 || !events[0].Applied || events[1].Refusal != "consent_scope_differs" {
		t.Fatalf("failover events = %+v, %v", events, err)
	}
	if _, err := st.db.Exec(`UPDATE provider_failover_events SET refusal = 'x' WHERE id = 1`); err == nil {
		t.Fatal("a failover event was updated")
	}
}

func TestMigrationFromVersionFourteenAddsProviderColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v14.sqlite")
	db := openSQLite(t, path)
	for _, migration := range migrations {
		if migration.version > 14 {
			break
		}
		for _, stmt := range migration.statements {
			if _, err := db.Exec(stmt); err != nil {
				t.Fatalf("create v14 schema at %d: %v", migration.version, err)
			}
		}
	}
	if _, err := db.Exec(`PRAGMA user_version = 14`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	var version int
	if err := st.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != SchemaVersion {
		t.Fatalf("user_version = %d, %v; want %d", version, err, SchemaVersion)
	}
	for _, column := range []struct{ table, name string }{
		{"tutor_config", "provider"},
		{"tutor_consent_events", "provider"},
		{"tutor_calls", "provider"},
		{"tutor_calls", "failover_from"},
		{"tutor_call_reservations", "provider"},
		{"note_refinements", "provider"},
		{"agent_runs", "provider"},
	} {
		var found bool
		rows, err := st.db.Query(`PRAGMA table_info(` + column.table + `)`)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var cid int
			var name, kind string
			var notNull, primary int
			var defaultValue any
			if err := rows.Scan(&cid, &name, &kind, &notNull, &defaultValue, &primary); err != nil {
				t.Fatal(err)
			}
			if name == column.name {
				found = true
			}
		}
		rows.Close()
		if !found {
			t.Fatalf("%s.%s was not added", column.table, column.name)
		}
	}
}
