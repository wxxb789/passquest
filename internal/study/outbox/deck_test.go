package outbox

import (
	"fmt"
	"strings"
	"testing"

	"github.com/wxxb789/passquest/internal/anki"
)

func TestReadNoteDecks(t *testing.T) {
	t.Parallel()
	// 200 three-byte characters: over the bound in bytes, exactly at it in
	// characters, which is the unit the schema and the docs state.
	multibyteDeck := strings.Repeat("日", anki.MaxDeckNameChars)
	cases := []struct {
		name    string
		cards   []anki.CardInfo
		asked   []int64
		matched bool
		deck    string
		detail  string
	}{
		{
			name: "every card is in the named deck",
			cards: []anki.CardInfo{
				{CardID: 5, NoteID: 101, DeckName: "Pass Quest"},
				{CardID: 6, NoteID: 101, DeckName: "Pass Quest"},
			},
			asked: []int64{5, 6}, matched: true,
		},
		{
			name:   "a card reported in another deck",
			cards:  []anki.CardInfo{{CardID: 5, NoteID: 101, DeckName: "Default"}},
			asked:  []int64{5},
			deck:   "Default",
			detail: `card landed in "Default", not "Pass Quest"`,
		},
		{
			name: "cards reported in two other decks",
			cards: []anki.CardInfo{
				{CardID: 5, NoteID: 101, DeckName: "Default"},
				{CardID: 6, NoteID: 101, DeckName: "Other"},
				{CardID: 7, NoteID: 101, DeckName: "Default"},
			},
			asked:  []int64{5, 6, 7},
			deck:   "Default, Other",
			detail: `card landed in "Default", "Other", not "Pass Quest"`,
		},
		{
			name:   "the endpoint names no deck",
			cards:  []anki.CardInfo{{CardID: 5, NoteID: 101}},
			asked:  []int64{5},
			detail: "Anki reported no usable deck for card 5",
		},
		{
			name:   "the endpoint names a deck with a line break",
			cards:  []anki.CardInfo{{CardID: 5, NoteID: 101, DeckName: "Pass\nQuest"}},
			asked:  []int64{5},
			detail: "Anki reported no usable deck for card 5",
		},
		{
			name:   "the endpoint names an absurdly long deck",
			cards:  []anki.CardInfo{{CardID: 5, NoteID: 101, DeckName: strings.Repeat("d", anki.MaxDeckNameChars+1)}},
			asked:  []int64{5},
			detail: "Anki reported no usable deck for card 5",
		},
		{
			name:   "the endpoint names a multibyte deck at the character bound",
			cards:  []anki.CardInfo{{CardID: 5, NoteID: 101, DeckName: multibyteDeck}},
			asked:  []int64{5},
			deck:   multibyteDeck,
			detail: fmt.Sprintf("card landed in %q, not %q", multibyteDeck, "Pass Quest"),
		},
		{
			name: "the reported decks together exceed one recordable name",
			cards: []anki.CardInfo{
				{CardID: 5, NoteID: 101, DeckName: strings.Repeat("a", 120)},
				{CardID: 6, NoteID: 101, DeckName: strings.Repeat("b", 120)},
			},
			asked:  []int64{5, 6},
			detail: fmt.Sprintf("Anki reported 2 decks for this note; the combined name exceeds %d characters", anki.MaxDeckNameChars),
		},
		{
			name:   "a card of the note is missing from the answer",
			cards:  []anki.CardInfo{{CardID: 9, NoteID: 101, DeckName: "Pass Quest"}},
			asked:  []int64{5},
			detail: "Anki reported no card 5 for this note",
		},
		{
			name:   "the note has no cards at all",
			cards:  nil,
			asked:  nil,
			detail: "Anki reported no card for this note",
		},
	}
	for _, c := range cases {
		got := readNoteDecks(c.cards, c.asked, "Pass Quest")
		if got.Matched != c.matched || got.Deck != c.deck || got.Detail != c.detail {
			t.Errorf("%s: readNoteDecks = %+v, want matched=%t deck=%q detail=%q", c.name, got, c.matched, c.deck, c.detail)
		}
	}
}
