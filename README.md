# goxel

A fast, modern DICOM image viewer built with Go and the Fyne toolkit.

![goxel in action](goxel.gif)

## Features

- **Multi-format DICOM support** - Load single files, directories, or multi-frame DICOM
- **3D Volume Rendering** - Real-time volume visualization with adjustable opacity and thresholds
- **2D Slice Navigation** - Browse through axial slices with window/level controls
- **Medical Imaging Presets** - Built-in window presets for Soft Tissue, Bone, Lung, Brain, and Liver
- **Material Segmentation** - Color-coded tissue classification (Background, Soft Tissue, Dense Tissue, Bone, Metal)
- **JPEG-LS Compression** - Native support for lossless DICOM compression
- **Cross-platform** - Runs on macOS, Linux, and Windows

## Installation

```bash
go install github.com/jpfielding/goxel/cmd@latest
```

Or build from source:

```bash
git clone https://github.com/jpfielding/goxel.git
cd goxel
go build -o goxel ./cmd
```

## Usage

### Viewer

```bash
# Open a DICOM file
goxel goxel -p /path/to/file.dcm

# Open a directory of DICOM slices
goxel goxel -p /path/to/dicom/directory
```

### Merge DICOM Slices

Combine multiple DICOM slice files into a single multi-frame DICOM with optional compression:

```bash
# Merge with JPEG-LS compression (default)
goxel merge -i /path/to/slices -o output.dcm

# Merge without compression
goxel merge -i /path/to/slices -o output.dcm -c=false
```

### Decode DICOM Metadata

```bash
goxel decode -u /path/to/file.dcm
```

## Controls

| Control | Action |
|---------|--------|
| Slice Slider | Navigate through image slices |
| Window Level | Adjust brightness/contrast center |
| Window Width | Adjust brightness/contrast range |
| Z Scale | Adjust 3D volume aspect ratio |
| Material Sliders | Adjust tissue threshold boundaries |
| Opacity Slider | Control 3D rendering transparency |

## Architecture

```
pkg/
├── dicom/          # DICOM parser and writer
│   ├── module/     # DICOM IOD modules (Patient, Study, Series, etc.)
│   ├── tag/        # DICOM tag definitions
│   └── transfer/   # Transfer syntax handling
├── compress/       # Image compression codecs
│   ├── jpegls/     # JPEG-LS lossless
│   ├── jpegli/     # JPEG lossy
│   ├── jpeg2k/     # JPEG 2000
│   └── rle/        # Run-length encoding
├── volume/         # 3D volume rendering
└── goxel/          # Fyne UI application
```

## Credits

Inspired by the [Fyne DICOM Viewer](https://apps.fyne.io/apps/com.fynelabs.dicomviewer.html).

## License

MIT
