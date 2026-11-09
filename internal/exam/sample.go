package exam

import _ "embed"

// The shipped sample is embedded so the CLI needs no external content file and
// no filesystem lookup at runtime.
//
//go:embed testdata/sample-0001.json
var sampleJSON []byte
