//go:build linux

package hostinfo

import (
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

func kernel() string {
	var name unix.Utsname
	if err := unix.Uname(&name); err != nil {
		return ""
	}

	return strings.TrimRight(string(name.Sysname[:]), "\x00") +
		" " + strings.TrimRight(string(name.Release[:]), "\x00")
}

// uptime reads /proc/uptime, whose first field is seconds since boot.
func uptime() time.Duration {
	raw, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}

	first, _, found := strings.Cut(string(raw), " ")
	if !found {
		return 0
	}

	secs, err := strconv.ParseFloat(first, 64)
	if err != nil {
		return 0
	}

	return time.Duration(secs * float64(time.Second))
}
