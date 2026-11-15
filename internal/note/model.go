package note

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// The Anki deck and note model. Q-0007-02 chose these names. They, the field
// names and order, the template name (anki.TemplateName) and the field HTML
// produced by HTML are what an existing deck is matched by, so a new card
// design changes only the stylesheet and the templates (Q-0012-02).
const (
	Deck      = "Pass Quest"
	ModelName = "PassQuest Note"
	Tag       = "passquest"
)

// ModelFields is the field order sent to createModel and used by export.
var ModelFields = []string{"Front", "Back", "Tactic", "Source"}

// ModelVersion is the version of ModelCSS, FrontTemplate and BackTemplate.
// Every released version's exact bytes stay pinned in KnownModelHashes so a
// deck that still carries one can be upgraded in place.
const ModelVersion = 2

// ModelCSS is the C-Stage card (goal 0012): system fonts only, no external
// asset, no fixed height and no overflow clipping, so long imported text wraps
// at phone width. Colours are tokens on .card; Anki's night mode puts
// nightMode/night_mode on the same element as .card, so the compound
// selectors carry the night tokens and the descendant pair covers a client that
// marks an ancestor instead.
const ModelCSS = `/* passquest-note-model 2 */
.card {
  --pq-canvas: #efede7;
  --pq-bg: #fbfaf7;
  --pq-ink: #1d2230;
  --pq-ink2: #4a5163;
  --pq-ink3: #5d6375;
  --pq-line: #e2e1dc;
  --pq-acc: #3a43c4;
  --pq-read: "Sitka Text", "Iowan Old Style", Charter, Georgia, "Noto Serif", serif;
  --pq-mono: "Cascadia Mono", "SF Mono", Consolas, Menlo, "Roboto Mono", monospace;
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI Variable Text", "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
  font-size: 17px;
  line-height: 1.55;
  color: var(--pq-ink);
  background: var(--pq-canvas);
  text-align: left;
  margin: 0;
  padding: 16px;
  overflow-wrap: anywhere;
  word-break: break-word;
}
.card.nightMode,
.card.night_mode,
.nightMode .card,
.night_mode .card {
  --pq-canvas: #0f1320;
  --pq-bg: #161b2b;
  --pq-ink: #eceef5;
  --pq-ink2: #c2c7d6;
  --pq-ink3: #9aa1b5;
  --pq-line: #2a3042;
  --pq-acc: #a9b0ff;
}
.pq-sheet {
  max-width: 34rem;
  margin: 0 auto;
  padding: 24px 28px 20px;
  color: var(--pq-ink);
  background: var(--pq-bg);
  border: 1px solid var(--pq-line);
  border-radius: 18px;
}
.pq-answer .pq-sheet {
  display: contents;
}
.pq-front {
  margin: 4px 0;
  font-family: var(--pq-read);
  font-size: 1.65em;
  line-height: 1.3;
  font-weight: 400;
  color: var(--pq-ink);
}
.pq-answer .pq-front {
  margin: 0;
  font-size: 1.15em;
}
#answer {
  height: 0;
  margin: 14px 0;
  border: 0;
  border-top: 1px solid var(--pq-line);
}
.pq-meaning {
  margin: 0 0 12px 0;
  font-size: 0.92em;
  line-height: 1.62;
  color: var(--pq-ink);
}
.pq-example {
  margin: 0 0 12px 0;
  padding-left: 14px;
  border-left: 2px solid var(--pq-line);
  font-family: var(--pq-read);
  font-style: italic;
  font-size: 0.9em;
  line-height: 1.6;
  color: var(--pq-ink2);
}
.pq-foot {
  display: flex;
  flex-wrap: wrap;
  gap: 6px 12px;
  align-items: center;
  justify-content: space-between;
  margin-top: 16px;
}
.pq-tactic {
  padding: 3px 8px;
  border: 1px solid var(--pq-line);
  border-left: 2px solid var(--pq-acc);
  border-radius: 6px;
  font-size: 13px;
  font-weight: 600;
  color: var(--pq-ink2);
}
.pq-label {
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--pq-acc);
}
.pq-source {
  font-family: var(--pq-mono);
  font-size: 12px;
  color: var(--pq-ink3);
}
@media (max-width: 480px) {
  .card {
    font-size: 16px;
    padding: 0;
  }
  .pq-sheet {
    padding: 16px 16px 14px;
    border: 0;
    border-radius: 0;
  }
  .pq-front {
    font-size: 1.4em;
  }
}`

// FrontTemplate renders the recognition prompt. The tactic is not on the
// front, so the prompt does not name the category of its answer (Q-0012-04).
const FrontTemplate = `<div class="pq-sheet pq-question">
<div class="pq-front" lang="en">{{Front}}</div>
</div>`

// BackTemplate reveals the meaning, example, tactic and source after the
// learner answers the Anki card. The question sheet it embeds through
// {{FrontSide}} dissolves into the answer sheet.
const BackTemplate = `<div class="pq-sheet pq-answer">{{FrontSide}}
<hr id="answer">
<div class="pq-back" lang="en">{{Back}}</div>
<div class="pq-foot">{{#Tactic}}<span class="pq-tactic"><span class="pq-label">Tactic</span> {{Tactic}}</span>{{/Tactic}}<span class="pq-source">{{Source}}</span></div>
</div>`

// The version 1 model (goal 0007), frozen byte for byte: a deck created by an
// earlier build carries exactly these bytes, which is how it is recognised as
// safe to upgrade.
const (
	modelCSSV1 = `.card {
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
  font-size: 17px;
  line-height: 1.5;
  color: #1f2328;
  background: #ffffff;
  text-align: left;
  padding: 12px 14px;
  max-width: 42rem;
  margin: 0 auto;
  overflow-wrap: anywhere;
  word-break: break-word;
}
.pq-front {
  font-size: 1.25em;
  font-weight: 600;
}
#answer {
  border: 0;
  border-top: 1px solid #d0d7de;
  margin: 14px 0;
}
.pq-meaning {
  margin: 0 0 10px 0;
}
.pq-example {
  margin: 0 0 10px 0;
}
.pq-tactic {
  margin-top: 12px;
  font-size: 0.9em;
  color: #57606a;
}
.pq-label {
  font-weight: 600;
}
.pq-source {
  margin-top: 8px;
  font-size: 0.8em;
  color: #6e7781;
}
.nightMode .card {
  color: #e6edf3;
  background: #0d1117;
}
.nightMode #answer {
  border-top-color: #30363d;
}
.nightMode .pq-tactic,
.nightMode .pq-source {
  color: #8b949e;
}
@media (max-width: 480px) {
  .card {
    font-size: 16px;
    padding: 10px 12px;
  }
}`
	frontTemplateV1 = `<div class="pq-front">{{Front}}</div>`
	backTemplateV1  = `{{FrontSide}}
<hr id="answer">
<div class="pq-back">{{Back}}</div>
{{#Tactic}}<div class="pq-tactic"><span class="pq-label">Tactic</span> {{Tactic}}</div>{{/Tactic}}
<div class="pq-source">{{Source}}</div>`
)

// KnownModel returns the exact stylesheet and card template of one released
// model version.
func KnownModel(version int) (css, front, back string, ok bool) {
	switch version {
	case 1:
		return modelCSSV1, frontTemplateV1, backTemplateV1, true
	case ModelVersion:
		return ModelCSS, FrontTemplate, BackTemplate, true
	}
	return "", "", "", false
}

// ModelHash is the SHA-256 of each part of one model version.
type ModelHash struct {
	CSS   string
	Front string
	Back  string
}

// KnownModelHashes pins every released model version. A deck whose stylesheet
// and template each match one of these is Pass Quest's own; anything else was
// changed by the learner in Anki.
var KnownModelHashes = map[int]ModelHash{
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

// ModelState classifies the model an Anki collection already holds.
type ModelState int

const (
	// ModelCurrent is this build's model.
	ModelCurrent ModelState = iota
	// ModelUpgradable is an earlier Pass Quest version the learner did not
	// change; it is replaced in place.
	ModelUpgradable
	// ModelCustomized was changed in Anki and is never overwritten.
	ModelCustomized
)

// ClassifyModel compares a stylesheet and card template read from Anki with
// the known versions. Line endings and surrounding whitespace are normalised
// first, because a client may store them differently; any other difference is
// the learner's customisation. Each part is matched on its own, so a deck left
// half-upgraded by an interrupted update is still upgradable.
func ClassifyModel(css, front, back string) ModelState {
	got := ModelHash{CSS: modelPartHash(css), Front: modelPartHash(front), Back: modelPartHash(back)}
	if got == KnownModelHashes[ModelVersion] {
		return ModelCurrent
	}
	cssKnown, frontKnown, backKnown := false, false, false
	for _, known := range KnownModelHashes {
		cssKnown = cssKnown || known.CSS == got.CSS
		frontKnown = frontKnown || known.Front == got.Front
		backKnown = backKnown || known.Back == got.Back
	}
	if cssKnown && frontKnown && backKnown {
		return ModelUpgradable
	}
	return ModelCustomized
}

func modelPartHash(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:])
}

// ModelSpec is the createModel document owned by this package.
type ModelSpec struct {
	Deck          string
	Name          string
	Fields        []string
	CSS           string
	FrontTemplate string
	BackTemplate  string
	Version       int
}

// TemplateModel returns the canonical model specification.
func TemplateModel() ModelSpec {
	return ModelSpec{
		Deck:          Deck,
		Name:          ModelName,
		Fields:        append([]string(nil), ModelFields...),
		CSS:           ModelCSS,
		FrontTemplate: FrontTemplate,
		BackTemplate:  BackTemplate,
		Version:       ModelVersion,
	}
}

// CSSSHA256 identifies the exact stylesheet of this build's model.
func CSSSHA256() string {
	sum := sha256.Sum256([]byte(ModelCSS))
	return hex.EncodeToString(sum[:])
}

// RenderSides substitutes already-rendered field HTML into the card templates
// the way Anki does: the question is the front template, the answer is the
// back template with the question in {{FrontSide}}. It is the single source of
// the desktop preview; Anki performs the real rendering.
func RenderSides(fields Fields) (question, answer string) {
	values := map[string]string{
		"Front":  fields.Front,
		"Back":   fields.Back,
		"Tactic": fields.Tactic,
		"Source": fields.Source,
	}
	question = renderTemplate(FrontTemplate, values)
	values["FrontSide"] = question
	return question, renderTemplate(BackTemplate, values)
}

// RenderCard is the answer side: the whole card after the learner answers.
func RenderCard(fields Fields) string {
	_, answer := RenderSides(fields)
	return answer
}

// CardDocument wraps one rendered side in the document Anki builds around a
// card: the model stylesheet, then a body carrying the card classes (plus the
// night-mode classes when night is true) around the #qa container.
func CardDocument(body, css string, night bool) string {
	classes := "card card1"
	if night {
		classes += " nightMode night_mode"
	}
	return `<!doctype html><html><head><meta charset="utf-8">` +
		`<meta name="viewport" content="width=device-width, initial-scale=1">` +
		`<style>` + css + `</style></head><body class="` + classes + `"><div id="qa">` +
		body + `</div></body></html>`
}

// renderTemplate expands one template in a single pass over the template text:
// {{Name}} becomes the field value, {{#Name}}…{{/Name}} keeps its content only
// when the field is not blank and {{^Name}}…{{/Name}} only when it is. A
// substituted value is never scanned again, so a field that itself contains
// "{{Back}}" is shown as written, as Anki shows it. An unknown tag is kept
// verbatim.
func renderTemplate(template string, values map[string]string) string {
	var out strings.Builder
	rest := template
	for {
		start := strings.Index(rest, "{{")
		if start < 0 {
			out.WriteString(rest)
			return out.String()
		}
		end := strings.Index(rest[start+2:], "}}")
		if end < 0 {
			out.WriteString(rest)
			return out.String()
		}
		out.WriteString(rest[:start])
		tag := rest[start+2 : start+2+end]
		after := rest[start+2+end+2:]
		if strings.HasPrefix(tag, "#") || strings.HasPrefix(tag, "^") {
			name := tag[1:]
			closeTag := "{{/" + name + "}}"
			stop := strings.Index(after, closeTag)
			if stop < 0 {
				out.WriteString("{{" + tag + "}}")
				rest = after
				continue
			}
			filled := strings.TrimSpace(values[name]) != ""
			if filled == (tag[0] == '#') {
				out.WriteString(renderTemplate(after[:stop], values))
			}
			rest = after[stop+len(closeTag):]
			continue
		}
		if value, ok := values[tag]; ok {
			out.WriteString(value)
		} else {
			out.WriteString("{{" + tag + "}}")
		}
		rest = after
	}
}
