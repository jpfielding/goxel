# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Goxel is a fast, modern, cross-platform DICOM medical image viewer built with Go and the Fyne UI toolkit. It provides both 2D slice viewing and 3D volume rendering with GPU acceleration using OpenGL.

## Build & Test Commands

```bash
# Build the application
make build-ctl                    # Outputs binary to bin/goxel

# Run the application
./bin/goxel goxel -p <path>      # View DICOM file or directory
./bin/goxel decode -u <path>     # Decode and inspect DICOM metadata
./bin/goxel merge -i <dir> -o <output>  # Merge slices into multi-frame file

# Testing
go test -short -v ./pkg/...      # Run short tests
go test -v ./pkg/...             # Run all tests (including integration)
go test -v ./pkg/<package>       # Test a specific package

# Code quality
make lint                         # Run golangci-lint
make vet                          # Run go vet
make vulnerability                # Check for security vulnerabilities

# Dependencies
make update-deps                  # Update go.mod and vendor directory

# Clean
make clean                        # Remove build outputs
make nuke                         # Full reset (git clean)
```

## Architecture

### Core Package Structure

The codebase is organized into three main functional layers:

**1. DICOM Layer (`pkg/dicom/`)**
- **Purpose**: Pure Go DICOM parsing and generation
- **Key files**:
  - `dicom.go` - Core API (ReadFile, Parse, IsCT/IsMR/IsDX helpers)
  - `reader.go` - Low-level DICOM parsing with transfer syntax detection
  - `writer.go` - DICOM file generation
  - `dataset_builder.go` - Fluent API for constructing DICOM datasets
  - `ct.go`, `dx.go`, `mr.go` - IOD (Information Object Definition) implementations for CT, DX, and MR modalities
  - `module/` - DICOM Information Modules (Patient, Study, Series, Equipment, CT/MR Image, VOI LUT)
  - `tag/` - DICOM tag definitions and constants
  - `transfer/` - Transfer syntax handling (ExplicitVR, ImplicitVR, compressed)
  - `vr/` - Value Representation types
- **Architecture**: Low-level parser (reader.go) -> Dataset model -> High-level IOD wrappers (ct.go/mr.go/dx.go) -> Module structs

**2. Compression Layer (`pkg/compress/`)**
- **Purpose**: Pure Go implementations of medical image compression formats
- **Codecs**:
  - `jpeg2k/` - JPEG 2000 (wavelet-based, excellent compression)
  - `jpegli/` - JPEG Lossless (traditional DPCM-based)
  - `jpegls/` - JPEG-LS (LOCO-I algorithm, very efficient for medical images)
  - `rle/` - Run-Length Encoding (PackBits variant)
- **Integration**: Codecs are automatically selected by `pkg/dicom` based on Transfer Syntax UID
- **Design**: All implementations are pure Go with no CGO dependencies for cross-platform compilation

**3. Volume Rendering Layer (`pkg/volume/`)**
- **Purpose**: GPU-accelerated 3D volume rendering and 2D slice extraction
- **Key files**:
  - `gpu_ray_caster.go` - OpenGL-based ray casting for 3D rendering
  - `cpu_ray_caster.go` - CPU fallback for systems without GPU
  - `config.go` - Rendering configuration (transfer functions, lighting)
  - `transfer_function.go` - Opacity/color mapping for volume visualization
  - `types.go` - Core volume data structures
- **Architecture**: Uses OpenGL 3.3 with GLSL shaders for real-time volume ray casting

**4. UI Layer (`pkg/goxel/`)**
- **Purpose**: Fyne-based GUI for viewing DICOM images
- **Key files**:
  - `ui.go` - Main UI orchestration and layout management
  - `dicom_loader.go` - Bridge between DICOM data and UI (ScanCollection, CompositeVolume)
  - `scan_adapter.go` - Converts DICOM datasets to UI-compatible pixel data
  - `volume.go` - 3D volume rendering widget
  - `interactive_view.go` - 2D slice view with annotation support
  - `dicox.go` - Custom image widget for windowing/level adjustments
  - `*_slider.go` - Custom UI controls for windowing, multi-range selection
  - `bundle.go` - Embedded resources (icon, etc.)
- **Multi-view support**: Side-by-side 2D/3D views, single 2D, single 3D layouts
- **Data model**: `ScanCollection` -> `CompositeVolume` (per series) -> `PixelData` (for 2D) or OpenGL textures (for 3D)

### Data Flow

**Loading DICOM files:**
1. `goxel.Load()` or `goxel.LoadDICOMDir()` reads file(s)
2. `dicom.Parse()` decodes DICOM structure
3. Compressed pixel data is decoded using appropriate codec from `pkg/compress/`
4. `parseDICOMDataset()` converts to `ScanCollection` with `CompositeVolume` per series
5. UI receives `ScanCollection` and can display 2D slices or 3D volume

**Rendering pipeline:**
- **2D**: `DICOXImage` widget applies window/level to pixel data and renders to Fyne canvas
- **3D**: `VolumeRenderer` uploads voxel data to OpenGL 3D texture, ray caster samples through volume with transfer function

**Creating DICOM files:**
1. Use IOD constructors (`dicom.NewCTImage()`, `dicom.NewMRImage()`)
2. Set metadata via module structs (Patient, Study, Series, Equipment, Image)
3. Call `SetPixelData()` with raw pixel values
4. Optionally enable compression (`UseCompression = true`, `CompressionCodec = "jpeg-ls"`)
5. `Write(path)` generates compliant DICOM file

### Key Abstractions

- **Dataset**: Low-level DICOM data structure (tag-value pairs)
- **IOD (Information Object Definition)**: High-level wrappers for CT/MR/DX images with typed module access
- **ScanCollection**: Multi-series container grouping related DICOM files (UI layer)
- **CompositeVolume**: Single 3D volume with metadata (series name, voxel spacing, Hounsfield units)
- **Transfer Syntax**: Encoding format (uncompressed, JPEG-LS, JPEG 2000, RLE)

## Important Patterns

### DICOM Tag Access
```go
// Low-level
elem, err := dataset.Get(tag.PatientName)
name := elem.GetString()

// High-level (via IOD)
ct := &dicom.CTImage{}
ct.CopyMetadataFrom(dataset)
name := ct.Patient.PatientName
```

### Compression Support
- Encoder/decoder automatically selected based on Transfer Syntax UID
- To create compressed DICOM: set `UseCompression = true` and `CompressionCodec = "jpeg-ls"/"jpeg-li"/"rle"` on IOD
- To read compressed DICOM: `dicom.Parse()` handles decompression transparently

### Multi-frame vs Single-frame
- Single-frame: One image per file (traditional CT/MR series)
- Multi-frame: Multiple slices in one file (supported via `NumberOfFrames`)
- Loader automatically detects and handles both formats

### Volume Coordinate Systems
- DICOM uses patient coordinate system (Image Orientation Patient, Image Position Patient)
- Volume renderer uses voxel indices (i, j, k)
- `VoxelSize{X,Y,Z}` provides physical spacing for aspect ratio correction

## Testing Notes

- Short tests (`-short`) skip slow integration tests
- Use `dicom.ReadFile()` with testdata samples from `pkg/testdata/`
- Compression roundtrip tests verify codec correctness
- Volume rendering tests may require GPU (will skip on headless systems)

## Dependencies

- **UI**: Fyne v2.7.1 (cross-platform toolkit)
- **3D Graphics**: go-gl/gl (OpenGL 3.3), go-gl/glfw (windowing)
- **CLI**: spf13/cobra (command structure)
- **No CGO required** for DICOM parsing/compression (pure Go)
