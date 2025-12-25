package jpeg2k

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSIZMarker_NumTiles(t *testing.T) {
	tests := []struct {
		name       string
		siz        SIZMarker
		wantXTiles int
		wantYTiles int
	}{
		{
			name: "single tile",
			siz: SIZMarker{
				XSiz: 256, YSiz: 256,
				XTsiz: 256, YTsiz: 256,
			},
			wantXTiles: 1,
			wantYTiles: 1,
		},
		{
			name: "2x2 tiles",
			siz: SIZMarker{
				XSiz: 256, YSiz: 256,
				XTsiz: 128, YTsiz: 128,
			},
			wantXTiles: 2,
			wantYTiles: 2,
		},
		{
			name: "partial tiles",
			siz: SIZMarker{
				XSiz: 300, YSiz: 200,
				XTsiz: 128, YTsiz: 128,
			},
			wantXTiles: 3, // ceil(300/128) = 3
			wantYTiles: 2, // ceil(200/128) = 2
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantXTiles, tt.siz.NumXTiles())
			assert.Equal(t, tt.wantYTiles, tt.siz.NumYTiles())
			assert.Equal(t, tt.wantXTiles*tt.wantYTiles, tt.siz.NumTiles())
		})
	}
}

func TestCODMarker_CodeBlockSize(t *testing.T) {
	cod := CODMarker{
		CodeBlockWidthExp:  4, // 2^(4+2) = 64
		CodeBlockHeightExp: 4,
	}
	assert.Equal(t, 64, cod.CodeBlockWidth())
	assert.Equal(t, 64, cod.CodeBlockHeight())

	cod.CodeBlockWidthExp = 2 // 2^(2+2) = 16
	cod.CodeBlockHeightExp = 3 // 2^(3+2) = 32
	assert.Equal(t, 16, cod.CodeBlockWidth())
	assert.Equal(t, 32, cod.CodeBlockHeight())
}

func TestProgressionOrder_String(t *testing.T) {
	assert.Equal(t, "LRCP", ProgressionLRCP.String())
	assert.Equal(t, "RLCP", ProgressionRLCP.String())
	assert.Equal(t, "RPCL", ProgressionRPCL.String())
	assert.Equal(t, "PCRL", ProgressionPCRL.String())
	assert.Equal(t, "CPRL", ProgressionCPRL.String())
}

func TestSubband_String(t *testing.T) {
	assert.Equal(t, "LL", SubbandLL.String())
	assert.Equal(t, "HL", SubbandHL.String())
	assert.Equal(t, "LH", SubbandLH.String())
	assert.Equal(t, "HH", SubbandHH.String())
}

func TestBuildSIZ(t *testing.T) {
	components := []ComponentInfo{
		{Precision: 8, Signed: false, XRsiz: 1, YRsiz: 1},
	}

	siz := BuildSIZ(512, 256, components, 0, 0)

	assert.Equal(t, uint32(512), siz.XSiz)
	assert.Equal(t, uint32(256), siz.YSiz)
	assert.Equal(t, uint32(512), siz.XTsiz) // Full image tile
	assert.Equal(t, uint32(256), siz.YTsiz)
	assert.Equal(t, 1, len(siz.Components))
	assert.Equal(t, 1, siz.NumTiles())
}

func TestBuildDefaultCOD(t *testing.T) {
	cod := BuildDefaultCOD(5, 1, ProgressionLRCP, false)

	assert.Equal(t, ProgressionLRCP, cod.Progression)
	assert.Equal(t, uint16(1), cod.NumLayers)
	assert.Equal(t, byte(5), cod.DecompLevels)
	assert.Equal(t, TransformReversible53, cod.Transform)
	assert.Equal(t, byte(0), cod.MCT)
	assert.Equal(t, 64, cod.CodeBlockWidth())
	assert.Equal(t, 64, cod.CodeBlockHeight())

	// With MCT
	cod = BuildDefaultCOD(5, 1, ProgressionLRCP, true)
	assert.Equal(t, byte(1), cod.MCT)
}

func TestBuildDefaultQCD(t *testing.T) {
	qcd := BuildDefaultQCD(5, 2)

	assert.Equal(t, byte(0), qcd.Sqcd) // Reversible
	assert.Equal(t, byte(2), qcd.GuardBits)
	assert.Equal(t, 16, len(qcd.StepSizes)) // 3*5 + 1 = 16 subbands
}

func TestCodestreamWriter_WriteSIZ(t *testing.T) {
	var buf bytes.Buffer
	cw := NewCodestreamWriter(&buf)

	siz := &SIZMarker{
		Rsiz:   0,
		XSiz:   256,
		YSiz:   256,
		XOsiz:  0,
		YOsiz:  0,
		XTsiz:  256,
		YTsiz:  256,
		XTOsiz: 0,
		YTOsiz: 0,
		Components: []ComponentInfo{
			{Precision: 8, Signed: false, XRsiz: 1, YRsiz: 1},
		},
	}

	err := cw.WriteSIZ(siz)
	require.NoError(t, err)
	require.NoError(t, cw.Flush())

	data := buf.Bytes()

	// Check marker
	assert.Equal(t, byte(0xFF), data[0])
	assert.Equal(t, byte(0x51), data[1])

	// Check length (38 + 3*1 = 41)
	assert.Equal(t, byte(0x00), data[2])
	assert.Equal(t, byte(41), data[3])
}

func TestCodestreamWriter_WriteCOD(t *testing.T) {
	var buf bytes.Buffer
	cw := NewCodestreamWriter(&buf)

	cod := BuildDefaultCOD(5, 1, ProgressionLRCP, false)

	err := cw.WriteCOD(cod)
	require.NoError(t, err)
	require.NoError(t, cw.Flush())

	data := buf.Bytes()

	// Check marker
	assert.Equal(t, byte(0xFF), data[0])
	assert.Equal(t, byte(0x52), data[1])

	// Check length (12)
	assert.Equal(t, byte(0x00), data[2])
	assert.Equal(t, byte(12), data[3])
}

func TestCodestreamRoundTrip_SIZ(t *testing.T) {
	original := &SIZMarker{
		Rsiz:   0,
		XSiz:   512,
		YSiz:   256,
		XOsiz:  0,
		YOsiz:  0,
		XTsiz:  128,
		YTsiz:  128,
		XTOsiz: 0,
		YTOsiz: 0,
		Components: []ComponentInfo{
			{Precision: 8, Signed: false, XRsiz: 1, YRsiz: 1},
			{Precision: 8, Signed: false, XRsiz: 1, YRsiz: 1},
			{Precision: 8, Signed: false, XRsiz: 1, YRsiz: 1},
		},
	}

	// Write
	var buf bytes.Buffer
	cw := NewCodestreamWriter(&buf)
	require.NoError(t, cw.WriteSOC())
	require.NoError(t, cw.WriteSIZ(original))

	// Add minimal COD and QCD for a valid main header
	cod := BuildDefaultCOD(5, 1, ProgressionLRCP, true)
	require.NoError(t, cw.WriteCOD(cod))
	qcd := BuildDefaultQCD(5, 2)
	require.NoError(t, cw.WriteQCD(qcd))

	// Write SOT to end main header
	require.NoError(t, cw.WriteSOT(&SOTMarker{TileIndex: 0, TilePartLen: 14}))
	require.NoError(t, cw.Flush())

	// Read back
	cr := NewCodestreamReader(bytes.NewReader(buf.Bytes()))
	err := cr.ReadMainHeader()
	require.NoError(t, err)

	// Verify SIZ
	assert.Equal(t, original.XSiz, cr.SIZ.XSiz)
	assert.Equal(t, original.YSiz, cr.SIZ.YSiz)
	assert.Equal(t, original.XTsiz, cr.SIZ.XTsiz)
	assert.Equal(t, original.YTsiz, cr.SIZ.YTsiz)
	assert.Equal(t, len(original.Components), len(cr.SIZ.Components))
	for i := range original.Components {
		assert.Equal(t, original.Components[i].Precision, cr.SIZ.Components[i].Precision)
		assert.Equal(t, original.Components[i].Signed, cr.SIZ.Components[i].Signed)
	}

	// Verify COD
	assert.Equal(t, cod.Progression, cr.COD.Progression)
	assert.Equal(t, cod.NumLayers, cr.COD.NumLayers)
	assert.Equal(t, cod.DecompLevels, cr.COD.DecompLevels)
	assert.Equal(t, cod.Transform, cr.COD.Transform)
	assert.Equal(t, cod.MCT, cr.COD.MCT)
}

func TestCodestreamWriter_SOT(t *testing.T) {
	var buf bytes.Buffer
	cw := NewCodestreamWriter(&buf)

	sot := &SOTMarker{
		TileIndex:    5,
		TilePartLen:  1000,
		TilePartIdx:  0,
		NumTileParts: 1,
	}

	err := cw.WriteSOT(sot)
	require.NoError(t, err)
	require.NoError(t, cw.Flush())

	// Read back using CodestreamReader
	cr := NewCodestreamReader(bytes.NewReader(buf.Bytes()))

	// Skip marker (already consumed that we're at SOT)
	marker, err := cr.r.ReadUint16()
	require.NoError(t, err)
	assert.Equal(t, uint16(MarkerSOT), marker)

	readSOT, err := cr.ReadSOT()
	require.NoError(t, err)

	assert.Equal(t, sot.TileIndex, readSOT.TileIndex)
	assert.Equal(t, sot.TilePartLen, readSOT.TilePartLen)
	assert.Equal(t, sot.TilePartIdx, readSOT.TilePartIdx)
	assert.Equal(t, sot.NumTileParts, readSOT.NumTileParts)
}

func TestParseCodestreamHeader(t *testing.T) {
	// Build a minimal valid codestream header
	var buf bytes.Buffer
	cw := NewCodestreamWriter(&buf)

	require.NoError(t, cw.WriteSOC())

	siz := BuildSIZ(256, 256, []ComponentInfo{
		{Precision: 8, Signed: false, XRsiz: 1, YRsiz: 1},
	}, 0, 0)
	require.NoError(t, cw.WriteSIZ(siz))

	cod := BuildDefaultCOD(3, 1, ProgressionRLCP, false)
	require.NoError(t, cw.WriteCOD(cod))

	qcd := BuildDefaultQCD(3, 1)
	require.NoError(t, cw.WriteQCD(qcd))

	require.NoError(t, cw.WriteSOT(&SOTMarker{}))
	require.NoError(t, cw.Flush())

	// Parse
	parsedSIZ, parsedCOD, parsedQCD, err := ParseCodestreamHeader(buf.Bytes())
	require.NoError(t, err)

	assert.Equal(t, uint32(256), parsedSIZ.XSiz)
	assert.Equal(t, uint32(256), parsedSIZ.YSiz)
	assert.Equal(t, 1, len(parsedSIZ.Components))

	assert.Equal(t, ProgressionRLCP, parsedCOD.Progression)
	assert.Equal(t, byte(3), parsedCOD.DecompLevels)
	assert.Equal(t, TransformReversible53, parsedCOD.Transform)

	assert.Equal(t, byte(0), parsedQCD.Sqcd) // Reversible
	assert.Equal(t, 10, len(parsedQCD.StepSizes)) // 3*3 + 1
}
