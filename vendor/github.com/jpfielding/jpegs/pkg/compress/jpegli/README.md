# JPEG Lossless Codec (jpegli)

Pure Go implementation of JPEG Lossless (ITU-T T.81 Annex H) encoder and decoder.

## Features

- **Lossless compression** using differential pulse code modulation (DPCM)
- **Predictors 1-7** supported for optimal compression
- **8-bit and 16-bit** grayscale images
- **DICOS/DICOM compatible**: Transfer Syntax `1.2.840.10008.1.2.4.70`
- **Pure Go**: No CGO dependencies

## Usage

### Encoding

```go
import "github.com/jpfielding/goxel/pkg/compress/jpegli"

// Encode with default options (predictor 1)
err := jpegli.Encode(writer, img, nil)

// Encode with custom predictor
opts := &jpegli.Options{
    Predictor:      1,  // 1-7 (see predictor table below)
    PointTransform: 0,  // 0 for lossless
}
err := jpegli.Encode(writer, img, opts)
```

### Decoding

```go
import "github.com/jpfielding/goxel/pkg/compress/jpegli"

img, err := jpegli.Decode(reader)
```

## Predictors

| ID | Formula | Description |
|----|---------|-------------|
| 1 | Ra | Previous sample (left) |
| 2 | Rb | Sample above |
| 3 | Rc | Sample above-left (diagonal) |
| 4 | Ra + Rb - Rc | Linear interpolation |
| 5 | Ra + (Rb - Rc)/2 | Weighted average |
| 6 | Rb + (Ra - Rc)/2 | Weighted average |
| 7 | (Ra + Rb)/2 | Average of left and above |

Where:
- Ra = sample immediately to the left
- Rb = sample immediately above
- Rc = sample diagonally above-left

## Supported Image Types

| Type | Precision |
|------|-----------|
| `*image.Gray` | 8-bit |
| `*image.Gray16` | 16-bit |

## JPEG Lossless Stream Format

```
SOI  - Start of Image (0xFFD8)
SOF3 - Start of Frame (Lossless, Huffman) (0xFFC3)
DHT  - Define Huffman Table (0xFFC4)
SOS  - Start of Scan (0xFFDA)
[entropy-coded data]
EOI  - End of Image (0xFFD9)
```

## References

- ITU-T Rec. T.81 Annex H (JPEG Lossless Mode)
- DICOM Transfer Syntax: 1.2.840.10008.1.2.4.70 (JPEG Lossless, First-Order Prediction)
