package volume

import "image/color"

// TransferFunctionType defines different colormaps.
// All types are now config-based, using RenderConfig color/opacity maps.
type TransferFunctionType int

const (
	// TransferFunctionDefault uses the "DEFAULT" color map from config
	TransferFunctionDefault TransferFunctionType = iota
	// TransferFunctionFinding uses the "FINDING" color map (red highlighting)
	TransferFunctionFinding
	// TransferFunctionMonochrome uses the "MONOCHROME" color map (grayscale)
	TransferFunctionMonochrome
	// TransferFunctionCustom uses bands from the current slider state
	TransferFunctionCustom
)

// TransferFunctionSize is the resolution of the transfer function texture
const TransferFunctionSize = 1024

// CreateTransferFunction generates a 1024-entry RGBA transfer function
// using the specified type and the current config.
func CreateTransferFunction(tfType TransferFunctionType) []color.RGBA {
	cfg := GetConfig()

	switch tfType {
	case TransferFunctionFinding:
		return CreateTransferFunctionFromConfig(cfg, "FINDING", 30000)
	case TransferFunctionMonochrome:
		return CreateTransferFunctionFromConfig(cfg, "MONOCHROME", 30000)
	case TransferFunctionCustom:
		// Use default bands; caller should use CreateTransferFunctionFromBands directly
		return CreateTransferFunctionFromBands(cfg, "DEFAULT", DefaultColorBands())
	default:
		return CreateTransferFunctionFromConfig(cfg, "DEFAULT", 30000)
	}
}

// TransferFunctionNames returns human-readable names for the transfer function types.
func TransferFunctionNames() map[TransferFunctionType]string {
	return map[TransferFunctionType]string{
		TransferFunctionDefault:    "Default (MVS)",
		TransferFunctionFinding:     "Finding (Red)",
		TransferFunctionMonochrome: "Monochrome",
		TransferFunctionCustom:     "Custom Bands",
	}
}
