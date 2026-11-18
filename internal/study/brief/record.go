package brief

import (
	"context"
	"errors"

	"github.com/wxxb789/passquest/internal/agent"
	"github.com/wxxb789/passquest/internal/store"
	"github.com/wxxb789/passquest/internal/study"
	"github.com/wxxb789/passquest/internal/study/internal/kernel"
)

// StepInput is one step the durable host reports through the callback command.
type StepInput struct {
	RunID      string
	AttemptKey string
	Index      int
	Kind       string
	Title      string
	Body       string
}

// Recorded is one recorded step with the attempt count the run now holds. A
// duplicate keeps the stored step unchanged and only counts a new attempt
// (and reconciliation event), which is why Step is the stored row while
// AttemptKey is the validated request's key.
type Recorded struct {
	Step       store.AgentStep
	AttemptKey string
	Duplicate  bool
	Attempts   int
}

// Record validates and records one host-reported step against the addressed
// run's stored step budget. The CLI's `agent record-step` command, the durable
// host's callback, stays its entry point.
func Record(ctx context.Context, s *study.Service, in StepInput) (Recorded, error) {
	_ = ctx
	st := kernel.Store(s)
	run, ok, err := st.AgentRun(in.RunID)
	if err != nil {
		return Recorded{}, err
	}
	if !ok {
		return Recorded{}, study.Refuse("unknown_run", "run %s is unknown", in.RunID)
	}
	step, err := agent.ValidateStep(in.RunID, in.AttemptKey, in.Kind, in.Title, in.Body, in.Index, run.MaxSteps)
	if err != nil {
		return Recorded{}, study.Refuse("invalid_step", "%s", err.Error())
	}
	recorded, duplicate, attempts, err := st.RecordAgentStep(step.RunID, step.Index, step.Kind, step.Title, step.Body, step.AttemptKey, kernel.P(s).Now())
	if err != nil {
		switch {
		case errors.Is(err, store.ErrAgentRunNotFound):
			return Recorded{}, study.Refuse("unknown_run", "%s", err.Error())
		case errors.Is(err, store.ErrAgentRunCancelled):
			return Recorded{}, study.Refuse("run_cancelled", "%s", err.Error())
		case errors.Is(err, store.ErrAgentRunCompleted):
			return Recorded{}, study.Refuse("run_completed", "%s", err.Error())
		default:
			return Recorded{}, err
		}
	}
	return Recorded{Step: recorded, AttemptKey: step.AttemptKey, Duplicate: duplicate, Attempts: attempts}, nil
}
