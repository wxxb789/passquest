package note

import "strings"

// The Anki deck and note model. Q-0007-02 chose these names; a different name
// is a flag, not a migration, because no card exists until the learner pushes.
const (
	Deck      = "Pass Quest"
	ModelName = "PassQuest Note"
	Tag       = "passquest"
)

// ModelFields is the field order sent to createModel and used by export.
var ModelFields = []string{"Front", "Back", "Tactic", "Source"}

// ModelCSS is deliberately small: a system font stack, no external asset, no
// fixed height and no overflow clipping, so long imported text wraps at phone
// width. Goal 0012 owns the later visual system.
const ModelCSS = `.card {
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

// FrontTemplate renders the recognition prompt.
const FrontTemplate = `<div class="pq-front">{{Front}}</div>`

// BackTemplate reveals the meaning, example, tactic and source after the
// learner answers the Anki card.
const BackTemplate = `{{FrontSide}}
<hr id="answer">
<div class="pq-back">{{Back}}</div>
{{#Tactic}}<div class="pq-tactic"><span class="pq-label">Tactic</span> {{Tactic}}</div>{{/Tactic}}
<div class="pq-source">{{Source}}</div>`

// ModelSpec is the createModel document owned by this package.
type ModelSpec struct {
	Deck          string
	Name          string
	Fields        []string
	CSS           string
	FrontTemplate string
	BackTemplate  string
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
	}
}

// RenderCard substitutes already-rendered field HTML into the card templates.
// It exists for inspection and tests; Anki performs the real rendering.
func RenderCard(fields Fields) string {
	values := map[string]string{
		"Front":  fields.Front,
		"Back":   fields.Back,
		"Tactic": fields.Tactic,
		"Source": fields.Source,
	}
	front := renderTemplate(FrontTemplate, values)
	back := renderTemplate(BackTemplate, values)
	return strings.ReplaceAll(back, "{{FrontSide}}", front)
}

func renderTemplate(template string, values map[string]string) string {
	out := template
	for name, value := range values {
		open := "{{#" + name + "}}"
		closeTag := "{{/" + name + "}}"
		for {
			start := strings.Index(out, open)
			if start < 0 {
				break
			}
			end := strings.Index(out[start:], closeTag)
			if end < 0 {
				break
			}
			end += start + len(closeTag)
			block := out[start+len(open) : end-len(closeTag)]
			if strings.TrimSpace(value) == "" {
				block = ""
			}
			out = out[:start] + block + out[end:]
		}
		out = strings.ReplaceAll(out, "{{"+name+"}}", value)
	}
	return out
}
