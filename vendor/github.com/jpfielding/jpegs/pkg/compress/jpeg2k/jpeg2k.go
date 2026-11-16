// Package jpeg2k implements a lossless JPEG 2000 Part 1 (ITU-T T.800 |
// ISO/IEC 15444-1) codec, ported from dicos.rs crates/jpeg2k.
//
// Profile: raw codestream (no JP2 box), one tile, one tile-part, one
// unsigned 16-bit component, reversible 5/3 DWT, LRCP, one quality layer,
// default precincts, code-block style 0, zero grid origins. Streams outside
// it decode to ErrUnsupported. Output is verified against OpenJPEG.
//
// Streams written by this package before v0.3.0 (and by dicos.rs 1.0.0)
// were not JPEG 2000; Decode still reads them (see LegacyPolicy).
package jpeg2k

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"io"
)

// DICOM transfer syntax UID for JPEG 2000 Image Compression (Lossless Only).
const TransferSyntaxUID = "1.2.840.10008.1.2.4.90"

// Options configures encoding.
type Options struct {
	// DecompLevels is the number of 5/3 DWT levels, 0..32 (default 5).
	DecompLevels int
	// CodeBlockWidthExp and CodeBlockHeightExp are the code-block size
	// exponents, each 2..10 with a sum of at most 12 (T.800 Table A.18;
	// default 6, i.e. 64x64).
	CodeBlockWidthExp, CodeBlockHeightExp int
}

// DefaultOptions returns the options Encode uses for nil.
func DefaultOptions() *Options {
	return &Options{DecompLevels: 5, CodeBlockWidthExp: 6, CodeBlockHeightExp: 6}
}

// Validate checks the options against the supported profile.
func (o *Options) Validate() error {
	switch {
	case o.CodeBlockWidthExp < 2 || o.CodeBlockWidthExp > 10:
		return fmt.Errorf("%w: code-block width exponent %d not in 2..10", ErrInvalidData, o.CodeBlockWidthExp)
	case o.CodeBlockHeightExp < 2 || o.CodeBlockHeightExp > 10:
		return fmt.Errorf("%w: code-block height exponent %d not in 2..10", ErrInvalidData, o.CodeBlockHeightExp)
	case o.CodeBlockWidthExp+o.CodeBlockHeightExp > 12:
		return fmt.Errorf("%w: code-block exponents sum to %d > 12", ErrInvalidData, o.CodeBlockWidthExp+o.CodeBlockHeightExp)
	case o.DecompLevels < 0 || o.DecompLevels > 32:
		return fmt.Errorf("%w: %d decomposition levels not in 0..32", ErrInvalidData, o.DecompLevels)
	}
	return nil
}

// Encode writes img as a T.800 codestream. Only *image.Gray16 is supported
// (the format is unsigned 16-bit gray); other types return ErrUnsupported.
// A nil opts means DefaultOptions().
func Encode(w io.Writer, img image.Image, opts *Options) error {
	if opts == nil {
		opts = DefaultOptions()
	}
	if err := opts.Validate(); err != nil {
		return err
	}
	src, ok := img.(*image.Gray16)
	if !ok {
		return fmt.Errorf("%w: image type %T (only *image.Gray16)", ErrUnsupported, img)
	}
	b := src.Bounds()
	width, height := b.Dx(), b.Dy()
	if width <= 0 || height <= 0 {
		return fmt.Errorf("%w: empty image %v", ErrInvalidData, b)
	}
	if uint64(width) > 0xFFFFFFFF || uint64(height) > 0xFFFFFFFF {
		return fmt.Errorf("%w: image %dx%d exceeds the SIZ range", ErrUnsupported, width, height)
	}
	component := make([]int32, 0, width*height)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		row := src.Pix[src.PixOffset(b.Min.X, y):]
		for x := range width {
			component = append(component, int32(binary.BigEndian.Uint16(row[2*x:]))-dcShift)
		}
	}
	mb := func(kind bandKind, _ int) int { return qcdEpsilon(kind) + guardBits - 1 }
	tile, err := encodeTile(component, width, height, opts.DecompLevels, opts.CodeBlockWidthExp, opts.CodeBlockHeightExp, mb)
	if err != nil {
		return err
	}
	out, err := appendCodestream(nil, width, height, opts.DecompLevels, opts.CodeBlockWidthExp, opts.CodeBlockHeightExp, tile)
	if err != nil {
		return err
	}
	_, err = w.Write(out)
	return err
}

// Decode reads a codestream with LegacyAuto. T.800 streams decode to
// *image.Gray16; legacy streams to *image.Gray, *image.Gray16 or
// *image.RGBA as they were written.
func Decode(r io.Reader) (image.Image, error) {
	return DecodeWithPolicy(r, LegacyAuto)
}

// DecodeWithPolicy reads a codestream under policy p.
func DecodeWithPolicy(r io.Reader, p LegacyPolicy) (image.Image, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	h, err := parseCodestream(data)
	if err != nil {
		return nil, err
	}
	legacy, err := useLegacy(h, p)
	if err != nil {
		return nil, err
	}
	if legacy {
		return decodeLegacy(h)
	}
	return decodeStandard(h)
}

// DecodeConfig returns the dimensions and colour model Decode would produce,
// rejecting streams outside the supported profile.
func DecodeConfig(r io.Reader) (image.Config, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return image.Config{}, err
	}
	h, err := parseCodestream(data)
	if err != nil {
		return image.Config{}, err
	}
	if legacy, err := useLegacy(h, LegacyAuto); err != nil || legacy {
		return legacyConfig(h), err
	}
	if _, err := h.validateStandard(); err != nil {
		return image.Config{}, err
	}
	return image.Config{ColorModel: color.Gray16Model, Width: h.width(), Height: h.height()}, nil
}

func useLegacy(h *header, p LegacyPolicy) (bool, error) {
	switch p {
	case LegacyAuto:
		return h.isLegacy(), nil
	case StandardOnly:
		return false, nil
	case LegacyOnly:
		if !h.isLegacy() {
			return false, fmt.Errorf("%w: not a legacy stream", ErrInvalidData)
		}
		return true, nil
	}
	return false, fmt.Errorf("%w: unknown LegacyPolicy %d", ErrInvalidData, p)
}

func decodeStandard(h *header) (image.Image, error) {
	mbFor, err := h.validateStandard()
	if err != nil {
		return nil, err
	}
	w, ht, levels := h.width(), h.height(), int(h.levels)
	xcb, ycb := int(h.cbw)+2, int(h.cbh)+2
	// Bound everything the header sizes: the int32 coefficient buffer and
	// the per-code-block state, before building geometry.
	if err := checkDecodedSize(w, ht, 1, 4); err != nil {
		return nil, err
	}
	if err := checkCodeBlockState(w, ht, levels, xcb, ycb); err != nil {
		return nil, err
	}
	coeffs, err := decodeTile(h.payload, w, ht, levels, xcb, ycb, mbFor)
	if err != nil {
		return nil, err
	}
	img := image.NewGray16(image.Rect(0, 0, w, ht))
	for i, v := range coeffs {
		binary.BigEndian.PutUint16(img.Pix[2*i:], uint16(clampInt(int(v)+dcShift, 0xFFFF)))
	}
	return img, nil
}
