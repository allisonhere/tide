package ui

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/image/draw"
)

// kittyImageMsg is sent when an article's lead image has been fetched and decoded.
type kittyImageMsg struct {
	articleID int64
	img       image.Image
	err       error
}

// kittyUploadedMsg is sent after image data has been transmitted to the terminal.
type kittyUploadedMsg struct {
	articleID int64
}

// kittyPlacedMsg is sent after the image has been placed on screen.
type kittyPlacedMsg struct{}

// kittyImage holds a decoded, scaled image ready for kitty rendering.
type kittyImage struct {
	id       uint32 // kitty image ID
	rows     int    // height in terminal rows
	cols     int    // width in terminal cols
	encoded  string // base64-encoded RGBA data
	uploaded bool   // true once transmitted to terminal
}

var nextKittyID uint32 = 1

// detectKittySupport checks whether the terminal supports the kitty graphics
// protocol by inspecting environment variables. Covers kitty, WezTerm, and
// Ghostty — the main terminals that implement the protocol.
func detectKittySupport() bool {
	switch os.Getenv("TERM_PROGRAM") {
	case "kitty", "WezTerm", "ghostty":
		return true
	}
	if t := os.Getenv("TERM"); strings.Contains(t, "kitty") {
		return true
	}
	return false
}

// fetchArticleImageCmd fetches and decodes the lead image for an article.
func fetchArticleImageCmd(articleID int64, imageURL string) tea.Cmd {
	return func() tea.Msg {
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Get(imageURL)
		if err != nil {
			return kittyImageMsg{articleID: articleID, err: err}
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return kittyImageMsg{articleID: articleID, err: fmt.Errorf("HTTP %d", resp.StatusCode)}
		}

		img, _, err := image.Decode(resp.Body)
		if err != nil {
			return kittyImageMsg{articleID: articleID, err: err}
		}

		return kittyImageMsg{articleID: articleID, img: img}
	}
}

const (
	// Default cell dimensions in pixels (common terminal defaults).
	defaultCellWidth  = 8
	defaultCellHeight = 16
)

// scaleImage scales img to fit within maxCols terminal columns,
// preserving aspect ratio. Returns the scaled image and its dimensions in cells.
func scaleImage(img image.Image, maxCols int) (*image.RGBA, int, int) {
	bounds := img.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()

	targetPixelW := maxCols * defaultCellWidth
	if srcW <= targetPixelW {
		targetPixelW = srcW
	}

	scale := float64(targetPixelW) / float64(srcW)
	targetPixelH := int(float64(srcH) * scale)

	cols := targetPixelW / defaultCellWidth
	rows := targetPixelH / defaultCellHeight
	if rows < 1 {
		rows = 1
	}
	if cols < 1 {
		cols = 1
	}

	// Scale to exact cell-aligned pixel dimensions for clean rendering.
	finalW := cols * defaultCellWidth
	finalH := rows * defaultCellHeight

	dst := image.NewRGBA(image.Rect(0, 0, finalW, finalH))
	draw.BiLinear.Scale(dst, dst.Bounds(), img, bounds, draw.Over, nil)
	return dst, rows, cols
}

// prepareKittyImage scales an image and prepares it for kitty transmission.
func prepareKittyImage(img image.Image, maxCols int) *kittyImage {
	scaled, rows, cols := scaleImage(img, maxCols)

	id := nextKittyID
	nextKittyID++

	return &kittyImage{
		id:      id,
		rows:    rows,
		cols:    cols,
		encoded: base64.StdEncoding.EncodeToString(scaled.Pix),
	}
}

// kittyTransmitCmd sends the image data to the terminal via /dev/tty.
// Uses direct display mode (not unicode placeholders) for broad terminal compat.
func kittyTransmitCmd(articleID int64, ki *kittyImage) tea.Cmd {
	return func() tea.Msg {
		tty, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
		if err != nil {
			return kittyUploadedMsg{articleID: articleID}
		}
		defer tty.Close()

		w := ki.cols * defaultCellWidth
		h := ki.rows * defaultCellHeight

		// Transmit image data in chunks (kitty protocol max ~4096 base64 chars per chunk).
		// Use a=t (transmit only, don't display yet).
		data := ki.encoded
		first := true
		for len(data) > 0 {
			chunk := data
			more := 0
			if len(chunk) > 4096 {
				chunk = data[:4096]
				data = data[4096:]
				more = 1
			} else {
				data = ""
			}

			var buf bytes.Buffer
			if first {
				fmt.Fprintf(&buf, "\x1b_Gf=32,s=%d,v=%d,i=%d,a=t,t=d,m=%d;", w, h, ki.id, more)
				first = false
			} else {
				fmt.Fprintf(&buf, "\x1b_Gm=%d;", more)
			}
			buf.WriteString(chunk)
			buf.WriteString("\x1b\\")
			tty.Write(buf.Bytes())
		}

		ki.uploaded = true
		return kittyUploadedMsg{articleID: articleID}
	}
}

// kittyPlaceCmd places an already-transmitted image at a specific screen
// position using ANSI cursor positioning + kitty display command.
func kittyPlaceCmd(ki *kittyImage, screenRow, screenCol int) tea.Cmd {
	if ki == nil || !ki.uploaded {
		return nil
	}
	imageID := ki.id
	return func() tea.Msg {
		tty, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
		if err != nil {
			return kittyPlacedMsg{}
		}
		defer tty.Close()

		// Save cursor, move to target position, place image, restore cursor.
		fmt.Fprintf(tty, "\x1b7\x1b[%d;%dH\x1b_Ga=p,i=%d,C=1\x1b\\\x1b8",
			screenRow, screenCol, imageID)

		return kittyPlacedMsg{}
	}
}

// kittyDeleteCmd deletes a kitty image from the terminal.
func kittyDeleteCmd(imageID uint32) tea.Cmd {
	return func() tea.Msg {
		tty, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
		if err != nil {
			return nil
		}
		defer tty.Close()
		fmt.Fprintf(tty, "\x1b_Ga=d,d=i,i=%d\x1b\\", imageID)
		return nil
	}
}

// kittyImageBlankLines returns N blank lines to reserve space for the image
// in the viewport content.
func kittyImageBlankLines(rows int) string {
	return strings.Repeat("\n", rows)
}
