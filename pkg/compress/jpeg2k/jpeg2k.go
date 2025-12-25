// Package jpeg2k implements JPEG 2000 (Part-1) encoding and decoding.
// This implementation supports lossless compression using the 5/3 reversible
// discrete wavelet transform as specified in ITU-T Rec. T.800 | ISO/IEC 15444-1.
package jpeg2k

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"io"
)

// Common errors
var (
	ErrInvalidFormat    = errors.New("invalid JPEG 2000 format")
	ErrUnsupportedImage = errors.New("unsupported image type")
	ErrDecodeFailed     = errors.New("decode failed")
)

// Options configures JPEG 2000 encoding
type Options struct {
	DecompLevels int              // Number of DWT decomposition levels (default: 5)
	NumLayers    int              // Number of quality layers (default: 1)
	TileWidth    int              // Tile width (0 = single tile)
	TileHeight   int              // Tile height (0 = single tile)
	Progression  ProgressionOrder // Progression order (default: LRCP)
	UseMCT       bool             // Use multi-component transform for RGB
}

// DefaultOptions returns default encoding options
func DefaultOptions() *Options {
	return &Options{
		DecompLevels: 5,
		NumLayers:    1,
		TileWidth:    0,
		TileHeight:   0,
		Progression:  ProgressionLRCP,
		UseMCT:       true,
	}
}

// Encode writes an image to JPEG 2000 format
func Encode(w io.Writer, img image.Image, opts *Options) error {
	if opts == nil {
		opts = DefaultOptions()
	}

	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	if width <= 0 || height <= 0 {
		return errors.New("invalid image dimensions")
	}

	// Extract pixel data based on image type
	var components [][]int
	var precision int
	var signed bool
	var numComps int

	switch src := img.(type) {
	case *image.Gray:
		precision = 8
		signed = false
		numComps = 1
		data := make([]int, width*height)
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				data[y*width+x] = int(src.GrayAt(x+bounds.Min.X, y+bounds.Min.Y).Y)
			}
		}
		components = [][]int{data}

	case *image.Gray16:
		precision = 16
		signed = false
		numComps = 1
		data := make([]int, width*height)
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				data[y*width+x] = int(src.Gray16At(x+bounds.Min.X, y+bounds.Min.Y).Y)
			}
		}
		components = [][]int{data}

	case *image.RGBA:
		precision = 8
		signed = false
		numComps = 3
		r := make([]int, width*height)
		g := make([]int, width*height)
		b := make([]int, width*height)
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				c := src.RGBAAt(x+bounds.Min.X, y+bounds.Min.Y)
				idx := y*width + x
				r[idx] = int(c.R)
				g[idx] = int(c.G)
				b[idx] = int(c.B)
			}
		}
		if opts.UseMCT {
			ApplyRCT([][]int{r, g, b})
		}
		components = [][]int{r, g, b}

	default:
		return ErrUnsupportedImage
	}

	// Build codestream
	var buf bytes.Buffer
	cw := NewCodestreamWriter(&buf)

	// Write SOC
	if err := cw.WriteSOC(); err != nil {
		return err
	}

	// Build and write SIZ
	compInfo := make([]ComponentInfo, numComps)
	for i := range compInfo {
		compInfo[i] = ComponentInfo{
			Precision: precision,
			Signed:    signed,
			XRsiz:     1,
			YRsiz:     1,
		}
	}
	siz := BuildSIZ(width, height, compInfo, opts.TileWidth, opts.TileHeight)
	if err := cw.WriteSIZ(siz); err != nil {
		return err
	}

	// Build and write COD
	useMCT := opts.UseMCT && numComps >= 3
	cod := BuildDefaultCOD(opts.DecompLevels, opts.NumLayers, opts.Progression, useMCT)
	if err := cw.WriteCOD(cod); err != nil {
		return err
	}

	// Build and write QCD
	qcd := BuildDefaultQCD(opts.DecompLevels, 2)
	if err := cw.WriteQCD(qcd); err != nil {
		return err
	}

	// Encode tiles
	numTiles := siz.NumTiles()
	for tileIdx := 0; tileIdx < numTiles; tileIdx++ {
		// Encode each component
		var tileData bytes.Buffer
		for compIdx := 0; compIdx < numComps; compIdx++ {
			te := NewTileEncoder(width, height, opts.DecompLevels, 64, 64)
			encoded, err := te.EncodeTile(components[compIdx])
			if err != nil {
				return err
			}
			tileData.Write(encoded)
		}

		// Write SOT
		tileLen := uint32(12 + tileData.Len()) // SOT(12) + tile data
		sot := &SOTMarker{
			TileIndex:    uint16(tileIdx),
			TilePartLen:  tileLen,
			TilePartIdx:  0,
			NumTileParts: 1,
		}
		if err := cw.WriteSOT(sot); err != nil {
			return err
		}

		// Write SOD
		if err := cw.WriteSOD(); err != nil {
			return err
		}

		// Write tile data
		if err := cw.WriteBytes(tileData.Bytes()); err != nil {
			return err
		}
	}

	// Write EOC
	if err := cw.WriteEOC(); err != nil {
		return err
	}

	if err := cw.Flush(); err != nil {
		return err
	}

	_, err := w.Write(buf.Bytes())
	return err
}

// Decode reads a JPEG 2000 image
func Decode(r io.Reader) (image.Image, error) {
	// Read all data
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	if len(data) < 4 {
		return nil, ErrInvalidFormat
	}

	// Check SOC marker
	if binary.BigEndian.Uint16(data[0:2]) != MarkerSOC {
		return nil, ErrInvalidFormat
	}

	// Parse codestream header
	cr := NewCodestreamReader(bytes.NewReader(data))
	if err := cr.ReadMainHeader(); err != nil {
		return nil, err
	}

	width := int(cr.SIZ.XSiz - cr.SIZ.XOsiz)
	height := int(cr.SIZ.YSiz - cr.SIZ.YOsiz)
	numComps := len(cr.SIZ.Components)
	precision := cr.SIZ.Components[0].Precision
	decompLevels := int(cr.COD.DecompLevels)
	useMCT := cr.COD.MCT != 0

	// Find SOT and decode tile
	pos := findMarker(data, MarkerSOT)
	if pos < 0 {
		return nil, ErrInvalidFormat
	}

	// Skip SOT header (marker + length + content = 12 bytes)
	pos += 2 // Skip marker
	sotLen := int(binary.BigEndian.Uint16(data[pos : pos+2]))
	pos += sotLen

	// Find SOD
	sodPos := findMarker(data[pos:], MarkerSOD)
	if sodPos < 0 {
		return nil, ErrInvalidFormat
	}
	pos += sodPos + 2 // Skip SOD marker

	// Decode tile data
	components := make([][]int, numComps)
	td := NewTileDecoder(width, height, decompLevels, 64, 64)

	for compIdx := 0; compIdx < numComps; compIdx++ {
		if pos >= len(data) {
			return nil, ErrDecodeFailed
		}

		// Calculate tile data length from new format
		// New format: width(2) + height(2) + coeffs(width*height*4)
		if pos+4 > len(data) {
			return nil, ErrDecodeFailed
		}
		tileW := int(data[pos])<<8 | int(data[pos+1])
		tileH := int(data[pos+2])<<8 | int(data[pos+3])
		tileDataLen := 4 + tileW*tileH*4

		decoded, err := td.DecodeTile(data[pos:])
		if err != nil {
			return nil, err
		}
		components[compIdx] = decoded
		pos += tileDataLen
	}

	// Apply inverse MCT if needed
	if useMCT && numComps >= 3 {
		ApplyInverseRCT(components)
	}

	// Create output image
	if numComps == 1 {
		if precision <= 8 {
			img := image.NewGray(image.Rect(0, 0, width, height))
			for y := 0; y < height; y++ {
				for x := 0; x < width; x++ {
					val := components[0][y*width+x]
					if val < 0 {
						val = 0
					}
					if val > 255 {
						val = 255
					}
					img.SetGray(x, y, color.Gray{Y: uint8(val)})
				}
			}
			return img, nil
		}
		img := image.NewGray16(image.Rect(0, 0, width, height))
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				val := components[0][y*width+x]
				if val < 0 {
					val = 0
				}
				if val > 65535 {
					val = 65535
				}
				img.SetGray16(x, y, color.Gray16{Y: uint16(val)})
			}
		}
		return img, nil
	}

	// RGB image
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			idx := y*width + x
			r := clamp(components[0][idx], 0, 255)
			g := clamp(components[1][idx], 0, 255)
			b := clamp(components[2][idx], 0, 255)
			img.SetRGBA(x, y, color.RGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 255})
		}
	}
	return img, nil
}

// DecodeConfig returns the image configuration without decoding
func DecodeConfig(r io.Reader) (image.Config, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return image.Config{}, err
	}

	if len(data) < 4 || binary.BigEndian.Uint16(data[0:2]) != MarkerSOC {
		return image.Config{}, ErrInvalidFormat
	}

	cr := NewCodestreamReader(bytes.NewReader(data))
	if err := cr.ReadMainHeader(); err != nil {
		return image.Config{}, err
	}

	width := int(cr.SIZ.XSiz - cr.SIZ.XOsiz)
	height := int(cr.SIZ.YSiz - cr.SIZ.YOsiz)
	numComps := len(cr.SIZ.Components)

	var colorModel color.Model
	if numComps == 1 {
		if cr.SIZ.Components[0].Precision <= 8 {
			colorModel = color.GrayModel
		} else {
			colorModel = color.Gray16Model
		}
	} else {
		colorModel = color.RGBAModel
	}

	return image.Config{
		Width:      width,
		Height:     height,
		ColorModel: colorModel,
	}, nil
}

func findMarker(data []byte, marker uint16) int {
	for i := 0; i < len(data)-1; i++ {
		if binary.BigEndian.Uint16(data[i:i+2]) == marker {
			return i
		}
	}
	return -1
}

func clamp(val, min, max int) int {
	if val < min {
		return min
	}
	if val > max {
		return max
	}
	return val
}

// Register format with image package
func init() {
	image.RegisterFormat("j2k", "\xff\x4f\xff\x51", Decode, DecodeConfig)
}
