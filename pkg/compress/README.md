# Compression Codecs

This package provides pure Go implementations of lossless image compression formats used in DICOS/DICOM imaging.

## Supported Formats

| Package | Format | DICOM Transfer Syntax | Description |
|---------|--------|----------------------|-------------|
| [jpeg2k](jpeg2k/) | JPEG 2000 | `1.2.840.10008.1.2.4.90` | Wavelet-based, excellent compression |
| [jpegli](jpegli/) | JPEG Lossless | `1.2.840.10008.1.2.4.70` | Traditional DPCM-based |
| [jpegls](jpegls/) | JPEG-LS | `1.2.840.10008.1.2.4.80/81` | LOCO-I algorithm, very efficient |
| [rle](rle/) | RLE (PackBits) | `1.2.840.10008.1.2.5` | Run-length encoding |

## Feature Comparison

| Feature | jpeg2k | jpegli | jpegls | rle |
|---------|--------|--------|--------|-----|
| Lossless | Yes | Yes | Yes | Yes |
| Near-lossless | No | No | Yes | No |
| 8-bit Gray | Yes | Yes | Yes | Yes |
| 16-bit Gray | Yes | Yes | Yes | Yes |
| RGB | Yes | No | No | No |
| Compression Ratio | Excellent | Good | Excellent | Fair |
| Speed | Moderate | Fast | Fast | Very Fast |

## Usage

All codecs follow the same API pattern:

```go
// Encoding
err := codec.Encode(writer, image, options)

// Decoding
img, err := codec.Decode(reader)
```

See individual package READMEs for detailed usage and options.

## DICOS Integration

These codecs are automatically used by the `pkg/dicos` package when decoding compressed pixel data. The appropriate codec is selected based on the Transfer Syntax UID in the DICOS file:

```go
import "gitlab.ses.psdo.leidos.com/.../pkg/dicos"

// Automatically handles compressed frames
volume, err := dicos.DecodeVolume(dataset)
```

## Pure Go

All implementations are pure Go with no CGO dependencies, enabling:
- Cross-compilation to any Go-supported platform
- No external library dependencies
- Consistent behavior across platforms
