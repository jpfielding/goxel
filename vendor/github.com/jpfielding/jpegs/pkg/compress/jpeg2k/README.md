# JPEG 2000 Codec

Pure Go implementation of JPEG 2000 Part-1 (ITU-T T.800 / ISO/IEC 15444-1) encoder and decoder.

## Features

- **Lossless compression** using 5/3 reversible discrete wavelet transform (DWT)
- **Multi-component support**: Gray, Gray16, RGB images
- **Reversible Color Transform (RCT)** for RGB images
- **DICOS/DICOM compatible**: Transfer Syntax `1.2.840.10008.1.2.4.90`
- **Pure Go**: No CGO dependencies

## Usage

### Encoding

```go
import "github.com/jpfielding/goxel/pkg/compress/jpeg2k"

// Encode with default options
var buf bytes.Buffer
err := jpeg2k.Encode(&buf, img, nil)

// Encode with custom options
opts := &jpeg2k.Options{
    DecompLevels: 5,    // DWT decomposition levels
    NumLayers:    1,    // Quality layers
    Progression:  jpeg2k.ProgressionLRCP,
    UseMCT:       true, // Use color transform for RGB
}
err := jpeg2k.Encode(&buf, img, opts)
```

### Decoding

```go
import "github.com/jpfielding/goxel/pkg/compress/jpeg2k"

// Decode image
img, err := jpeg2k.Decode(reader)

// Get image config without full decode
config, err := jpeg2k.DecodeConfig(reader)
```

### Image Format Registration

The package registers with Go's image package for automatic format detection:

```go
import (
    "image"
    _ "github.com/jpfielding/goxel/pkg/compress/jpeg2k"
)

// Auto-detects JPEG 2000 by SOC marker (0xFF4F)
img, format, err := image.Decode(reader)
```

## Supported Image Types

| Input Type | Precision | Components |
|------------|-----------|------------|
| `*image.Gray` | 8-bit | 1 |
| `*image.Gray16` | 16-bit | 1 |
| `*image.RGBA` | 8-bit | 3 (RGB) |

## Architecture

```
jpeg2k/
├── jpeg2k.go      # Public API (Encode, Decode, DecodeConfig)
├── markers.go     # JPEG 2000 marker constants and structures
├── codestream.go  # Codestream reader/writer
├── bitstream.go   # Bit-level I/O utilities
├── dwt.go         # 5/3 reversible wavelet transform
├── tile.go        # Tile encoding/decoding
├── rct.go         # Reversible Color Transform
├── mq.go          # MQ arithmetic coder
├── ebcot.go       # EBCOT block coding (Tier-1)
└── *_test.go      # Unit tests
```

## JPEG 2000 Codestream Format

```
SOC  - Start of Codestream (0xFF4F)
SIZ  - Image and Tile Size
COD  - Coding Style Default
QCD  - Quantization Default
SOT  - Start of Tile-part
SOD  - Start of Data
[tile data]
EOC  - End of Codestream (0xFFD9)
```

## References

- ITU-T Rec. T.800 | ISO/IEC 15444-1 (JPEG 2000 Part-1)
- DICOM Transfer Syntax: 1.2.840.10008.1.2.4.90 (JPEG 2000 Lossless)
