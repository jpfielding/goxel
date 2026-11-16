# JPEG 2000 Codec

Pure Go lossless JPEG 2000 Part 1 (ITU-T T.800 / ISO/IEC 15444-1) codec,
ported from dicos.rs `crates/jpeg2k` (`pure_jpeg2k` 2.x).

## Conformance scope

Raw codestreams only (no JP2 box). Encode and decode support:

- one tile, one tile-part, one component, **unsigned 16-bit** samples
- reversible 5/3 DWT (0..32 levels), LRCP, one quality layer
- default precincts (each resolution at most 32768 wide and tall)
- code-block exponents 2..10 with a sum of at most 12, code-block style 0
- zero image and tile origins

Everything else that is legal T.800 is rejected with `ErrUnsupported`
(multiple tiles or components, other precisions, signed samples, MCT, 9/7,
other progressions, layers, `SOP`/`EPH`, user precincts, `COC`, `QCC`,
`RGN`, `POC`, `PPM`, `PPT`, `PLM`, `PLT`). Malformed streams return
`ErrInvalidData`; the tile-part must end exactly at `EOC`, and every payload
byte must belong to a packet.

`Encode` accepts only `*image.Gray16`; other image types return
`ErrUnsupported`.

Verification (OpenJPEG 2.5.4, dicos.rs 2.1.0 at `1ebf683`):

- **dicos.rs** - over 13,200 configurations (11 widths × 8 heights from 1 to
  257, 0-8 levels, five code-block shapes, six patterns) the Go encoder's
  output is byte-identical to the Rust encoder's, and each decoder reads the
  other's streams exactly. The MQ/tier-1 coder reproduces dicos.rs's
  OpenJPEG code-block vector and 8x8 golden byte for byte.
- **OpenJPEG** - `opj_decompress` reads all 13,200 Go streams to the exact
  source pixels, and Go decodes 1,512 `opj_compress` streams (default and
  `-n`/`-b` variants over the same images) exactly. `testdata/interop`
  holds four OpenJPEG streams; `JPEG2K_INTEROP=1 go test` re-runs the
  `opj_decompress` direction when the tool is on `PATH`.

## Usage

```go
var buf bytes.Buffer
err := jpeg2k.Encode(&buf, gray16, nil) // nil = DefaultOptions()

opts := &jpeg2k.Options{DecompLevels: 3, CodeBlockWidthExp: 5, CodeBlockHeightExp: 5}
err = jpeg2k.Encode(&buf, gray16, opts)

img, err := jpeg2k.Decode(r)                              // LegacyAuto
img, err = jpeg2k.DecodeWithPolicy(r, jpeg2k.StandardOnly) // untrusted data
cfg, err := jpeg2k.DecodeConfig(r)
```

The package no longer registers itself with `image.RegisterFormat`; call
`Decode` directly (or register it yourself).

## Legacy streams

Before v0.3.0 this package wrote a private format that only looked like
JPEG 2000: raw big-endian int32 5/3 coefficients (truncating division, no DC
shift), `SOD = 0xFFD3`, all-zero QCD exponents, `Psot = 12 + len`, and
8-bit gray, 16-bit gray or RGB (RCT) images. dicos.rs 1.0.0 wrote the same
for 16-bit gray with `Psot = 14 + len`. No conformant decoder can read these.

`Decode` (`LegacyAuto`) still decodes them through a frozen decoder
(`legacy.go`) when a stream carries the legacy fingerprint: legacy `SOD`,
all-zero QCD exponents and a payload of exactly the legacy size, checked
before any packet is decoded. They decode to `*image.Gray`,
`*image.Gray16` or `*image.RGBA` as written. `StandardOnly` rejects them,
`LegacyOnly` accepts nothing else. `testdata/legacy` pins both producers'
output (`go-*.j2c` from jpegs v0.2.0, `rs-*.j2c` from dicos.rs 1.0.0).
Note that dicos.rs 2.x itself reads only the `rs-*` files: it rejects Go's
`Psot` and its 8-bit and RGB files.

## Limits

`MaxDecodedBytes` (default 1 GiB) caps the coefficient buffer
(width × height × 4 bytes; 8 per sample per component for legacy streams)
and, separately, the estimated per-code-block state (128 bytes per block),
before anything sized by the header is allocated. Lower it for untrusted
input.

## Layout

| File | Contents | dicos.rs source |
|---|---|---|
| `jpeg2k.go` | `Encode`, `Decode`, `DecodeWithPolicy`, `DecodeConfig`, `Options` | `lib.rs`, `codestream.rs` |
| `codestream.go` | marker constants, header parser, support matrix, writer | `codestream.rs`, `markers.rs` |
| `tile.go` | tile-component pipeline (DWT, tier-1, LRCP packets) | `tile.rs` |
| `geometry.go` | resolution, band and code-block geometry (B.5-B.7) | `geometry.rs` |
| `packet.go` | packet headers (B.10) | `packet.rs` |
| `tagtree.go` | tag trees (B.10.2) | `tagtree.rs` |
| `ebcot.go` | EBCOT tier-1 (Annex D) | `ebcot.rs` |
| `mq.go` | MQ arithmetic coder (Annex C) | `mq.rs` |
| `dwt.go` | reversible 5/3 DWT (Annex F) | `dwt.rs` |
| `legacy.go` | frozen legacy decoder, `LegacyPolicy` | `legacy.rs` |
| `limits.go` | `MaxDecodedBytes` | — |

Deliberate departures from dicos.rs: `εb + G − 1 < 0` in QCD is rejected
instead of underflowing; trailing payload bytes after the last packet are
rejected; per-code-block state is bounded by `MaxDecodedBytes`; legacy
streams must use `SOD = 0xFFD3`, and conformant ones `0xFF93`; Go's legacy
`Psot = 12 + len` and 8-bit/RGB legacy files are readable.

## References

- ITU-T Rec. T.800 | ISO/IEC 15444-1
- DICOM transfer syntax `1.2.840.10008.1.2.4.90` (`TransferSyntaxUID`)
