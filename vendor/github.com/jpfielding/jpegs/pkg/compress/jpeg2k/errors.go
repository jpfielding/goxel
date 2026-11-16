package jpeg2k

import "errors"

// Sentinel errors. Errors from the conformant codec wrap one of these or
// ErrImageTooLarge (see limits.go).
var (
	// ErrInvalidData reports a malformed or truncated codestream, or an
	// invalid encode parameter.
	ErrInvalidData = errors.New("jpeg2k: invalid data")
	// ErrUnsupported reports a well-formed codestream (or image type) outside
	// the supported T.800 profile.
	ErrUnsupported = errors.New("jpeg2k: unsupported")
)
