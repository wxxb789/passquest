package kernel

import (
	"errors"
	"fmt"
)

// Error is the only error type study code constructs, and it is always a
// refusal (learner-fixable; CLI usage error, exit 2). An internal failure is
// never wrapped: entry points return the raw error, exactly the value the CLI
// hands to internalError today, so err.Error() — the CLI's stderr bytes and
// the desktop's internal_error message — cannot drift. "Internal" is simply
// "not a *Error".
type Error struct {
	// Code is the CLI's usage code for this refusal, or the code the area's
	// classification table assigns to the wrapped sentinel.
	Code string
	// Message is exactly the bytes the CLI prints today.
	Message string
	// Err is the domain sentinel the area classifies as learner-fixable, nil
	// for a refusal raised by study code. Invariant: Err != nil implies
	// Message == Err.Error() (Classify sets it so).
	Err error
}

func (e *Error) Error() string { return e.Message }

func (e *Error) Unwrap() error { return e.Err }

// Refuse builds a refusal: a learner-fixable study error carrying the code
// and the message the CLI prints for it.
func Refuse(code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// Rule maps one domain sentinel to the code the CLI prints for it.
type Rule struct {
	Is   error
	Code string
}

// Classify wraps a table hit as *Error{Code, err.Error(), err}; a miss and nil
// are returned unchanged (never wrapped). An existing *Error is returned as
// it is, so a refusal already raised by study code keeps its own code.
func Classify(err error, table []Rule) error {
	if err == nil {
		return nil
	}
	var existing *Error
	if errors.As(err, &existing) {
		return err
	}
	for _, rule := range table {
		if errors.Is(err, rule.Is) {
			return &Error{Code: rule.Code, Message: err.Error(), Err: err}
		}
	}
	return err
}
