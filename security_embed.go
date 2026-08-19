package qh8z

import _ "embed"

// SecurityPolicy is the source-of-truth vulnerability disclosure policy.
//
//go:embed SECURITY.md
var SecurityPolicy string
