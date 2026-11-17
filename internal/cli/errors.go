package cli

import (
	"encoding/json"
	"fmt"
	"io"
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
