// Package design holds the one design-token sheet every Marum surface wears.
//
// The Mini App and the admin are served by different packages and styled by
// different stylesheets, and for a while each carried its own copy of the
// palette. Copies drift: a brass adjusted in one place stayed unadjusted in
// the other. Both stylesheets now start with this file, so a colour exists
// in exactly one place.
package design

import _ "embed"

// Tokens is the token sheet: the semantic colours for the light and dark
// themes, the radii, and nothing else. It is prepended to each surface's own
// stylesheet at serve time.
//
//go:embed tokens.css
var Tokens []byte
