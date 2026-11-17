// Package practice orchestrates local practice: choosing the question a
// request names, holding it the way its adapter displays it, and grading the
// answer. Results carry exactly the inputs the CLI's view builders and the
// desktop's studio mappers read; nothing here renders.
package practice

import (
	"context"
	"errors"
	"time"

	"github.com/wxxb789/passquest/internal/attempt"
	"github.com/wxxb789/passquest/internal/exam"
	"github.com/wxxb789/passquest/internal/skill"
	"github.com/wxxb789/passquest/internal/source"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/study"
	"github.com/wxxb789/passquest/internal/study/internal/kernel"
	"github.com/wxxb789/passquest/internal/tactic"
)

// Kind is one practice request kind. The values are the studio.Practice*
// protocol strings: a desktop presentation row records the kind the window
// asked for, including "next".
type Kind string

const (
	KindSample Kind = "sample"
	KindSource Kind = "source"
	KindTactic Kind = "tactic"
	KindSkill  Kind = "skill"
	KindNext   Kind = "next"
)

// Tactic stages are protocol values.
const (
	StageGuided      = "guided"
	StageIndependent = "independent"
)

// Passage exposure values are part of the skill practice protocol. A fresh
// passage can supply independent transfer evidence; an already-seen passage
// cannot, even when the question is new.
const (
	PassageFresh = "fresh"
	PassageSeen  = "already_seen"
)

// Request selects practice the way the matching CLI command does. Kind picks
// the selection rule; SourceID, Page and QuestionID pin the material, TacticID
// and SkillID name the registered lesson, and Stage the tactic stage (empty
// means guided until one guided attempt exists). A kind-next request carries
// the resolved recommendation target instead: TacticID with Stage, or SkillID
// with the material the target named.
type Request struct {
	Kind       Kind
	SourceID   string
	Page       int
	QuestionID string
	TacticID   string
	Stage      string
	SkillID    string
	// RequireSourceID refuses a source request whose id is empty or all
	// whitespace before reading the store. The desktop window sets it: a blank
	// id is a form error there, not a lookup. The CLI leaves it false, so
	// `practice --source '   '` stays a lookup that fails as not-found.
	RequireSourceID bool
}

// Hold says what showing the item spends. It reproduces today's four display
// sequences exactly.
type Hold uint8

const (
	// HoldDocument is the CLI's --json awaiting document: nothing is spent for
	// practice or skill; a tactic request reads the clock once and records the
	// guided walkthrough.
	HoldDocument Hold = iota + 1
	// HoldPrompt is the CLI's human prompt: the presentation is recorded
	// before the question is shown, and a guided walkthrough is recorded as
	// delivered.
	HoldPrompt
	// HoldOneShot is the CLI's --json answer: nothing is spent now; Answer
	// records the presentation after the attempt.
	HoldOneShot
	// HoldToken is the desktop: the guard runs, the presentation is recorded,
	// and the answer names the token it returned.
	HoldToken
)

// TacticRun is the tactic attempt a selection carries: the resolved stage, the
// display fields copied from the lesson and, for a guided stage only, the
// lesson whose walkthrough is delivered.
type TacticRun struct {
	ID     string
	Title  string
	Stage  string
	Lesson *tactic.Tactic
}

func (r TacticRun) assistance() attempt.Assistance {
	if r.Stage == StageGuided {
		return attempt.AssistanceAssisted
	}
	return attempt.AssistanceIndependent
}

// SkillRun is the skill practice question a selection carries: the lesson, the
// assistance the recorded attempt will hold and the passage exposure.
type SkillRun struct {
	Skill           skill.Skill
	Assistance      attempt.Assistance
	PassageExposure string
}

// Item is one selected practice item: the material a view builder or studio
// mapper reads, plus how Answer finds it again.
type Item struct {
	Content  exam.Exam
	Question exam.Question
	Number   int
	Page     *source.Page     // imported material, nil for embedded content
	Review   *source.Question // the stored review row of an imported question
	Tactic   *TacticRun
	Skill    *SkillRun
	// Started is the presentation time a HoldPrompt item was already recorded
	// with; Answer reads it instead of a second Now.
	Started time.Time
	// Presentation is the desktop row a token item was displayed as, set by
	// Show (HoldToken), Hint and Open.
	Presentation *store.PracticePresentation
	ref          ref
}

// ref says how Answer finds the item again: the hold sequence it was shown
// under and, for a token item, the presentation token.
type ref struct {
	hold  Hold
	token string
}

// Token is the presentation token of a token item.
func (i Item) Token() string {
	if i.Presentation != nil {
		return i.Presentation.Token
	}
	return i.ref.token
}

// Token is the item an answer names by token, before Answer reads it.
func Token(token string) Item {
	return Item{ref: ref{hold: HoldToken, token: token}}
}

// Choice is one answer: a skip, the already parsed selection of the CLI's
// stdin loop, or the label an adapter read.
type Choice struct {
	Label  string
	Skip   bool
	Parsed *attempt.Selection
}

// Graded is one recorded answer. Records is every attempt after the append,
// the input the desktop's feedback condition is computed from; Replayed says
// the token was already answered, so the stored feedback is returned and
// nothing was written.
type Graded struct {
	Item     Item
	Attempt  attempt.Attempt
	Feedback attempt.Feedback
	Records  []attempt.Attempt
	Replayed bool
}

// Show selects the item the request names and holds it the way h says. A token
// display guards against an active assessment run before selecting anything, so
// the guard's refusal wins over a failing selection exactly as it did when the
// desktop ran its own guard before the whole selection; the CLI's holds never
// guard. It classifies the selection error (a refusal keeps its own code);
// display and write failures are returned raw.
func Show(ctx context.Context, s *study.Service, r Request, h Hold) (Item, error) {
	_ = ctx
	st := kernel.Store(s)
	if h == HoldToken {
		if err := kernel.Guard(s, "practice"); err != nil {
			return Item{}, err
		}
	}
	sel, err := choose(st, r)
	if err != nil {
		return Item{}, kernel.Classify(err, practiceRules)
	}
	switch h {
	case HoldDocument:
		if r.Kind == KindTactic {
			at := kernel.P(s).Now()
			if err := recordGuidedExposure(st, sel, at); err != nil {
				return Item{}, err
			}
		}
		return sel.item(nil, ref{hold: h}), nil
	case HoldPrompt:
		started := kernel.P(s).Now()
		if err := st.RecordPresentation(sel.Content.ID, sel.Question.ID, practiceSurface(sel), started); err != nil {
			return Item{}, err
		}
		if err := recordGuidedExposure(st, sel, started); err != nil {
			return Item{}, err
		}
		item := sel.item(nil, ref{hold: h})
		item.Started = started
		return item, nil
	case HoldOneShot:
		return sel.item(nil, ref{hold: h}), nil
	case HoldToken:
		now := kernel.P(s).Now()
		p := store.PracticePresentation{
			Token:          kernel.P(s).NewToken(),
			Kind:           string(r.Kind),
			ContentID:      sel.Content.ID,
			ContentVersion: sel.Content.Version,
			QuestionID:     sel.Question.ID,
			PresentedAt:    now,
		}
		if sel.Page != nil {
			p.SourceID, _, _ = source.ParseContentID(sel.Content.ID)
		}
		if sel.Tactic != nil {
			p.TacticID, p.Stage = sel.Tactic.ID, sel.Tactic.Stage
		}
		if sel.Skill != nil {
			p.SkillID = sel.Skill.Skill.ID
		}
		if err := st.OpenPracticePresentation(p, practiceSurface(sel)); err != nil {
			return Item{}, err
		}
		if err := recordGuidedExposure(st, sel, now); err != nil {
			return Item{}, err
		}
		stored, err := st.PracticePresentationByToken(p.Token)
		if err != nil {
			return Item{}, err
		}
		return sel.item(&stored, ref{hold: h, token: stored.Token}), nil
	}
	return Item{}, kernel.Refuse("invalid_input", "unknown practice hold %d", h)
}

// Open is the latest open token item, or nil. It records nothing: the item was
// already presented.
func Open(ctx context.Context, s *study.Service) (*Item, error) {
	_ = ctx
	st := kernel.Store(s)
	p, found, err := st.LatestOpenPracticePresentation()
	if err != nil || !found {
		return nil, err
	}
	sel, err := presentedSelection(st, p)
	if err != nil {
		return nil, err
	}
	item := sel.item(&p, ref{hold: HoldToken, token: p.Token})
	return &item, nil
}

// Hint opens the authored walkthrough for the displayed item and records it as
// an exposure, so the attempt is recorded as assisted (Q-0011-05). An item
// without an authored walkthrough is refused with hint_unavailable.
func Hint(ctx context.Context, s *study.Service, token string) (Item, error) {
	_ = ctx
	st := kernel.Store(s)
	p, err := openPresentation(st, token)
	if err != nil {
		return Item{}, err
	}
	sel, err := presentedSelection(st, p)
	if err != nil {
		return Item{}, err
	}
	item := sel.item(nil, ref{hold: HoldToken, token: token})
	lesson, ok := HintTactic(item)
	if !ok {
		return Item{}, kernel.Refuse("hint_unavailable", "no authored walkthrough exists for question %s; the attempt stays as it is", sel.Question.ID)
	}
	now := kernel.P(s).Now()
	if p, err = st.MarkPracticeHint(token, now); err != nil {
		return Item{}, err
	}
	if err := st.RecordExposure(store.Exposure{
		TacticID:   lesson.ID,
		ContentID:  sel.Content.ID,
		QuestionID: sel.Question.ID,
		ExposedAt:  p.HintOpenedAt,
	}); err != nil {
		return Item{}, err
	}
	item.Presentation = &p
	return item, nil
}

// Answer grades the item under the choice. Grading a token again returns the
// stored feedback and writes nothing; grading an open token guards against an
// active assessment run first.
func Answer(ctx context.Context, s *study.Service, it Item, c Choice) (Graded, error) {
	_ = ctx
	st := kernel.Store(s)
	if it.ref.hold == HoldToken {
		return answerToken(s, it.ref.token, c)
	}
	sel := it.selection()
	var started time.Time
	switch it.ref.hold {
	case HoldOneShot:
		started = kernel.P(s).Now()
	case HoldPrompt:
		started = it.Started
	default:
		return Graded{}, kernel.Refuse("invalid_input", "practice item was not shown for an answer")
	}
	chosen, err := chooseSelection(sel.Question, c)
	if err != nil {
		return Graded{}, err
	}
	answered := kernel.P(s).Now()
	record, err := buildPracticeAttempt(st, sel, chosen, kernel.P(s).NewSession(), started, answered)
	if err != nil {
		return Graded{}, err
	}
	record, err = st.Append(record)
	if err != nil {
		return Graded{}, err
	}
	feedback := attempt.Explain(sel.Question, record)
	if it.ref.hold == HoldOneShot {
		if err := st.RecordPresentation(sel.Content.ID, sel.Question.ID, practiceSurface(sel), started); err != nil {
			return Graded{}, err
		}
	}
	return Graded{Item: it, Attempt: record, Feedback: feedback}, nil
}

// answerToken grades the presentation the token names. The order is the
// display's: a closed token replays its stored feedback before the guard, an
// open one guards, clamps the answer to the presentation time, upgrades an
// assisted attempt when the hint was opened, and replays the stored feedback
// if another answer closed the token first.
func answerToken(s *study.Service, token string, c Choice) (Graded, error) {
	st := kernel.Store(s)
	p, err := st.PracticePresentationByToken(token)
	if errors.Is(err, store.ErrNotFound) {
		return Graded{}, kernel.Refuse("unknown_token", "practice item %q was never displayed", token)
	}
	if err != nil {
		return Graded{}, err
	}
	if !p.Open() {
		return storedGraded(st, p)
	}
	if err := kernel.Guard(s, "practice"); err != nil {
		return Graded{}, err
	}
	sel, err := presentedSelection(st, p)
	if err != nil {
		return Graded{}, err
	}
	chosen, err := chooseSelection(sel.Question, c)
	if err != nil {
		return Graded{}, err
	}
	now := kernel.P(s).Now()
	if now.Before(p.PresentedAt) {
		now = p.PresentedAt
	}
	record, err := buildPracticeAttempt(st, sel, chosen, kernel.P(s).NewSession(), p.PresentedAt, now)
	if err != nil {
		return Graded{}, err
	}
	if !p.HintOpenedAt.IsZero() && record.Assistance != attempt.AssistanceAssisted {
		if record.TacticID != "" {
			record, err = record.WithTactic(record.TacticID, attempt.AssistanceAssisted)
		} else {
			record, err = record.WithAssistance(attempt.AssistanceAssisted)
		}
		if err != nil {
			return Graded{}, err
		}
	}
	stored, closed, err := st.AppendPracticeAttempt(token, record, now)
	if errors.Is(err, store.ErrPracticeClosed) {
		return storedGraded(st, closed)
	}
	if err != nil {
		return Graded{}, err
	}
	records, err := st.List()
	if err != nil {
		return Graded{}, err
	}
	return Graded{
		Item:     sel.item(&closed, ref{hold: HoldToken, token: token}),
		Attempt:  stored,
		Feedback: attempt.Explain(sel.Question, stored),
		Records:  records,
	}, nil
}

// chooseSelection turns the choice into the selection attempt.BuildRecord
// grades.
func chooseSelection(question exam.Question, c Choice) (attempt.Selection, error) {
	if c.Skip {
		return attempt.Selection{Skip: true}, nil
	}
	if c.Parsed != nil {
		return *c.Parsed, nil
	}
	parsed, err := attempt.ParseSelection(question, c.Label)
	if err != nil {
		return attempt.Selection{}, kernel.Refuse("invalid_answer", "%s", err.Error())
	}
	return parsed, nil
}
