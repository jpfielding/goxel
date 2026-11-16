package rle

import "errors"

// Sentinel errors returned (wrapped) by Decode and Encode. Test with
// errors.Is; the wrapping message carries the offset or value involved.
var (
	// ErrTruncated reports a header or PackBits segment that ends before
	// all the data it declares.
	ErrTruncated = errors.New("rle: truncated stream")

	// ErrInvalidData reports a structurally invalid stream: a bad segment
	// count, or a segment offset that points into the header, past the end
	// of the data, or before the previous segment.
	ErrInvalidData = errors.New("rle: invalid data")

	// ErrUnsupported reports a valid stream this codec does not handle,
	// such as RGB (3 segments) or 32-bit samples, or an unsupported image
	// type on encode.
	ErrUnsupported = errors.New("rle: unsupported")

	// ErrDimensionMismatch reports a segment whose decoded length differs
	// from width × height.
	ErrDimensionMismatch = errors.New("rle: decoded size does not match dimensions")
)
