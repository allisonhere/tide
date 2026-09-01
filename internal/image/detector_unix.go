//go:build unix

package image

import (
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// probeTimeout bounds a terminal-query read so a silent terminal costs at most
// this much startup latency.
const probeTimeout = 250 * time.Millisecond

// ttyQuery writes seq to /dev/tty in raw mode and returns whatever the terminal
// replies within probeTimeout. seq should end with a Primary Device Attributes
// request (\x1b[c) so the read has a guaranteed terminator. It must only be
// called before Bubble Tea takes over stdin.
func ttyQuery(seq string) (string, bool) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return "", false
	}
	defer tty.Close()

	fd := int(tty.Fd())
	orig, err := unix.IoctlGetTermios(fd, ioctlGetTermios)
	if err != nil {
		return "", false
	}
	raw := *orig
	raw.Lflag &^= unix.ECHO | unix.ICANON | unix.ISIG | unix.IEXTEN
	raw.Iflag &^= unix.IXON | unix.ICRNL | unix.BRKINT | unix.INPCK | unix.ISTRIP
	raw.Cc[unix.VMIN] = 0
	raw.Cc[unix.VTIME] = 1 // 0.1s per read
	if err := unix.IoctlSetTermios(fd, ioctlSetTermios, &raw); err != nil {
		return "", false
	}
	defer unix.IoctlSetTermios(fd, ioctlSetTermios, orig)

	if _, err := tty.WriteString(seq); err != nil {
		return "", false
	}
	return readWithDeadline(tty, probeTimeout), true
}

// activeDetect asks the terminal whether it speaks the Kitty graphics protocol
// (and, incidentally, its cell pixel size). Only called before the TUI starts.
func activeDetect() (Capability, bool) {
	reply, ok := ttyQuery(probeSequence(31) + "\x1b[16t\x1b[c")
	if !ok {
		return Capability{Reason: "active probe: /dev/tty unavailable"}, false
	}

	cap := Capability{}
	if strings.Contains(reply, "_Gi=31;OK") {
		cap.Supported = true
		cap.Protocol = ProtocolKitty
		cap.Reason = "active probe: kitty graphics OK"
	}
	if w, h := parseCellSizeReport(reply); w > 0 && h > 0 {
		cap.CellW, cap.CellH = w, h
	}
	return cap, cap.Supported
}

// queryCellSize returns the terminal cell pixel size. It tries the free
// TIOCGWINSZ path first; if that yields nothing and activeProbe is set (safe
// only pre-TUI) it asks the terminal directly with CSI 16 t.
func queryCellSize(activeProbe bool) (int, int) {
	if w, h := probeCellSize(); w > 0 && h > 0 {
		return w, h
	}
	if !activeProbe {
		return 0, 0
	}
	if reply, ok := ttyQuery("\x1b[16t\x1b[c"); ok {
		return parseCellSizeReport(reply)
	}
	return 0, 0
}

// readWithDeadline reads from r until it would block past d or a Primary Device
// Attributes reply (ending in 'c') is seen.
func readWithDeadline(r *os.File, d time.Duration) string {
	deadline := time.Now().Add(d)
	var b strings.Builder
	buf := make([]byte, 256)
	for time.Now().Before(deadline) {
		n, err := r.Read(buf)
		if n > 0 {
			b.Write(buf[:n])
			// DA1 responses look like ESC [ ? ... c  — a reliable terminator.
			if strings.Contains(b.String(), "\x1b[?") && strings.HasSuffix(strings.TrimRight(b.String(), " "), "c") {
				break
			}
		}
		if err != nil {
			if n == 0 {
				continue // VTIME timeout yields (0, nil)
			}
			break
		}
	}
	return b.String()
}

// parseCellSizeReport extracts w,h from an "\x1b[6;<h>;<w>t" CSI 16t reply.
func parseCellSizeReport(s string) (int, int) {
	i := strings.Index(s, "\x1b[6;")
	if i < 0 {
		return 0, 0
	}
	rest := s[i+len("\x1b[6;"):]
	end := strings.IndexByte(rest, 't')
	if end < 0 {
		return 0, 0
	}
	parts := strings.Split(rest[:end], ";")
	if len(parts) != 2 {
		return 0, 0
	}
	h, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	w, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil {
		return 0, 0
	}
	return w, h
}

// probeCellSize reports the terminal cell pixel size via TIOCGWINSZ, or 0,0 when
// the terminal does not populate the pixel fields (common).
func probeCellSize() (int, int) {
	ws, err := unix.IoctlGetWinsize(int(os.Stdout.Fd()), unix.TIOCGWINSZ)
	if err != nil || ws == nil {
		return 0, 0
	}
	if ws.Xpixel == 0 || ws.Ypixel == 0 || ws.Col == 0 || ws.Row == 0 {
		return 0, 0
	}
	return int(ws.Xpixel) / int(ws.Col), int(ws.Ypixel) / int(ws.Row)
}

// CellSizeFromWinsize re-reads the terminal cell pixel size from TIOCGWINSZ, for
// use on WindowSizeMsg (a font-size change alters pixel geometry). Returns 0,0
// when the terminal does not report pixels; callers keep their previous value.
func CellSizeFromWinsize() (int, int) {
	return probeCellSize()
}
