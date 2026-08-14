//go:build darwin

package hostinfo

import (
	"time"

	"golang.org/x/sys/unix"
)

// kernel reports the Darwin release, which is what `uname -r` prints.
func kernel() string {
	release, err := unix.Sysctl("kern.osrelease")
	if err != nil {
		return ""
	}

	return "Darwin " + release
}

// uptime derives the boot time from the kernel and subtracts. Returns zero
// when it cannot be read, which FormatUptime renders as empty.
func uptime() time.Duration {
	tv, err := unix.SysctlTimeval("kern.boottime")
	if err != nil {
		return 0
	}

	return time.Since(time.Unix(tv.Sec, int64(tv.Usec)*1000))
}
