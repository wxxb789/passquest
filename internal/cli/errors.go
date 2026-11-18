package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/wxxb789/passquest/internal/study"
)

func writeError(opt Options, jsonMode bool, code, message string) {
	if jsonMode {
		_ = writeJSONTo(opt.Stderr, errorDoc{
			SchemaVersion: ProtocolVersion,
			Error:         errorBody{Code: code, Message: message},
		})
	} else {
		fmt.Fprintf(opt.Stderr, "passquest: %s\n", message)
	}
}

func usageError(opt Options, jsonMode bool, code, message string) int {
	writeError(opt, jsonMode, code, message)
	return 2
}

func internalError(opt Options, jsonMode bool, err error) int {
	if jsonMode {
		_ = writeJSONTo(opt.Stderr, errorDoc{
			SchemaVersion: ProtocolVersion,
			Error:         errorBody{Code: "internal_error", Message: err.Error()},
		})
	} else {
		fmt.Fprintf(opt.Stderr, "passquest: %v\n", err)
	}
	return 1
}

// fail writes a study refusal as the CLI's usage error (exit 2) and anything
// else as an internal error (exit 1). It is the CLI's single mapping of
// orchestration errors; an area mapper keeps its domain table until the area's
// entry point moves into study.
func fail(opt Options, jsonMode bool, err error) int {
	var e *study.Error
	if errors.As(err, &e) {
		return usageError(opt, jsonMode, e.Code, e.Message)
	}
	return internalError(opt, jsonMode, err)
}

func writeJSON(opt Options, value any) int {
	if err := writeJSONTo(opt.Stdout, value); err != nil {
		fmt.Fprintf(opt.Stderr, "passquest: encode JSON: %v\n", err)
		return 1
	}
	return 0
}

func writeJSONTo(w io.Writer, value any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}
