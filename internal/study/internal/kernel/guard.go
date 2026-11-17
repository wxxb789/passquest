package kernel

import (
	"errors"
	"os"
	"time"

	"github.com/wxxb789/passquest/internal/store"
)

// Guard refuses command with assessment_active while a run is active, and
// records an elapsed run abandoned instead of blocking. It reads Now only
// while a run is active.
func Guard(u *Unit, command string) error {
	st := Store(u)
	run, found, err := st.ActiveAssessmentRun()
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	at := P(u).Now()
	if !at.Before(run.DeadlineAt) {
		if _, err := st.AbandonAssessmentRun(run.ID, at, "window_elapsed"); err != nil && !errors.Is(err, store.ErrAssessmentRunState) {
			return err
		}
		return nil
	}
	return Refuse("assessment_active",
		"%s is unavailable while assessment run %d is active (deadline %s); complete it, or record `passquest assess abandon %d`, before using coaching",
		command, run.ID, run.DeadlineAt.UTC().Format(time.RFC3339), run.ID)
}

// GuardPath is the CLI's pre-flag-parse guard: with no database file there is
// no guard and no file is created; otherwise the store opens through Open, so
// PrivateDir decides OpenPrivate vs Open exactly as the command's own open
// does.
func GuardPath(p Ports, command string) error {
	if _, err := os.Stat(p.DBPath); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	u, err := Open(p)
	if err != nil {
		return err
	}
	defer u.Close()
	return Guard(u, command)
}
