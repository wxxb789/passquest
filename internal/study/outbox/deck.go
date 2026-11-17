package outbox

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/wxxb789/passquest/internal/anki"
	"github.com/wxxb789/passquest/internal/store"
)

// deckRead is the deck verdict for one written note: Matched is true only
// when the endpoint reported every card of the note and named them all in the
// expected deck. Anything else leaves the claim unconfirmed, and Detail says
// what could not be read or which deck the card reached instead.
type deckRead struct {
	Deck    string
	Detail  string
	Matched bool
}

// readNoteDecks decides one note's deck claim from its cardsInfo rows. asked
// lists the cards this push knows belong to the note; a row about another card
// is not part of this verdict.
func readNoteDecks(cards []anki.CardInfo, asked []int64, expected string) deckRead {
	answered := make(map[int64]bool, len(cards))
	var unexpected []string
	for _, card := range cards {
		answered[card.CardID] = true
		deck := strings.TrimSpace(card.DeckName)
		if !usableDeckName(deck) {
			return deckRead{Detail: fmt.Sprintf("Anki reported no usable deck for card %d", card.CardID)}
		}
		if deck != expected && !containsString(unexpected, deck) {
			unexpected = append(unexpected, deck)
		}
	}
	for _, id := range asked {
		if !answered[id] {
			return deckRead{Detail: fmt.Sprintf("Anki reported no card %d for this note", id)}
		}
	}
	if len(asked) == 0 {
		return deckRead{Detail: "Anki reported no card for this note"}
	}
	if len(unexpected) == 0 {
		return deckRead{Matched: true}
	}
	claim := strings.Join(unexpected, ", ")
	if !usableDeckName(claim) {
		return deckRead{Detail: fmt.Sprintf(
			"Anki reported %d decks for this note; the combined name exceeds %d characters",
			len(unexpected), anki.MaxDeckNameChars)}
	}
	quoted := make([]string, 0, len(unexpected))
	for _, deck := range unexpected {
		quoted = append(quoted, fmt.Sprintf("%q", deck))
	}
	return deckRead{
		Deck:   claim,
		Detail: fmt.Sprintf("card landed in %s, not %q", strings.Join(quoted, ", "), expected),
	}
}

// classifyWriteResult applies one Anki write result to its work item. It
// returns true when an update found the note missing and the caller should
// retry that item once with addNote.
func classifyWriteResult(item *pushWork, result anki.ActionResult, action string, readded bool) bool {
	if result.State == "unreachable" {
		item.State = store.AnkiStateQueued
		item.Outcome = "unreachable"
		item.Detail = result.Error
		return false
	}
	if result.State != "ok" {
		message := strings.TrimSpace(result.Error)
		if message == "" {
			message = "AnkiConnect returned an error"
		}
		switch {
		case !readded && action == "update" && anki.IsNotFoundError(message):
			item.Detail = "note was missing; re-adding"
			return true
		case readded && anki.IsDuplicateError(message):
			item.State = store.AnkiStateConflict
			item.Outcome = "conflict"
			item.Code = "note_id_unknown"
			item.Detail = "Anki already holds a note with this front and reports no note id; remove that card in Anki or align the local content, then push again"
		case anki.IsDuplicateError(message) && item.Note.AnkiPushedRevision == 0 && item.Entry.Revision == 1:
			item.State = store.AnkiStateAlreadyExists
			item.Outcome = "duplicate"
			item.Detail = "already present in Anki"
			item.Drain = true
			item.AnkiNoteID = 0
		case anki.IsDuplicateError(message):
			item.State = store.AnkiStateConflict
			item.Outcome = "conflict"
			item.Code = "note_id_unknown"
			item.Detail = "Anki already holds a note with this front and reports no note id; remove that card in Anki or align the local content, then push again"
		default:
			item.State = store.AnkiStateFailed
			item.Outcome = "failed"
			item.Detail = message
		}
		return false
	}
	if action == "update" {
		item.State = store.AnkiStatePushed
		item.Outcome = "ok"
		item.AnkiNoteID = item.Note.AnkiNoteID
		item.Drain = true
		return false
	}
	id, err := anki.DecodeInt(result.Result)
	if err != nil {
		item.State = store.AnkiStateFailed
		item.Outcome = "failed"
		item.Detail = err.Error()
		return false
	}
	item.State = store.AnkiStatePushed
	item.Outcome = "ok"
	if readded {
		item.Detail = "note was missing in Anki; re-added"
	}
	item.AnkiNoteID = id
	item.Drain = true
	return false
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func appendUnique(values []string, value string) []string {
	if !containsString(values, value) {
		values = append(values, value)
	}
	return values
}

// usableDeckName reports whether a deck name from the endpoint can be the deck
// part of the claim: non-empty, bounded and printable, so a name the learner
// cannot read in Anki is never reported as where the card went.
func usableDeckName(deck string) bool {
	if deck == "" || utf8.RuneCountInString(deck) > anki.MaxDeckNameChars {
		return false
	}
	for _, char := range deck {
		if char < 0x20 || char == 0x7f {
			return false
		}
	}
	return true
}
