# DICOM RLE Codec

Pure Go implementation of DICOM RLE (Run Length Encoding) using PackBits compression.

## Features

- **Lossless compression** using PackBits algorithm
- **8-bit and 16-bit** grayscale images
- **Byte-plane separation** for 16-bit images (improved compression)
- **DICOS/DICOM compatible**: Transfer Syntax `1.2.840.10008.1.2.5`
- **Pure Go**: No CGO dependencies

## Usage

### Encoding

```go
import "github.com/jpfielding/goxel/pkg/compress/rle"

err := rle.Encode(writer, img)
```

### Decoding

```go
import "github.com/jpfielding/goxel/pkg/compress/rle"

// Width and height must be provided (not stored in RLE stream)
img, err := rle.Decode(data, width, height)
```

## RLE Algorithm

DICOM RLE uses the PackBits algorithm:

| Control Byte | Action |
|--------------|--------|
| 0 to 127 | Copy next (n+1) bytes literally |
| -1 to -127 | Repeat next byte (-n+1) times |
| -128 | No operation (padding) |

### 16-bit Image Handling

For 16-bit grayscale images, pixels are split into two segments:
1. **Segment 1**: High bytes of all pixels
2. **Segment 2**: Low bytes of all pixels

This byte-plane separation improves compression because adjacent high bytes often have similar values.

## Supported Image Types

| Type | Segments |
|------|----------|
| `*image.Gray` | 1 (8-bit pixels) |
| `*image.Gray16` | 2 (high/low byte planes) |

## DICOM RLE Format

```
Header (64 bytes):
  Bytes 0-3:   Number of segments (uint32 LE)
  Bytes 4-63:  Up to 15 segment offsets (uint32 LE each)

Segments:
  [PackBits compressed data for each segment]
```

## References

- DICOM Part 5, Annex G (RLE Compression)
- DICOM Transfer Syntax: 1.2.840.10008.1.2.5 (RLE Lossless)
- Apple PackBits compression format
