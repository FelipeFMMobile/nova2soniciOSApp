package mcp

import "regexp"

type Policy string

const (
	ReadOnly       Policy = "read_only"
	ExplicitIntent Policy = "explicit_intent"
	ConfirmLater   Policy = "confirm_later"
)

var aliasPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,23}$`)
