package exam

import (
	"reflect"
	"testing"
)

func TestParagraphsSplitAtBlankLines(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []string
	}{
		{"embedded double newline", "First one.\n\nSecond one.", []string{"First one.", "Second one."}},
		{"crlf and padding", "  A line\r\nwrapped\r\n\r\n\r\n B  \r\n", []string{"A line\nwrapped", "B"}},
		{"blank", " \n\n ", []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Paragraphs(tt.text); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Paragraphs(%q) = %q, want %q", tt.text, got, tt.want)
			}
		})
	}
}

func TestLocateExcerptReturnsRuneOffsets(t *testing.T) {
	paragraphs := []string{"Intro text.", "Café opens at nine. It closes late."}
	tests := []struct {
		name    string
		excerpt string
		want    Span
		found   bool
	}{
		{"second paragraph after multibyte rune", "It closes late.", Span{Paragraph: 1, Start: 20, End: 35}, true},
		{"first paragraph", "Intro", Span{Paragraph: 0, Start: 0, End: 5}, true},
		{"across a break", "text. Café", Span{}, false},
		{"blank", "  ", Span{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := LocateExcerpt(paragraphs, tt.excerpt)
			if found != tt.found || got != tt.want {
				t.Fatalf("LocateExcerpt(%q) = %+v, %v; want %+v, %v", tt.excerpt, got, found, tt.want, tt.found)
			}
		})
	}
	sample, err := Sample()
	if err != nil {
		t.Fatal(err)
	}
	paras := Paragraphs(sample.Passage.Text)
	for _, q := range sample.Questions {
		span, ok := LocateExcerpt(paras, q.Evidence)
		if !ok {
			t.Fatalf("sample evidence of %s not located", q.ID)
		}
		runes := []rune(paras[span.Paragraph])
		if string(runes[span.Start:span.End]) != q.Evidence {
			t.Fatalf("span of %s = %q, want %q", q.ID, string(runes[span.Start:span.End]), q.Evidence)
		}
	}
}
