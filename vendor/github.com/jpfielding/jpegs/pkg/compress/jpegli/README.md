# JPEG Lossless Codec (jpegli)

Pure Go implementation of JPEG Lossless (ITU-T T.81 Annex H) encoder and decoder.

## Features

- **Lossless compression** using differential pulse code modulation (DPCM)
- **Predictors 1-7** supported for optimal compression
- **8-bit and 16-bit** grayscale images
- **Pure Go**: No CGO dependencies

## Usage

### Encoding

```go
import "github.com/jpfielding/jpegs/pkg/compress/jpegli"

// Encode with default options (predictor 1)
err := jpegli.Encode(writer, img, nil)

// Encode with explicit options (invalid values return ErrInvalidParameter)
opts := &jpegli.Options{
    Predictor:           1,  // 1-7 (see predictor table below); 0 = 1
    PointTransform:      0,  // 0 for lossless; samples are shifted right by Pt
    Precision:           12, // 2-16; 0 = 8 for *image.Gray, 16 otherwise
    RestartIntervalRows: 0,  // RSTn every N rows; 0 = none
}
err := jpegli.Encode(writer, img, opts)
```

### Decoding

```go
import "github.com/jpfielding/jpegs/pkg/compress/jpegli"

img, err := jpegli.Decode(reader)
```

Decoding is strict: a stream that ends before every sample is decoded, or
that does not end with EOI, returns an error wrapping `ErrTruncated` (never a
partial image). Errors wrap `ErrTruncated`, `ErrInvalidData`,
`ErrUnsupported`, `ErrInvalidParameter` or `ErrImageTooLarge`.

16-bit streams written by jpegs v0.2.0 and earlier, which appended 16 bits
after the SSSS=16 code, are recognised and decoded with a frozen legacy
decoder (`legacy.go`).

## Predictors

| ID | Formula | Description |
|----|---------|-------------|
| 1 | Ra | Previous sample (left) |
| 2 | Rb | Sample above |
| 3 | Rc | Sample above-left (diagonal) |
| 4 | Ra + Rb - Rc | Linear interpolation |
| 5 | Ra + ((Rb - Rc) >> 1) | Weighted average |
| 6 | Rb + ((Ra - Rc) >> 1) | Weighted average |
| 7 | (Ra + Rb) >> 1 | Average of left and above |

`>> 1` is an arithmetic shift, as in libjpeg-turbo and libjpeg; it rounds
negative odd values down where truncating `/2` rounds them up. Versions up to
v0.2.0 used `/2`.

Where:
- Ra = sample immediately to the left
- Rb = sample immediately above
- Rc = sample diagonally above-left

## Supported Image Types

| Type | Precision |
|------|-----------|
| `*image.Gray` | 8-bit (default) |
| `*image.Gray16` | 16-bit (default) |

Decode accepts precision 2-16, one component, predictors 1-7, any point
transform, any DC Huffman table and restart intervals that are a multiple of
the row width.

## JPEG Lossless Stream Format

```
SOI  - Start of Image (0xFFD8)
SOF3 - Start of Frame (Lossless, Huffman) (0xFFC3)
DHT  - Define Huffman Table (0xFFC4)
DRI  - Define Restart Interval (0xFFDD, optional)
SOS  - Start of Scan (0xFFDA)
[entropy-coded data, RSTn markers between restart intervals]
EOI  - End of Image (0xFFD9)
```

## References

- ITU-T Rec. T.81 Annex H (JPEG Lossless Mode)
