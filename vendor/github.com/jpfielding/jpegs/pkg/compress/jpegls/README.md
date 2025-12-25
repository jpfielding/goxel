# JPEG-LS Codec

Pure Go implementation of JPEG-LS (ITU-T T.87 / ISO/IEC 14495-1) encoder and decoder.

## Features

- **Lossless and near-lossless** compression
- **Context-based adaptive prediction** using LOCO-I algorithm
- **Golomb-Rice entropy coding**
- **8-bit and 16-bit** grayscale images
- **DICOS/DICOM compatible**: Transfer Syntax `1.2.840.10008.1.2.4.80` (lossless) and `.81` (near-lossless)
- **Pure Go**: No CGO dependencies

## Usage

### Encoding

```go
import "github.com/jpfielding/goxel/pkg/compress/jpegls"

// Lossless encoding (Near = 0)
err := jpegls.Encode(writer, img, nil)

// Near-lossless encoding
opts := &jpegls.Options{
    Near: 2,  // Maximum pixel error (0 = lossless)
}
err := jpegls.Encode(writer, img, opts)
```

### Decoding

```go
import "github.com/jpfielding/goxel/pkg/compress/jpegls"

img, err := jpegls.Decode(reader)
```

## JPEG-LS Algorithm

JPEG-LS uses the LOCO-I (LOw COmplexity LOssless COmpression for Images) algorithm:

1. **Edge detection** using local gradients
2. **Context modeling** based on neighboring pixels
3. **Adaptive prediction** using median edge detector (MED)
4. **Golomb-Rice coding** for prediction residuals
5. **Run-length encoding** for uniform regions

### Prediction Context

```
      c  b  d
      a  x
```

Where x is the current sample being encoded, and a, b, c, d are neighboring samples.

## Supported Image Types

| Type | Precision |
|------|-----------|
| `*image.Gray` | 8-bit |
| `*image.Gray16` | 16-bit |

## JPEG-LS Stream Format

```
SOI   - Start of Image (0xFFD8)
SOF55 - Start of Frame (JPEG-LS) (0xFFF7)
LSE   - JPEG-LS Preset Parameters (optional)
SOS   - Start of Scan (0xFFDA)
[entropy-coded data]
EOI   - End of Image (0xFFD9)
```

## References

- ITU-T Rec. T.87 | ISO/IEC 14495-1 (JPEG-LS)
- DICOM Transfer Syntaxes:
  - `1.2.840.10008.1.2.4.80` (JPEG-LS Lossless)
  - `1.2.840.10008.1.2.4.81` (JPEG-LS Near-Lossless)
