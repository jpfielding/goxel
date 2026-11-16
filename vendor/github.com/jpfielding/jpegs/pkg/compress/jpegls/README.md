# JPEG-LS Codec

Pure Go implementation of JPEG-LS (ITU-T T.87 / ISO/IEC 14495-1): the LOCO-I
algorithm with context modeling, length-limited Golomb coding and run mode.

## Conformance scope

The encoder writes, and the decoder reads, conformant T.87 streams:

| | Encode | Decode |
|---|---|---|
| Single component (grayscale), precision 2..16 | yes | yes |
| Lossless and near-lossless (`NEAR` 0..min(255, MAXVAL/2)) | yes | yes |
| Run mode, single-bit stuffing, length-limited Golomb code | yes | yes |
| LSE ID=1 presets (MAXVAL, T1, T2, T3, RESET) | never written (defaults used) | yes |
| 3 components, planar (`ILV=0`, one scan per component) | no | yes |
| Line / sample interleaved scans (`ILV=1/2`) | no | `ErrUnsupported` |
| Restart markers inside a scan | no | `ErrUnsupported` (a bare `DRI` segment is skipped) |
| LSE mapping tables (ID 2-4), point transform, subsampling | no | `ErrUnsupported` / `ErrInvalidData` |

Verification:

- **dicos.rs (`pure_jpegls`, `Profile::T87`)** - the port's reference. Over
  5160 configurations (precision 2..16, six image patterns, ten geometries
  incl. 1xN / Nx1, NEAR 0..max) the Go encoder output is byte-identical to the
  Rust encoder, and each decoder reads the other's streams bit-exactly. Also
  3-component planar streams and LSE presets.
- **CharLS 2.4.2** - decodes every Go-encoded stream tried (2742), and Go
  decodes every CharLS-encoded stream (2736) to the same samples as CharLS.
  The scan data is byte-identical; above 12 bits CharLS additionally writes
  an LSE segment spelling out the defaults.
- `testdata/rust` and `testdata/charls` hold a small sample of those streams.
  File names (`<pattern>-<W>x<H>-p<precision>-n<near>.jls`) describe the
  source image, which the tests regenerate. `testdata/corrupt` holds inputs
  that crashed the dicos.rs decoder under fuzzing.

One known difference: for an LSE `MAXVAL` other than 2^precision-1, this
codec and dicos.rs follow the T.87 text (RANGE, LIMIT and the clamping all
derive from MAXVAL); CharLS 2.4.2 does not, and the two disagree on such
streams. The encoder never writes them.

## Usage

```go
import "github.com/jpfielding/jpegs/pkg/compress/jpegls"

// Lossless, precision derived from the image.
err := jpegls.Encode(w, img, nil)

// Near-lossless: every decoded sample is within Near of the source.
err = jpegls.Encode(w, img, &jpegls.Options{Near: 2})

// Explicit precision, e.g. DICOM BitsStored.
err = jpegls.Encode(w, img, &jpegls.Options{Precision: 12})

img, err := jpegls.Decode(r)
```

### Image types and precision

| Encode input | Precision when `Options.Precision == 0` |
|---|---|
| `*image.Gray` | 8 |
| `*image.Gray16` | bits needed by the largest sample, at least 9 |

| Decoded frame | Result |
|---|---|
| 1 component, precision <= 8 | `*image.Gray` |
| 1 component, precision > 8 | `*image.Gray16` |
| 3 components, precision <= 8 | `*image.RGBA` (opaque) |
| 3 components, precision > 8 | `*image.RGBA64` (opaque) |

Sample values are stored as coded - a 12-bit sample of 4095 decodes to
`Gray16{4095}`, not scaled to 65535.

The derived precision of a `*image.Gray16` never drops to 8 or below because
`Decode` would then return `*image.Gray`, and code reading it through
`color.Color.RGBA()` would see every sample multiplied by 257. Set
`Options.Precision` explicitly to write such a stream anyway.

### Errors and limits

Errors wrap `ErrInvalidData` (malformed or truncated stream, invalid encode
parameter), `ErrUnsupported` (valid but outside the scope above) or
`ErrImageTooLarge`.

`MaxDecodedBytes` (default 1 GiB; `<= 0` disables) caps width x height x
components x bytes-per-sample as declared by the frame header. A larger frame
is rejected before anything is allocated. Below the cap, sample buffers grow
with the scan data actually decoded, so a truncated or crafted stream does
not cost the memory its header claims. Decoding a complete frame needs about
two bytes per sample of working memory on top of the returned image.

## Legacy streams

Before it implemented T.87, this package wrote a private variant of JPEG-LS
under the same markers: T.81-style `0xFF 0x00` byte stuffing, no run mode, an
uncapped Golomb code and non-standard statistics. Conformant decoders
(CharLS, dicos.rs) reject those streams, and DICOM / DICOS files containing
them exist. The encoder no longer produces the format; `Decode` still reads
it:

1. The stream is decoded as T.87. The scan must yield every sample and be
   followed directly by EOI.
2. If that fails - and the frame is single-component, 8 bits or more, without
   an LSE segment, as the old encoder wrote - it is decoded with the frozen
   legacy decoder in `legacy.go`, under the same strict rules.
3. If both fail, the T.87 error is returned.

The headers of the two formats are identical, so the scan data is the only
evidence. For frames of realistic size that is decisive. For very small
8-bit frames (roughly below 16x16, especially flat ones) the same bit string
can be a complete stream in both formats; T.87 then wins and a legacy stream
would be mis-read. `TestLegacyDetection` pins both behaviours.

`testdata/legacy` holds legacy streams shared with dicos.rs
(`Profile::LegacyGo`). To check a DICOM file with legacy frames end to end:

```bash
JPEGLS_LEGACY_DICOM=/path/to/file.dcm go test -run TestLegacyDICOM -v ./pkg/compress/jpegls
```

## Fuzzing

```bash
GOMEMLIMIT=1GiB timeout 120 go test -run '^$' -fuzz FuzzDecode -fuzztime 30s -parallel 2 ./pkg/compress/jpegls
GOMEMLIMIT=1GiB timeout 120 go test -run '^$' -fuzz FuzzRoundTrip -fuzztime 30s -parallel 2 ./pkg/compress/jpegls
```

## Stream format

```
SOI   - Start of Image (0xFFD8)
SOF55 - Start of Frame (JPEG-LS) (0xFFF7)
LSE   - JPEG-LS preset parameters (optional, 0xFFF8)
SOS   - Start of Scan (0xFFDA)
[entropy-coded data]
EOI   - End of Image (0xFFD9)
```

## References

- ITU-T Rec. T.87 | ISO/IEC 14495-1 (JPEG-LS)
- Weinberger, Seroussi, Sapiro: "The LOCO-I Lossless Image Compression Algorithm"
