package image

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	_ "image/gif"  // GIF decoder (first frame)
	_ "image/jpeg" // JPEG decoder
	_ "image/png"  // PNG decoder

	_ "golang.org/x/image/webp" // WebP decoder
)

// minDecodedDimension rejects sub-16px decodes as almost certainly icons /
// tracking pixels that slipped past URL filtering.
const minDecodedDimension = 16

// ErrImageTooSmall is returned by Decode when the decoded image is smaller than
// minDecodedDimension on either axis.
var ErrImageTooSmall = errors.New("image: decoded image too small to display")

// Decode decodes PNG, JPEG, GIF (first frame) or WebP bytes into an
// image.Image. It rejects images that are too small to be a real lead image.
func Decode(data []byte) (image.Image, error) {
	if len(data) == 0 {
		return nil, errors.New("image: empty payload")
	}
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("image: decode: %w", err)
	}
	b := img.Bounds()
	if b.Dx() < minDecodedDimension || b.Dy() < minDecodedDimension {
		return nil, fmt.Errorf("%w (%dx%d, %s)", ErrImageTooSmall, b.Dx(), b.Dy(), format)
	}
	return img, nil
}
