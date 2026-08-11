package theme

import "errors"

// ErrUnknownMode is returned by [Config.Validate] when the configured mode is
// not one prism understands. It is a sentinel rather than a formatted error so
// that a caller can distinguish a typo in the theme from a failure to read the
// configuration file at all.
var ErrUnknownMode = errors.New("unknown theme mode")
