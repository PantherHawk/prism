//go:build !darwin && !linux

package hostinfo

import "time"

// prism targets Unix. On anything else the two platform-specific facts are
// simply unknown, and the splash omits their lines.
func kernel() string { return "" }

func uptime() time.Duration { return 0 }
