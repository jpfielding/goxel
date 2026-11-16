package jpeg2k

import (
	"fmt"
)

// Conformant tile-component pipeline: 5/3 DWT <-> geometry <-> EBCOT tier-1
// <-> LRCP tier-2 packets. Ported from dicos.rs crates/jpeg2k/src/tile.rs.
// With one tile, component, layer and precinct per band, the packet order is
// resolution ascending, one packet per resolution.
//
// mbFor(kind, level) supplies Mb per band, where level is the decomposition
// level (NL for LL, NL..1 for detail bands), the key QCD orders exponents by.

// decompositionLevel is NL for r == 0 and NL-r+1 otherwise.
func decompositionLevel(r, levels int) int {
	if r == 0 {
		return levels
	}
	return levels - r + 1
}

// ensureSinglePrecinct rejects geometries whose resolutions span more than
// one precinct; the pipeline writes one packet per resolution.
func ensureSinglePrecinct(geom []resolution) error {
	for _, r := range geom {
		if r.precinctsW*r.precinctsH > 1 {
			return fmt.Errorf("%w: multi-precinct images (a resolution wider or taller than 32768)", ErrUnsupported)
		}
	}
	return nil
}

// cbOrigin is a code-block's top-left offset in the packed Mallat buffer.
func cbOrigin(b *band, cb *codeBlockGeom) (x, y int) {
	return b.xOff + cb.rect.x0, b.yOff + cb.rect.y0
}

func gather(buf []int32, stride int, b *band, cb *codeBlockGeom) []int32 {
	bw, bh := cb.rect.width(), cb.rect.height()
	x0, y0 := cbOrigin(b, cb)
	v := make([]int32, bw*bh)
	for yy := range bh {
		src := (y0+yy)*stride + x0
		copy(v[yy*bw:(yy+1)*bw], buf[src:src+bw])
	}
	return v
}

func scatter(buf []int32, stride int, b *band, cb *codeBlockGeom, coeffs []int32) {
	bw, bh := cb.rect.width(), cb.rect.height()
	x0, y0 := cbOrigin(b, cb)
	for yy := range bh {
		dst := (y0+yy)*stride + x0
		copy(buf[dst:dst+bw], coeffs[yy*bw:(yy+1)*bw])
	}
}

// encodeTile codes one w x h tile-component (row-major, DC-shifted) as
// concatenated LRCP packets.
func encodeTile(component []int32, w, h, levels, xcb, ycb int, mbFor func(bandKind, int) int) ([]byte, error) {
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("%w: tile has a zero dimension", ErrInvalidData)
	}
	if len(component) != w*h {
		return nil, fmt.Errorf("%w: %d samples for a %dx%d tile", ErrInvalidData, len(component), w, h)
	}
	geom := buildGeometry(w, h, levels, xcb, ycb)
	if err := ensureSinglePrecinct(geom); err != nil {
		return nil, err
	}
	buf := make([]int32, len(component))
	copy(buf, component)
	forwardDWT(buf, w, h, levels)

	var out []byte
	for _, res := range geom {
		level := decompositionLevel(res.r, levels)
		states := make([]*precinctState, len(res.bands))
		mbs := make([]int, len(res.bands))
		blocks := make([][]*codedBlock, len(res.bands))
		for bi := range res.bands {
			b := &res.bands[bi]
			states[bi] = newPrecinctState(b.gridW, b.gridH)
			mbs[bi] = mbFor(b.kind, level)
			blocks[bi] = make([]*codedBlock, len(b.cbs))
			for k := range b.cbs {
				cb := &b.cbs[k]
				coded := encodeCodeBlock(gather(buf, w, b, cb), cb.rect.width(), cb.rect.height(), b.kind)
				if coded.bitplanes > mbs[bi] {
					return nil, fmt.Errorf("%w: code-block needs %d bit-planes but Mb is %d (band %v)", ErrInvalidData, coded.bitplanes, mbs[bi], b.kind)
				}
				if coded.bitplanes > 0 {
					blocks[bi][k] = &coded
				}
			}
		}
		var err error
		if out, err = writePacket(states, res.bands, blocks, mbs, out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// decodeTile reconstructs a w x h tile-component from LRCP packets. The
// caller has already checked w*h against MaxDecodedBytes. Every byte of
// bitstream must belong to a packet.
func decodeTile(bitstream []byte, w, h, levels, xcb, ycb int, mbFor func(bandKind, int) int) ([]int32, error) {
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("%w: tile has a zero dimension", ErrInvalidData)
	}
	// Every packet is at least one byte: reject a short payload before
	// allocating anything sized by the header.
	if len(bitstream) < levels+1 {
		return nil, fmt.Errorf("%w: %d-byte tile cannot hold %d packets", ErrInvalidData, len(bitstream), levels+1)
	}
	geom := buildGeometry(w, h, levels, xcb, ycb)
	if err := ensureSinglePrecinct(geom); err != nil {
		return nil, err
	}
	buf := make([]int32, w*h)
	pos := 0
	for _, res := range geom {
		level := decompositionLevel(res.r, levels)
		states := make([]*precinctState, len(res.bands))
		mbs := make([]int, len(res.bands))
		for bi := range res.bands {
			states[bi] = newPrecinctState(res.bands[bi].gridW, res.bands[bi].gridH)
			mbs[bi] = mbFor(res.bands[bi].kind, level)
		}
		contribs, hdr, err := readPacket(states, res.bands, mbs, bitstream[pos:])
		if err != nil {
			return nil, err
		}
		body := pos + hdr
		for bi := range res.bands {
			b := &res.bands[bi]
			for k := range b.cbs {
				c := contribs[bi][k]
				if c == nil {
					continue // buf is already zero
				}
				cb := &b.cbs[k]
				nb := mbs[bi] - c.zeroBitplanes
				switch {
				case c.zeroBitplanes > mbs[bi]:
					return nil, fmt.Errorf("%w: %d zero bit-planes exceed Mb %d", ErrInvalidData, c.zeroBitplanes, mbs[bi])
				case nb == 0:
					return nil, fmt.Errorf("%w: included code-block with no magnitude bit-planes", ErrInvalidData)
				case c.passes > 3*nb-2:
					return nil, fmt.Errorf("%w: %d passes exceed 3*%d-2", ErrInvalidData, c.passes, nb)
				case c.length > len(bitstream)-body:
					return nil, fmt.Errorf("%w: code-block body truncated", ErrInvalidData)
				}
				coeffs, err := decodeCodeBlock(bitstream[body:body+c.length], cb.rect.width(), cb.rect.height(), b.kind, nb, c.passes)
				if err != nil {
					return nil, err
				}
				body += c.length
				scatter(buf, w, b, cb, coeffs)
			}
		}
		pos = body
	}
	if pos != len(bitstream) {
		return nil, fmt.Errorf("%w: %d bytes after the last packet", ErrInvalidData, len(bitstream)-pos)
	}
	inverseDWT(buf, w, h, levels)
	return buf, nil
}
