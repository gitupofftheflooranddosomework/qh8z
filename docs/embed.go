package policies

import _ "embed"

// Terms is the source-of-truth Terms of Service published by qh8z.
//
//go:embed legal/TERMS.md
var Terms string

// Privacy is the source-of-truth Privacy Policy published by qh8z.
//
//go:embed legal/PRIVACY.md
var Privacy string

// AcceptableUse is the source-of-truth Acceptable Use Policy published by qh8z.
//
//go:embed legal/ACCEPTABLE_USE.md
var AcceptableUse string
