//go:build !unix

package image

// activeDetect is a no-op on platforms without /dev/tty + termios; graphics
// support is reported as unavailable and Tide runs in text-only mode.
func activeDetect() (Capability, bool) {
	return Capability{Reason: "graphics not supported on this platform"}, false
}

func queryCellSize(bool) (int, int) { return 0, 0 }

// CellSizeFromWinsize is unavailable off-unix.
func CellSizeFromWinsize() (int, int) { return 0, 0 }
