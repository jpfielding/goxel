package volume

import "math"

// float16Bits converts f to IEEE 754 binary16 bits with round-half-up.
// Values beyond the half range become ±Inf; tiny values become subnormals or ±0.
// Used to pack volume textures as RGBA16F for WebGL2.
func float16Bits(f float32) uint16 {
	b := math.Float32bits(f)
	sign := uint16(b>>16) & 0x8000
	if b&0x7fffffff == 0 {
		return sign
	}
	exp := int((b>>23)&0xff) - 127 + 15
	mant := b & 0x7fffff
	switch {
	case (b>>23)&0xff == 0xff: // Inf or NaN
		if mant != 0 {
			return sign | 0x7e00
		}
		return sign | 0x7c00
	case exp >= 31:
		return sign | 0x7c00
	case exp <= 0:
		if exp < -10 {
			return sign
		}
		mant |= 0x800000
		shift := uint(14 - exp)
		h := uint16(mant >> shift)
		if (mant>>(shift-1))&1 != 0 {
			h++
		}
		return sign | h
	}
	h := sign | uint16(exp)<<10 | uint16(mant>>13)
	if mant&0x1000 != 0 {
		h++ // a carry into the exponent is still the correctly rounded value
	}
	return h
}

// downsample2x halves each axis of a dimX x dimY x dimZ volume with a 2x2x2
// box filter. Odd trailing samples are averaged with what is available.
func downsample2x(data []uint16, dimX, dimY, dimZ int) ([]uint16, int, int, int) {
	nx, ny, nz := max(1, dimX/2), max(1, dimY/2), max(1, dimZ/2)
	out := make([]uint16, nx*ny*nz)
	for z := 0; z < nz; z++ {
		for y := 0; y < ny; y++ {
			for x := 0; x < nx; x++ {
				var sum, n uint32
				for dz := 0; dz < 2; dz++ {
					sz := 2*z + dz
					if sz >= dimZ {
						continue
					}
					for dy := 0; dy < 2; dy++ {
						sy := 2*y + dy
						if sy >= dimY {
							continue
						}
						row := (sz*dimY + sy) * dimX
						for dx := 0; dx < 2; dx++ {
							sx := 2*x + dx
							if sx >= dimX {
								continue
							}
							sum += uint32(data[row+sx])
							n++
						}
					}
				}
				out[(z*ny+y)*nx+x] = uint16(sum / n)
			}
		}
	}
	return out, nx, ny, nz
}
