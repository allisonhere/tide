// Package image is Tide's article-image subsystem: terminal capability
// detection, lazy fetching, disk caching, resizing and Kitty-graphics
// rendering. It has no dependency on the Bubble Tea UI layer so its logic can
// be unit-tested without a terminal.
package image

import (
	"encoding/base64"
	"fmt"
	"strings"
)

// tideImageID is the fixed Kitty image id Tide uses. Only one article image is
// ever on screen, and every draw deletes the previous image before transmitting
// the new one, so a single stable id is sufficient and avoids id bookkeeping.
const tideImageID uint32 = 1974

// kittyChunkSize is the maximum number of base64 payload bytes per Kitty escape
// packet. The protocol requires <= 4096.
const kittyChunkSize = 4096

// escStart / escEnd delimit an APC Kitty graphics command.
const (
	escStart = "\x1b_G"
	escEnd   = "\x1b\\"
)

// buildTransmitPNG returns the ordered escape packets that (1) free any image
// previously held under id, then (2) transmit pngBytes as a PNG (f=100) in
// <=4096-byte base64 chunks. It does NOT place the image; call buildPlace next.
//
// q=2 silences both the "OK" and error replies so nothing leaks into the host
// program's stdin reader.
func buildTransmitPNG(id uint32, pngBytes []byte, chunk int) []string {
	return buildTransmit(id, 100, 0, 0, pngBytes, chunk)
}

// buildTransmitRGBA is the raw-pixel fallback (f=32) for the rare terminal that
// rejects PNG transmission. w and h are the pixel dimensions of the RGBA buffer.
func buildTransmitRGBA(id uint32, w, h int, pix []byte, chunk int) []string {
	return buildTransmit(id, 32, w, h, pix, chunk)
}

func buildTransmit(id uint32, format, w, h int, payload []byte, chunk int) []string {
	if chunk <= 0 {
		chunk = kittyChunkSize
	}
	b64 := base64.StdEncoding.EncodeToString(payload)

	// First, delete any resident image + placements for this id (frees memory).
	out := []string{buildDelete(id)}

	if len(b64) == 0 {
		return out
	}

	for i := 0; i < len(b64); i += chunk {
		end := i + chunk
		if end > len(b64) {
			end = len(b64)
		}
		more := 0
		if end < len(b64) {
			more = 1
		}

		var ctrl string
		if i == 0 {
			// Control keys on the first packet only.
			if format == 32 {
				ctrl = fmt.Sprintf("f=32,s=%d,v=%d,i=%d,a=t,q=2,m=%d", w, h, id, more)
			} else {
				ctrl = fmt.Sprintf("f=100,i=%d,a=t,q=2,m=%d", id, more)
			}
		} else {
			ctrl = fmt.Sprintf("m=%d", more)
		}
		out = append(out, escStart+ctrl+";"+b64[i:end]+escEnd)
	}
	return out
}

// buildPlace positions the cursor at the absolute 1-based cell coordinate
// (x, y), places image id into a cols x rows cell box without moving the cursor
// (C=1), then restores the cursor. It is safe to interleave with a full-screen
// TUI renderer as long as it is emitted as a single write.
func buildPlace(id uint32, x, y, cols, rows int) string {
	place := fmt.Sprintf("%sa=p,i=%d,c=%d,r=%d,C=1,q=2%s", escStart, id, cols, rows, escEnd)
	return wrapAtCursor(x, y, place)
}

// buildKeepalivePlace is identical to buildPlace; it exists as a distinct name
// so callers can express intent (a cheap re-assert of an existing placement
// after a full repaint, no retransmit).
func buildKeepalivePlace(id uint32, x, y, cols, rows int) string {
	return buildPlace(id, x, y, cols, rows)
}

// buildDelete removes image id and its placements from the screen and frees its
// data (d=I). Using d=I rather than d=i avoids leaking transmitted image data
// for the lifetime of the terminal session.
func buildDelete(id uint32) string {
	return fmt.Sprintf("%sa=d,d=I,i=%d,q=2%s", escStart, id, escEnd)
}

// buildDeleteAll removes every image and placement (d=A). Used on quit / panic.
func buildDeleteAll() string {
	return escStart + "a=d,d=A" + escEnd
}

// DeleteAllSequence is the escape string that removes every image and
// placement. main.go writes it to stdout as a last-resort cleanup after the
// program exits or panics.
func DeleteAllSequence() string { return buildDeleteAll() }

// wrapAtCursor brackets seq with DECSC/DECRC (save/restore cursor) and an
// absolute cursor move to (x, y) (1-based). The terminal's own cursor position
// is left exactly as it was.
func wrapAtCursor(x, y int, seq string) string {
	if x < 1 {
		x = 1
	}
	if y < 1 {
		y = 1
	}
	var b strings.Builder
	b.WriteString("\x1b7")                          // DECSC – save cursor
	b.WriteString(fmt.Sprintf("\x1b[%d;%dH", y, x)) // CUP – move to row;col
	b.WriteString(seq)
	b.WriteString("\x1b8") // DECRC – restore cursor
	return b.String()
}

// probeSequence is the query written during active capability detection: it asks
// the terminal to transmit-and-report a 1x1 24-bit pixel. q=0 so we actually
// receive the "OK" reply. A Primary Device Attributes query (\x1b[c) is appended
// by the caller as a reliable terminator for the read.
func probeSequence(id uint32) string {
	// "AAAA" decodes to 3 zero bytes = one black RGB pixel.
	return fmt.Sprintf("%si=%d,s=1,v=1,a=q,t=d,f=24;AAAA%s", escStart, id, escEnd)
}
