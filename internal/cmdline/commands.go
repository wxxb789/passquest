package cmdline

import (
	"fmt"
	"strings"

	"github.com/wxxb789/passquest/internal/adaptive"
	"github.com/wxxb789/passquest/internal/study/recommend"
)

// NextCommands renders the recommendation and its follow-up as the commands
// that reach them.
func NextCommands(rec *adaptive.Recommendation, dbArg string, quoter CommandQuoter) (string, string) {
	primary, followUp := recommend.Targets(rec)
	return targetCommand(primary, dbArg, quoter), targetCommand(followUp, dbArg, quoter)
}

// NextDatabaseArgument is the database selection repeated in generated commands
// that open learner history: empty for the app-owned default path, which a
// copied command resolves the same way, and non-empty for any other path chosen
// with --db or PASSQUEST_DB so the command does not silently fall back to the
// default store. The selected path and the default path are the caller's, so
// the resolution rule has one definition (study.DatabasePath).
func NextDatabaseArgument(selected, defaultPath string) string {
	if selected == defaultPath {
		return ""
	}
	return selected
}

// targetCommand renders one recommendation target as the command that reaches
// it.
func targetCommand(t *recommend.Target, dbArg string, quoter CommandQuoter) string {
	if t == nil {
		return ""
	}
	switch t.Kind {
	case recommend.TargetTactic:
		return tacticPracticeCommand(t.SkillID, dbArg, quoter)
	case recommend.TargetLesson:
		return fmt.Sprintf("passquest skill show %s", t.SkillID)
	default:
		return skillPracticeCommand(t.SkillID, t.Material, t.IncludeQuestion, dbArg, quoter)
	}
}

func tacticPracticeCommand(skillID, dbArg string, quoter CommandQuoter) string {
	command := fmt.Sprintf("passquest tactic practice %s --stage guided", skillID)
	return withDatabaseArgument(command, dbArg, quoter)
}

// withDatabaseArgument appends the selected database to a command that opens
// learner history. Read-only lesson commands such as `skill show` never open
// the store and do not use it.
func withDatabaseArgument(command, dbArg string, quoter CommandQuoter) string {
	if dbArg == "" {
		return command
	}
	return command + " --db " + quoter.Quote(dbArg)
}

func skillPracticeCommand(skillID string, material *adaptive.Material, includeQuestion bool, dbArg string, quoter CommandQuoter) string {
	parts := []string{"passquest", "skill", "practice", skillID}
	if material != nil && material.SourceID != "" {
		parts = append(parts, "--source", material.SourceID)
	}
	if material != nil && includeQuestion {
		parts = append(parts, "--question", material.QuestionID)
	}
	if dbArg != "" {
		parts = append(parts, "--db", quoter.Quote(dbArg))
	}
	return strings.Join(parts, " ")
}
