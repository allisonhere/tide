package image

import (
	"bytes"
	"image"
	stdpng "image/png"
	"strings"
	"sync"
)

// Renderer produces the Kitty-graphics escape sequences for a single article
// image. It performs NO terminal I/O: the caller embeds the returned string at
// the very end of the TUI frame so the host renderer is the only writer and the
// image is placed after every text cell has been painted (nothing overwrites
// it, and nothing can chop the escape sequence).
type Renderer interface {
	// Supported reports whether this renderer can actually produce graphics.
	Supported() bool
	// Frame returns the sequence that places img at the 1-based cell coordinate
	// (x, y), spanning cols x rows cells, leaving the cursor untouched. It
	// includes a one-time image transmission whenever the pixels changed since
	// the previous call; steady-state calls return just the ~40-byte placement.
	Frame(img image.Image, x, y, cols, rows int) string
	// Hide returns the sequence that removes the current placement from the
	// screen. The transmitted image data is kept so a later Frame re-shows it
	// with only a placement.
	Hide() string
	// ClearAll returns the sequence that purges every image and placement from
	// the terminal (used on quit).
	ClearAll() string
}

// noopRenderer is used when graphics are unsupported or disabled.
type noopRenderer struct{}

// NewNoopRenderer returns a Renderer that produces nothing.
func NewNoopRenderer() Renderer { return noopRenderer{} }

func (noopRenderer) Supported() bool                              { return false }
func (noopRenderer) Frame(image.Image, int, int, int, int) string { return "" }
func (noopRenderer) Hide() string                                 { return "" }
func (noopRenderer) ClearAll() string                             { return "" }

// kittyRenderer builds Kitty protocol strings. It keeps a fingerprint of the
// last transmitted image so it only re-sends pixel data when the image really
// changed.
type kittyRenderer struct {
	mu       sync.Mutex
	id       uint32
	haveData bool
	lastKey  string
}

// NewKittyRenderer returns a string-producing Kitty renderer.
func NewKittyRenderer() Renderer {
	return &kittyRenderer{id: tideImageID}
}

func (r *kittyRenderer) Supported() bool { return r != nil }

func (r *kittyRenderer) Frame(img image.Image, x, y, cols, rows int) string {
	if img == nil {
		return ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	var b strings.Builder
	key := imageIdentity(img)
	if !r.haveData || key != r.lastKey {
		png, err := encodePNG(img)
		if err != nil {
			return ""
		}
		for _, packet := range buildTransmitPNG(r.id, png, kittyChunkSize) {
			b.WriteString(packet)
		}
		r.haveData = true
		r.lastKey = key
	}
	b.WriteString(buildPlace(r.id, x, y, cols, rows))
	return b.String()
}

func (r *kittyRenderer) Hide() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	// d=i removes placements but keeps the image data resident for a cheap
	// re-show; d=A on quit frees everything.
	return escStart + "a=d,d=i,i=" + itoa(r.id) + ",q=2" + escEnd
}

func (r *kittyRenderer) ClearAll() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.haveData = false
	r.lastKey = ""
	return buildDeleteAll()
}

func itoa(v uint32) string {
	if v == 0 {
		return "0"
	}
	var buf [10]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

func encodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	enc := stdpng.Encoder{CompressionLevel: stdpng.BestSpeed}
	if err := enc.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// imageIdentity is a cheap fingerprint used to decide whether a retransmit is
// needed: exact bounds plus a sparse sampling of pixels.
func imageIdentity(img image.Image) string {
	b := img.Bounds()
	h := uint64(1469598103934665603) // FNV-1a offset basis
	mix := func(v uint32) {
		h ^= uint64(v)
		h *= 1099511628211
	}
	mix(uint32(b.Dx()))
	mix(uint32(b.Dy()))
	stepX := b.Dx()/16 + 1
	stepY := b.Dy()/16 + 1
	for y := b.Min.Y; y < b.Max.Y; y += stepY {
		for x := b.Min.X; x < b.Max.X; x += stepX {
			cr, cg, cb, ca := img.At(x, y).RGBA()
			mix(cr)
			mix(cg)
			mix(cb)
			mix(ca)
		}
	}
	const hexdigits = "0123456789abcdef"
	var out [16]byte
	for i := 0; i < 16; i++ {
		out[15-i] = hexdigits[h&0xf]
		h >>= 4
	}
	return string(out[:])
}
