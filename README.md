# goxel

A fast, modern DICOM image viewer built with Go and the Fyne toolkit.

![goxel in action](goxel.gif)

**Try it in the browser:** https://jpfielding.github.io/goxel/ (loads a ~17 MB sample CT on first visit).

## Features

- **Multi-format DICOM support** - Load single files, directories, or multi-frame DICOM
- **3D Volume Rendering** - Real-time volume visualization with adjustable opacity and thresholds
- **2D Slice Navigation** - Browse through axial slices with window/level controls
- **Medical Imaging Presets** - Built-in window presets for Soft Tissue, Bone, Lung, Brain, and Liver
- **Material Segmentation** - Color-coded tissue classification (Background, Soft Tissue, Dense Tissue, Bone, Metal)
- **JPEG-LS Compression** - Native support for lossless DICOM compression
- **Cross-platform** - Runs on macOS, Linux, and Windows, and in the browser (WebAssembly + WebGL2)
- **Responsive layout** - Desktop panels, or phone portrait/landscape layouts with a tab bar and paged controls

## Installation

```bash
go install github.com/jpfielding/goxel/cmd@latest
```

Or build from source:

```bash
git clone https://github.com/jpfielding/goxel.git
cd goxel
make build-ctl            # bin/goxel (needs cgo for OpenGL)
```

### Browser build

```bash
make build-wasm           # bin/web: goxel.wasm, wasm_exec.js, index.html, samples/
make serve-wasm           # builds, then serves bin/web on http://127.0.0.1:8080
```

`build-wasm` needs no cgo. The Pages workflow (`.github/workflows/pages.yml`)
publishes `bin/web` from `main`; set the repository's Pages source to
"GitHub Actions" once.

Known limits of the browser build:

- One file at a time from the file picker (with several selected, the
  largest is loaded); no folder open and no drag-and-drop.
- Volumes over 32 M voxels are halved on each axis for the 3D texture.
- About 40 MB of wasm (about 12 MB gzipped) before the sample loads.
- Without WebGL2 the 3D view falls back to the CPU renderer, which is slow
  in single-threaded wasm.
- Touch: one-finger drag and pinch are translated to mouse and wheel input;
  two-finger pan is not supported (use Pan on the Camera page).
- Finding boxes and the axis gizmo are drawn on desktop only.

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
| Drag / Shift + drag / scroll | Orbit / pan / zoom the 3D view |
| View along X Y Z, Fit view | Camera presets |
| Focus volume / Focus slice | Enlarge one view; Esc exits |
| Slice plane, Cutaway | Outline the 2D slice in 3D, or cut the volume at it |
| 3D windowing | Center, Width, Opacity, Density |
| Palette | Materials / Threat / Grayscale, Quality, Material bands editor |
| Lighting | Depth cue, Ambient, Diffuse, Specular |
| Slice group, arrow keys | Plane, slice, Previous / Next |
| 2D windowing, Image | Center/Width, Zoom, Composite projection |

## Architecture

```
pkg/
├── dicom/          # DICOM parser and writer
│   ├── module/     # DICOM IOD modules (Patient, Study, Series, etc.)
│   ├── tag/        # DICOM tag definitions
│   └── transfer/   # Transfer syntax handling
├── compress/       # Image compression codecs
│   ├── jpegls/     # JPEG-LS lossless
│   ├── jpegli/     # JPEG lossless
│   ├── jpeg2k/     # JPEG 2000
│   └── rle/        # Run-length encoding
├── volume/         # 3D volume rendering
└── goxel/          # Fyne UI application
```

## Credits

Inspired by the [Fyne DICOM Viewer](https://apps.fyne.io/apps/com.fynelabs.dicomviewer.html).
Aided heavily by Claude.ai, esp in the opengl work.  

## License

MIT
