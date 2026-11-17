package note

import (
	"crypto/sha256"
	"encoding/hex"
	"math"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func sha(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// TestModelVersionsArePinned: the current design is version 2 and every known
// version's exact bytes hash to the pinned values, so an edit to the design
// cannot ship without a new version and a frozen copy of the old one.
func TestModelVersionsArePinned(t *testing.T) {
	if ModelVersion != 2 || TemplateModel().Version != ModelVersion {
		t.Fatalf("model version = %d / %d", ModelVersion, TemplateModel().Version)
	}
	pinned := map[int]ModelHash{
		1: {
			CSS:   "28ec8543f2441a8fdd19b252342ba390cfe56416d508b1cc58d09f05d8186d33",
			Front: "ca10ec17b38342837005f213fc2c1b6ac9ac30b08bd688fae8b8b8a328bb1602",
			Back:  "839a9ebad5d4170f068652cf09cbca583e59d3348f50591775532901d314e386",
		},
		2: {
			CSS:   "2d9b3b204806e56f74dfe25603f809a09b8a1fcd16524c8956a5325ee00fc942",
			Front: "dabec57625188e89e048dab33b395300b97fda2af31204c45cdba12b895f1f0d",
			Back:  "29be13351ae0b754015550124a411dca256eb5feca9c8237ec40201e3f4803cc",
		},
	}
	if len(KnownModelHashes) != len(pinned) {
		t.Fatalf("known versions = %d, pinned = %d", len(KnownModelHashes), len(pinned))
	}
	for version, want := range pinned {
		css, front, back, ok := KnownModel(version)
		if !ok {
			t.Fatalf("version %d is not known", version)
		}
		if got := (ModelHash{CSS: sha(css), Front: sha(front), Back: sha(back)}); got != want || KnownModelHashes[version] != want {
			t.Fatalf("version %d hashes = %+v, known = %+v, want %+v", version, got, KnownModelHashes[version], want)
		}
	}
	if _, _, _, ok := KnownModel(3); ok {
		t.Fatal("an unreleased version is known")
	}
	if CSSSHA256() != pinned[ModelVersion].CSS {
		t.Fatalf("CSSSHA256 = %s", CSSSHA256())
	}
	model := TemplateModel()
	if model.CSS != ModelCSS || model.FrontTemplate != FrontTemplate || model.BackTemplate != BackTemplate {
		t.Fatal("TemplateModel is not the current design")
	}
}

func TestClassifyModel(t *testing.T) {
	v1CSS, v1Front, v1Back, _ := KnownModel(1)
	crlf := func(value string) string { return strings.ReplaceAll(value, "\n", "\r\n") + "\n" }
	cases := []struct {
		name             string
		css, front, back string
		want             ModelState
	}{
		{"current", ModelCSS, FrontTemplate, BackTemplate, ModelCurrent},
		{"current with CRLF", crlf(ModelCSS), crlf(FrontTemplate), crlf(BackTemplate), ModelCurrent},
		{"version 1", v1CSS, v1Front, v1Back, ModelUpgradable},
		{"version 1 with CRLF", crlf(v1CSS), crlf(v1Front), crlf(v1Back), ModelUpgradable},
		{"half updated", ModelCSS, v1Front, v1Back, ModelUpgradable},
		{"edited stylesheet", v1CSS + "\n.card { color: red; }", v1Front, v1Back, ModelCustomized},
		{"edited front", ModelCSS, FrontTemplate + "{{Tactic}}", BackTemplate, ModelCustomized},
		{"edited back", v1CSS, v1Front, strings.Replace(v1Back, "Tactic", "Tip", 1), ModelCustomized},
		{"empty", "", "", "", ModelCustomized},
	}
	for _, tc := range cases {
		if got := ClassifyModel(tc.css, tc.front, tc.back); got != tc.want {
			t.Errorf("%s: ClassifyModel = %d, want %d", tc.name, got, tc.want)
		}
	}
}

// TestModelHasNoRemoteAssetOrScript is the u12 forbidden-token list: no
// remote or embedded asset, no script, no handler and no clipping, in the
// stylesheet or either template.
func TestModelHasNoRemoteAssetOrScript(t *testing.T) {
	combined := strings.ToLower(ModelCSS + "\n" + FrontTemplate + "\n" + BackTemplate)
	for _, forbidden := range []string{
		"url(", "@import", "@font-face", "http", "//", "javascript:", "overflow: hidden", "overflow:hidden",
		"position: fixed", "position:fixed", "min-height", "100vh", "expression(", "behavior:",
	} {
		if strings.Contains(combined, forbidden) {
			t.Errorf("model contains %q", forbidden)
		}
	}
	for _, pattern := range []string{`<\s*/?\s*(script|link|img|iframe|object|embed|svg|style|base|meta)\b`, `\bon\w+\s*=`} {
		if match := regexp.MustCompile(pattern).FindString(combined); match != "" {
			t.Errorf("model contains %q", match)
		}
	}
	if !strings.Contains(ModelCSS, "overflow-wrap: anywhere") {
		t.Error("model CSS does not wrap long text")
	}
}

// systemFonts is the allowlist of installed or generic families the card may
// name; anything else would need a download Anki never makes.
var systemFonts = map[string]bool{
	"-apple-system": true, "BlinkMacSystemFont": true, "Segoe UI Variable Text": true, "Segoe UI": true,
	"Roboto": true, "Helvetica Neue": true, "Arial": true, "sans-serif": true,
	"Sitka Text": true, "Iowan Old Style": true, "Charter": true, "Georgia": true, "Noto Serif": true, "serif": true,
	"Cascadia Mono": true, "SF Mono": true, "Consolas": true, "Menlo": true, "Roboto Mono": true, "monospace": true,
}

func TestModelUsesSystemFontsOnly(t *testing.T) {
	declarations := regexp.MustCompile(`(?m)(font-family|--pq-read|--pq-mono)\s*:\s*([^;]+);`).FindAllStringSubmatch(ModelCSS, -1)
	if len(declarations) < 3 {
		t.Fatalf("font declarations = %d", len(declarations))
	}
	for _, declaration := range declarations {
		for _, family := range strings.Split(declaration[2], ",") {
			family = strings.Trim(strings.TrimSpace(family), `"'`)
			if strings.HasPrefix(family, "var(--pq-") {
				continue
			}
			if !systemFonts[family] {
				t.Errorf("%s names %q, which is not a system font", declaration[1], family)
			}
		}
	}
	if !strings.Contains(ModelCSS, "font-family: -apple-system, BlinkMacSystemFont") {
		t.Error("the card does not start with the system UI stack")
	}
	for _, size := range regexp.MustCompile(`font-size:\s*([0-9.]+)px`).FindAllStringSubmatch(ModelCSS, -1) {
		if value, _ := strconv.ParseFloat(size[1], 64); value < 12 {
			t.Errorf("font-size %spx is below 12 px", size[1])
		}
	}
}

// cssRule returns the declarations of the first rule whose selector list is
// exactly selector, ignoring whitespace.
func cssRule(t *testing.T, css, selector string) string {
	t.Helper()
	squash := func(value string) string { return strings.Join(strings.Fields(value), "") }
	css = regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(css, "")
	for _, rule := range regexp.MustCompile(`(?s)([^{}]+)\{([^{}]*)\}`).FindAllStringSubmatch(css, -1) {
		if squash(rule[1]) == squash(selector) {
			return rule[2]
		}
	}
	t.Fatalf("no rule %q", selector)
	return ""
}

func cssTokens(declarations string) map[string]string {
	tokens := map[string]string{}
	for _, match := range regexp.MustCompile(`(--pq-[a-z0-9]+)\s*:\s*(#[0-9a-fA-F]{6})\s*;`).FindAllStringSubmatch(declarations, -1) {
		tokens[match[1]] = match[2]
	}
	return tokens
}

func TestModelNightModeMatchesAnkisDOM(t *testing.T) {
	night := cssRule(t, ModelCSS, ".card.nightMode, .card.night_mode, .nightMode .card, .night_mode .card")
	if len(cssTokens(night)) < 6 {
		t.Fatalf("night rule tokens = %v", cssTokens(night))
	}
	if strings.Contains(ModelCSS, "prefers-color-scheme") {
		t.Error("the card follows the system scheme instead of Anki's night-mode switch")
	}
	// Every colour comes from a token, so the night rule recolours everything.
	for _, match := range regexp.MustCompile(`(?m)^\s*(color|background|background-color)\s*:\s*([^;]+);`).FindAllStringSubmatch(ModelCSS, -1) {
		if !strings.HasPrefix(strings.TrimSpace(match[2]), "var(--pq-") {
			t.Errorf("%s: %s is not a token", match[1], match[2])
		}
	}
	for _, match := range regexp.MustCompile(`(?m)^\s*border(-top|-left)?\s*:\s*([^;]+);`).FindAllStringSubmatch(ModelCSS, -1) {
		if strings.Contains(match[2], "#") {
			t.Errorf("border %s is a literal colour", match[2])
		}
	}
}

func relativeLuminance(hexColour string) float64 {
	var channels [3]float64
	for i := range channels {
		value, _ := strconv.ParseUint(hexColour[1+2*i:3+2*i], 16, 8)
		c := float64(value) / 255
		if c <= 0.03928 {
			channels[i] = c / 12.92
		} else {
			channels[i] = math.Pow((c+0.055)/1.055, 2.4)
		}
	}
	return 0.2126*channels[0] + 0.7152*channels[1] + 0.0722*channels[2]
}

func contrast(a, b string) float64 {
	la, lb := relativeLuminance(a), relativeLuminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// TestModelTextContrast: every token used as a text colour reaches WCAG AA
// (4.5:1) on both the sheet and the canvas, in light and in night mode.
func TestModelTextContrast(t *testing.T) {
	if got := contrast("#777777", "#ffffff"); math.Abs(got-4.48) > 0.01 {
		t.Fatalf("contrast formula = %.3f", got)
	}
	light := cssTokens(cssRule(t, ModelCSS, ".card"))
	night := cssTokens(cssRule(t, ModelCSS, ".card.nightMode, .card.night_mode, .nightMode .card, .night_mode .card"))
	textTokens := map[string]bool{}
	for _, match := range regexp.MustCompile(`(?m)^\s*color\s*:\s*var\((--pq-[a-z0-9]+)\)`).FindAllStringSubmatch(ModelCSS, -1) {
		textTokens[match[1]] = true
	}
	if len(textTokens) < 4 {
		t.Fatalf("text tokens = %v", textTokens)
	}
	for theme, tokens := range map[string]map[string]string{"light": light, "night": night} {
		for token := range textTokens {
			for _, surface := range []string{"--pq-bg", "--pq-canvas"} {
				fg, bg := tokens[token], tokens[surface]
				if fg == "" || bg == "" {
					t.Fatalf("%s: %s or %s is undefined", theme, token, surface)
				}
				if ratio := contrast(fg, bg); ratio < 4.5 {
					t.Errorf("%s: %s %s on %s %s = %.2f:1", theme, token, fg, surface, bg, ratio)
				}
			}
		}
	}
}

func TestModelIsReadableAtPhoneWidth(t *testing.T) {
	if !strings.Contains(ModelCSS, "@media (max-width: 480px)") {
		t.Fatal("no phone-width rule")
	}
	for _, match := range regexp.MustCompile(`(?m)^\s*(min-width|width)\s*:\s*([0-9.]+)px`).FindAllStringSubmatch(ModelCSS, -1) {
		if value, _ := strconv.ParseFloat(match[2], 64); value > 320 {
			t.Errorf("%s: %spx is wider than a 320 px phone", match[1], match[2])
		}
	}
	if regexp.MustCompile(`(?m)^\s*white-space\s*:\s*nowrap`).MatchString(ModelCSS) {
		t.Error("the card forbids wrapping")
	}
}

func TestRenderSides(t *testing.T) {
	fields := HTML("closes at 4 p.m.", "To shut.\n\nExample: The library closes at 4 p.m.", "NOT / EXCEPT", "src · p001 · q2")
	question, answer := RenderSides(fields)
	if strings.Contains(question, `id="answer"`) || !strings.Contains(answer, `<hr id="answer">`) {
		t.Fatalf("answer marker: question %q, answer %q", question, answer)
	}
	if !strings.Contains(answer, question) {
		t.Fatal("the answer does not contain the question side")
	}
	if strings.Contains(question, "NOT / EXCEPT") || strings.Contains(question, "pq-tactic") {
		t.Fatalf("the question names the tactic (Q-0012-04): %s", question)
	}
	for _, want := range []string{`<span class="pq-label">Tactic</span> NOT / EXCEPT`, fields.Back, `<span class="pq-source">src · p001 · q2</span>`} {
		if !strings.Contains(answer, want) {
			t.Fatalf("answer misses %q: %s", want, answer)
		}
	}
	if strings.Contains(question+answer, "{{") {
		t.Fatalf("unresolved tag: %s", answer)
	}
	if RenderCard(fields) != answer {
		t.Fatal("RenderCard is not the answer side")
	}
	_, untagged := RenderSides(HTML("front", "meaning", "", "source"))
	if strings.Contains(untagged, "pq-tactic") || strings.Contains(untagged, "Tactic") {
		t.Fatalf("an empty tactic left its section: %s", untagged)
	}
}

// TestRenderTemplateIsSinglePass: a value is never expanded again, whatever
// the order the fields are visited in, exactly as Anki renders it.
func TestRenderTemplateIsSinglePass(t *testing.T) {
	fields := Fields{Front: "{{Back}}", Back: "{{Front}} {{FrontSide}}", Tactic: "{{/Tactic}}{{Source}}", Source: "{{#Front}}x{{/Front}}"}
	for i := 0; i < 20; i++ {
		question, answer := RenderSides(fields)
		if question != `<div class="pq-sheet pq-question">`+"\n"+`<div class="pq-front" lang="en">{{Back}}</div>`+"\n</div>" {
			t.Fatalf("question = %q", question)
		}
		for _, want := range []string{
			`<div class="pq-back" lang="en">{{Front}} {{FrontSide}}</div>`,
			`<span class="pq-label">Tactic</span> {{/Tactic}}{{Source}}</span>`,
			`<span class="pq-source">{{#Front}}x{{/Front}}</span>`,
		} {
			if !strings.Contains(answer, want) {
				t.Fatalf("answer misses %q: %s", want, answer)
			}
		}
	}
	values := map[string]string{"A": "a", "B": " "}
	for template, want := range map[string]string{
		"{{A}}-{{B}}-{{C}}":           "a- -{{C}}",
		"{{#A}}[{{A}}]{{/A}}":         "[a]",
		"{{#B}}[{{B}}]{{/B}}":         "",
		"{{^B}}empty{{/B}}":           "empty",
		"{{^A}}empty{{/A}}":           "",
		"{{#A}}unclosed":              "{{#A}}unclosed",
		"open {{A":                    "open {{A",
		"{{#A}}{{#B}}x{{/B}}y{{/A}}z": "yz",
	} {
		if got := renderTemplate(template, values); got != want {
			t.Errorf("renderTemplate(%q) = %q, want %q", template, got, want)
		}
	}
}

func TestCardDocumentMirrorsAnki(t *testing.T) {
	body := `<div class="pq-front">x</div>`
	light := CardDocument(body, ModelCSS, false)
	night := CardDocument(body, ModelCSS, true)
	if !strings.Contains(light, `<body class="card card1"><div id="qa">`+body+`</div></body>`) {
		t.Fatalf("light document = %s", light)
	}
	if !strings.Contains(night, `<body class="card card1 nightMode night_mode"><div id="qa">`+body+`</div></body>`) {
		t.Fatalf("night document = %s", night)
	}
	if !strings.Contains(light, "<style>"+ModelCSS+"</style>") || !strings.HasPrefix(light, "<!doctype html>") {
		t.Fatal("the document does not carry the model stylesheet")
	}
	if strings.Contains(strings.ToLower(night), "<script") || strings.Contains(night, `<html class=`) {
		t.Fatalf("unexpected markup: %s", night)
	}
}
