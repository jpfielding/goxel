package jpeg2k

import (
	"bytes"
	"image"
	"image/color"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncode_Gray8_SmallImage(t *testing.T) {
	// Create a small grayscale image
	img := image.NewGray(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			img.SetGray(x, y, color.Gray{Y: uint8((x + y) * 8)})
		}
	}

	var buf bytes.Buffer
	err := Encode(&buf, img, nil)
	require.NoError(t, err)
	assert.True(t, buf.Len() > 0, "encoded data should not be empty")

	// Verify SOC marker at start
	data := buf.Bytes()
	assert.Equal(t, byte(0xFF), data[0])
	assert.Equal(t, byte(0x4F), data[1])
}

func TestEncode_Gray16_SmallImage(t *testing.T) {
	img := image.NewGray16(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			img.SetGray16(x, y, color.Gray16{Y: uint16((x + y) * 2048)})
		}
	}

	var buf bytes.Buffer
	err := Encode(&buf, img, nil)
	require.NoError(t, err)
	assert.True(t, buf.Len() > 0)
}

func TestEncode_RGBA_SmallImage(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			img.SetRGBA(x, y, color.RGBA{R: uint8(x * 16), G: uint8(y * 16), B: 128, A: 255})
		}
	}

	var buf bytes.Buffer
	err := Encode(&buf, img, nil)
	require.NoError(t, err)
	assert.True(t, buf.Len() > 0)
}

func TestEncode_InvalidDimensions(t *testing.T) {
	img := image.NewGray(image.Rect(0, 0, 0, 0))
	var buf bytes.Buffer
	err := Encode(&buf, img, nil)
	require.Error(t, err)
}

func TestEncode_WithOptions(t *testing.T) {
	img := image.NewGray(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			img.SetGray(x, y, color.Gray{Y: uint8(x ^ y)})
		}
	}

	opts := &Options{
		DecompLevels: 3,
		NumLayers:    1,
		Progression:  ProgressionLRCP,
	}

	var buf bytes.Buffer
	err := Encode(&buf, img, opts)
	require.NoError(t, err)
	assert.True(t, buf.Len() > 0)
}

func TestDecode_InvalidData(t *testing.T) {
	// Empty data
	_, err := Decode(bytes.NewReader([]byte{}))
	require.Error(t, err)

	// Too short
	_, err = Decode(bytes.NewReader([]byte{0xFF}))
	require.Error(t, err)

	// Wrong magic
	_, err = Decode(bytes.NewReader([]byte{0x00, 0x00, 0x00, 0x00}))
	require.Error(t, err)
}

func TestDecodeConfig_InvalidData(t *testing.T) {
	_, err := DecodeConfig(bytes.NewReader([]byte{}))
	require.Error(t, err)

	_, err = DecodeConfig(bytes.NewReader([]byte{0xFF, 0x4F})) // Just SOC, no SIZ
	require.Error(t, err)
}

func TestRoundTrip_Gray8(t *testing.T) {
	// Create a gradient image
	width, height := 32, 32
	original := image.NewGray(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			original.SetGray(x, y, color.Gray{Y: uint8((x + y) % 256)})
		}
	}

	// Encode
	var buf bytes.Buffer
	err := Encode(&buf, original, nil)
	require.NoError(t, err)

	// Decode
	decoded, err := Decode(bytes.NewReader(buf.Bytes()))
	require.NoError(t, err)

	// Verify dimensions
	bounds := decoded.Bounds()
	assert.Equal(t, width, bounds.Dx())
	assert.Equal(t, height, bounds.Dy())

	// Verify pixel values (lossless should be exact)
	gray, ok := decoded.(*image.Gray)
	require.True(t, ok, "decoded image should be Gray")

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			expected := original.GrayAt(x, y).Y
			actual := gray.GrayAt(x, y).Y
			assert.Equal(t, expected, actual, "pixel mismatch at (%d, %d)", x, y)
		}
	}
}

func TestRoundTrip_Gray16(t *testing.T) {
	width, height := 32, 32
	original := image.NewGray16(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			original.SetGray16(x, y, color.Gray16{Y: uint16((x + y) * 1024)})
		}
	}

	var buf bytes.Buffer
	err := Encode(&buf, original, nil)
	require.NoError(t, err)

	decoded, err := Decode(bytes.NewReader(buf.Bytes()))
	require.NoError(t, err)

	bounds := decoded.Bounds()
	assert.Equal(t, width, bounds.Dx())
	assert.Equal(t, height, bounds.Dy())

	gray16, ok := decoded.(*image.Gray16)
	require.True(t, ok, "decoded image should be Gray16")

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			expected := original.Gray16At(x, y).Y
			actual := gray16.Gray16At(x, y).Y
			assert.Equal(t, expected, actual, "pixel mismatch at (%d, %d)", x, y)
		}
	}
}

func TestRoundTrip_RGBA(t *testing.T) {
	width, height := 32, 32
	original := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			original.SetRGBA(x, y, color.RGBA{
				R: uint8(x * 8),
				G: uint8(y * 8),
				B: uint8((x + y) * 4),
				A: 255,
			})
		}
	}

	var buf bytes.Buffer
	err := Encode(&buf, original, nil)
	require.NoError(t, err)

	decoded, err := Decode(bytes.NewReader(buf.Bytes()))
	require.NoError(t, err)

	bounds := decoded.Bounds()
	assert.Equal(t, width, bounds.Dx())
	assert.Equal(t, height, bounds.Dy())

	rgba, ok := decoded.(*image.RGBA)
	require.True(t, ok, "decoded image should be RGBA")

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			expected := original.RGBAAt(x, y)
			actual := rgba.RGBAAt(x, y)
			assert.Equal(t, expected.R, actual.R, "R mismatch at (%d, %d)", x, y)
			assert.Equal(t, expected.G, actual.G, "G mismatch at (%d, %d)", x, y)
			assert.Equal(t, expected.B, actual.B, "B mismatch at (%d, %d)", x, y)
		}
	}
}

func TestRoundTrip_SolidColor(t *testing.T) {
	// Solid color should compress very well
	width, height := 64, 64
	original := image.NewGray(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			original.SetGray(x, y, color.Gray{Y: 128})
		}
	}

	var buf bytes.Buffer
	err := Encode(&buf, original, nil)
	require.NoError(t, err)

	decoded, err := Decode(bytes.NewReader(buf.Bytes()))
	require.NoError(t, err)

	gray, ok := decoded.(*image.Gray)
	require.True(t, ok)

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			assert.Equal(t, uint8(128), gray.GrayAt(x, y).Y)
		}
	}
}

func TestRoundTrip_NonPowerOfTwo(t *testing.T) {
	// Test non-power-of-2 dimensions
	width, height := 33, 27
	original := image.NewGray(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			original.SetGray(x, y, color.Gray{Y: uint8((x * y) % 256)})
		}
	}

	var buf bytes.Buffer
	err := Encode(&buf, original, nil)
	require.NoError(t, err)

	decoded, err := Decode(bytes.NewReader(buf.Bytes()))
	require.NoError(t, err)

	bounds := decoded.Bounds()
	assert.Equal(t, width, bounds.Dx())
	assert.Equal(t, height, bounds.Dy())

	gray, ok := decoded.(*image.Gray)
	require.True(t, ok)

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			expected := original.GrayAt(x, y).Y
			actual := gray.GrayAt(x, y).Y
			assert.Equal(t, expected, actual, "pixel mismatch at (%d, %d)", x, y)
		}
	}
}

func TestDecodeConfig_Gray(t *testing.T) {
	width, height := 48, 32
	img := image.NewGray(image.Rect(0, 0, width, height))

	var buf bytes.Buffer
	err := Encode(&buf, img, nil)
	require.NoError(t, err)

	config, err := DecodeConfig(bytes.NewReader(buf.Bytes()))
	require.NoError(t, err)

	assert.Equal(t, width, config.Width)
	assert.Equal(t, height, config.Height)
	assert.Equal(t, color.GrayModel, config.ColorModel)
}

func TestDecodeConfig_RGBA(t *testing.T) {
	width, height := 48, 32
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	var buf bytes.Buffer
	err := Encode(&buf, img, nil)
	require.NoError(t, err)

	config, err := DecodeConfig(bytes.NewReader(buf.Bytes()))
	require.NoError(t, err)

	assert.Equal(t, width, config.Width)
	assert.Equal(t, height, config.Height)
	assert.Equal(t, color.RGBAModel, config.ColorModel)
}

func TestDefaultOptions(t *testing.T) {
	opts := DefaultOptions()
	assert.Equal(t, 5, opts.DecompLevels)
	assert.Equal(t, 1, opts.NumLayers)
	assert.Equal(t, ProgressionLRCP, opts.Progression)
	assert.True(t, opts.UseMCT)
}

func BenchmarkEncode_Gray_64x64(b *testing.B) {
	img := image.NewGray(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			img.SetGray(x, y, color.Gray{Y: uint8((x + y) % 256)})
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		Encode(&buf, img, nil)
	}
}

func BenchmarkDecode_Gray_64x64(b *testing.B) {
	img := image.NewGray(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			img.SetGray(x, y, color.Gray{Y: uint8((x + y) % 256)})
		}
	}

	var buf bytes.Buffer
	Encode(&buf, img, nil)
	data := buf.Bytes()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Decode(bytes.NewReader(data))
	}
}

func BenchmarkRoundTrip_Gray_256x256(b *testing.B) {
	img := image.NewGray(image.Rect(0, 0, 256, 256))
	for y := 0; y < 256; y++ {
		for x := 0; x < 256; x++ {
			img.SetGray(x, y, color.Gray{Y: uint8(x ^ y)})
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		Encode(&buf, img, nil)
		Decode(bytes.NewReader(buf.Bytes()))
	}
}
