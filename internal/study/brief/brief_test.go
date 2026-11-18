package brief

import (
	"path/filepath"
	"testing"

	"github.com/wxxb789/passquest/internal/store"
)

func TestAgentLatestTokenFailsClosedOnAReadError(t *testing.T) {
	t.Parallel()
	db := filepath.Join(t.TempDir(), "passquest.sqlite")
	st, err := store.Open(db)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}
	if _, err := agentLatestToken(st, "run-20261110T100000Z-aaaaaaaa"); err == nil {
		t.Fatal("a closed store returned no error")
	}
}
