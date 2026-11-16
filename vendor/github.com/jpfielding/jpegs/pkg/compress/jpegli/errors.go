package jpegli

import (
	"errors"
	"fmt"
)

// Sentinel errors returned (wrapped) by Decode and Encode. Test with
// errors.Is; the wrapping message names the field or offset involved.
var (
	// ErrTruncated reports a stream that ends, or reaches a marker, before
	// all the data it declares. Truncation errors are *TruncatedError values
	// that match ErrTruncated with errors.Is.
	ErrTruncated = errors.New("jpegli: truncated stream")

	// ErrInvalidData reports a structurally invalid stream: bad marker
	// segments, an invalid Huffman code, restart markers out of place, or
	// entropy-coded data left over after the last sample.
	ErrInvalidData = errors.New("jpegli: invalid data")

	// ErrUnsupported reports a valid stream this codec does not handle, such
	// as a non-lossless SOF, more than one component, or a second scan.
	ErrUnsupported = errors.New("jpegli: unsupported")

	// ErrInvalidParameter reports a header field or encoder option outside
	// the range T.81 Table B.2/B.3 allows (precision, predictor, point
	// transform, dimensions, restart interval, sample values).
	ErrInvalidParameter = errors.New("jpegli: invalid parameter")
)

// TruncatedError reports where a truncated stream ended. Offset is the byte
// position in the input at which decoding ran out of data; Context names the
// phase (a marker segment, "entropy-coded scan", "EOI").
type TruncatedError struct {
	Offset  int
	Context string
}

func (e *TruncatedError) Error() string {
	return fmt.Sprintf("jpegli: truncated stream at offset %d (%s)", e.Offset, e.Context)
}

// Is reports whether target is ErrTruncated.
func (e *TruncatedError) Is(target error) bool { return target == ErrTruncated }
