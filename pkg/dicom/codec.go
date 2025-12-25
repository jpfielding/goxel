package dicom

import (
	"bytes"
	"image"
	"io"

	"github.com/jpfielding/goxel/pkg/dicom/transfer"
	"github.com/jpfielding/jpegs/pkg/compress/jpeg2k"
	"github.com/jpfielding/jpegs/pkg/compress/jpegli"
	"github.com/jpfielding/jpegs/pkg/compress/jpegls"
	"github.com/jpfielding/jpegs/pkg/compress/rle"
)

// Codec defines the interface for DICOM pixel data compression.
type Codec interface {
	// Encode compresses an image to the writer.
	Encode(w io.Writer, img image.Image) error
	// Decode decompresses data from bytes to an image.
	// width/height provided for codecs that need them (RLE).
	Decode(data []byte, width, height int) (image.Image, error)
	// Name returns the codec identifier (e.g., "jpeg-ls").
	Name() string
	// TransferSyntaxUID returns the DICOM transfer syntax for this codec.
	TransferSyntaxUID() string
}

// Predefined codec implementations.
var (
	CodecJPEGLS   Codec = &jpegLSCodec{}
	CodecJPEGLi   Codec = &jpegLiCodec{}
	CodecRLE      Codec = &rleCodec{}
	CodecJPEG2000 Codec = &jpeg2kCodec{}
)

// codecsByName maps codec name strings to Codec implementations.
var codecsByName = map[string]Codec{
	"jpeg-ls":   CodecJPEGLS,
	"jpegls":    CodecJPEGLS,
	"jpeg-li":   CodecJPEGLi,
	"jpegli":    CodecJPEGLi,
	"rle":       CodecRLE,
	"jpeg-2000": CodecJPEG2000,
	"jpeg2000":  CodecJPEG2000,
	"j2k":       CodecJPEG2000,
}

// codecsByTransferSyntax maps transfer syntax UIDs to Codec implementations.
var codecsByTransferSyntax = map[string]Codec{
	string(transfer.JPEGLSLossless):         CodecJPEGLS,
	string(transfer.JPEGLSNearLossless):     CodecJPEGLS,
	string(transfer.JPEGLosslessFirstOrder): CodecJPEGLi,
	string(transfer.JPEGLossless):           CodecJPEGLi,
	string(transfer.RLELossless):            CodecRLE,
	string(transfer.JPEG2000Lossless):       CodecJPEG2000,
	string(transfer.JPEG2000):               CodecJPEG2000,
}

// CodecByName returns a Codec by its name, or nil if not found.
func CodecByName(name string) Codec {
	return codecsByName[name]
}

// CodecByTransferSyntax returns a Codec for the given transfer syntax UID, or nil if not found.
func CodecByTransferSyntax(ts string) Codec {
	return codecsByTransferSyntax[ts]
}

// jpegLSCodec implements Codec for JPEG-LS compression.
type jpegLSCodec struct{}

func (c *jpegLSCodec) Encode(w io.Writer, img image.Image) error {
	return jpegls.Encode(w, img, nil)
}

func (c *jpegLSCodec) Decode(data []byte, width, height int) (image.Image, error) {
	return jpegls.Decode(bytes.NewReader(data))
}

func (c *jpegLSCodec) Name() string {
	return "jpeg-ls"
}

func (c *jpegLSCodec) TransferSyntaxUID() string {
	return string(transfer.JPEGLSLossless)
}

// jpegLiCodec implements Codec for JPEG Lossless compression.
type jpegLiCodec struct{}

func (c *jpegLiCodec) Encode(w io.Writer, img image.Image) error {
	return jpegli.Encode(w, img, nil)
}

func (c *jpegLiCodec) Decode(data []byte, width, height int) (image.Image, error) {
	return jpegli.Decode(bytes.NewReader(data))
}

func (c *jpegLiCodec) Name() string {
	return "jpeg-li"
}

func (c *jpegLiCodec) TransferSyntaxUID() string {
	return string(transfer.JPEGLosslessFirstOrder)
}

// rleCodec implements Codec for RLE compression.
type rleCodec struct{}

func (c *rleCodec) Encode(w io.Writer, img image.Image) error {
	return rle.Encode(w, img)
}

func (c *rleCodec) Decode(data []byte, width, height int) (image.Image, error) {
	return rle.Decode(data, width, height)
}

func (c *rleCodec) Name() string {
	return "rle"
}

func (c *rleCodec) TransferSyntaxUID() string {
	return string(transfer.RLELossless)
}

// jpeg2kCodec implements Codec for JPEG 2000 compression.
type jpeg2kCodec struct{}

func (c *jpeg2kCodec) Encode(w io.Writer, img image.Image) error {
	return jpeg2k.Encode(w, img, nil)
}

func (c *jpeg2kCodec) Decode(data []byte, width, height int) (image.Image, error) {
	return jpeg2k.Decode(bytes.NewReader(data))
}

func (c *jpeg2kCodec) Name() string {
	return "jpeg-2000"
}

func (c *jpeg2kCodec) TransferSyntaxUID() string {
	return string(transfer.JPEG2000Lossless)
}
