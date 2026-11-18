package outbox

import (
	"fmt"
	"strings"

	"github.com/wxxb789/passquest/internal/store"
)

// ConsentRefusal names restricted sources without a global grant. Export and
// push share this policy; verb states what was refused. A provider-scoped
// tutor grant authorizes inference, not this export destination.
func ConsentRefusal(st *store.Store, notes []store.Note, verb string) (string, bool) {
	var denied []string
	for _, n := range notes {
		if n.Restricted && n.SourceID != "" && !consentGranted(st, n.SourceID) {
			denied = appendUnique(denied, n.SourceID)
		}
	}
	if len(denied) == 0 {
		return "", false
	}
	return fmt.Sprintf(
		"restricted note text from %s cannot be %s because consent is not recorded; run passquest tutor consent SOURCE --reviewer NAME",
		strings.Join(denied, ", "), verb), true
}

// consentGranted reports whether the latest recorded global consent for a
// source is a grant. Note text and its Anki export are a different destination
// from provider inference, so a provider-scoped tutor grant never authorizes
// them; only the global decision does. A lookup error is treated as no consent.
func consentGranted(st *store.Store, sourceID string) bool {
	event, ok, err := st.TutorConsent(sourceID, "")
	return err == nil && ok && event.Granted
}
