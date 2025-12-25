# pkg/dicom

A native Go implementation for reading and writing DICOM (Digital Imaging and Communications in Medicine) files. This package provides comprehensive support for medical imaging data with no CGO dependencies.

## Table of Contents

- [What is DICOM?](#what-is-dicom)
- [DICOM File Structure](#dicom-file-structure)
- [Transfer Syntaxes](#transfer-syntaxes)
- [Value Representations (VR)](#value-representations-vr)
- [Tags and Data Elements](#tags-and-data-elements)
- [Information Object Definitions (IODs)](#information-object-definitions-iods)
- [Modules](#modules)
- [Usage Examples](#usage-examples)
  - [Reading DICOM Files](#reading-dicom-files)
  - [Accessing Metadata](#accessing-metadata)
  - [Working with Pixel Data](#working-with-pixel-data)
  - [Creating DICOM Files](#creating-dicom-files)
  - [Compression Support](#compression-support)
  - [Working with Different Modalities](#working-with-different-modalities)
- [API Reference](#api-reference)

## What is DICOM?

DICOM (Digital Imaging and Communications in Medicine) is the international standard for medical images and related information. It defines the formats for medical images that can be exchanged with the data and quality necessary for clinical use.

Key concepts:

- **Standardized Format**: DICOM ensures that medical images from different manufacturers can be viewed and processed consistently
- **Metadata-Rich**: Beyond pixel data, DICOM files contain extensive metadata about the patient, study, equipment, and acquisition parameters
- **Modality-Agnostic**: Supports CT, MRI, X-Ray, Ultrasound, PET, and many other imaging modalities
- **Compression**: Supports multiple compression formats optimized for medical imaging

## DICOM File Structure

A DICOM file consists of:

```
┌─────────────────────────────────────┐
│  Preamble (128 bytes)               │  Optional padding
├─────────────────────────────────────┤
│  Prefix "DICM" (4 bytes)            │  File identification
├─────────────────────────────────────┤
│  File Meta Information (Group 0x0002)│  Transfer syntax, SOP Class
├─────────────────────────────────────┤
│  Dataset (Groups 0x0004-0xFFFF)     │  Patient, Study, Image data
└─────────────────────────────────────┘
```

### Data Elements

Each data element consists of:

```
┌──────────┬─────────┬────────────┬──────────┐
│   Tag    │   VR    │   Length   │  Value   │
│ (4 bytes)│(2 bytes)│ (2-6 bytes)│(Variable)│
└──────────┴─────────┴────────────┴──────────┘
```

- **Tag**: Identifies the data element (Group, Element)
- **VR**: Value Representation (data type)
- **Length**: Size of the value field
- **Value**: The actual data

## Transfer Syntaxes

Transfer Syntax defines how DICOM data is encoded. Key aspects:

- **Byte Order**: Little Endian vs Big Endian
- **VR Encoding**: Explicit (VR included) vs Implicit (VR inferred from data dictionary)
- **Compression**: Native (uncompressed) vs Encapsulated (compressed)

### Common Transfer Syntaxes

| Transfer Syntax | UID | Description |
|----------------|-----|-------------|
| Implicit VR Little Endian | 1.2.840.10008.1.2 | VR inferred, little endian |
| Explicit VR Little Endian | 1.2.840.10008.1.2.1 | VR explicit, little endian (most common) |
| JPEG-LS Lossless | 1.2.840.10008.1.2.4.80 | Lossless compression, excellent for medical images |
| JPEG Lossless (Process 14) | 1.2.840.10008.1.2.4.70 | Traditional lossless JPEG |
| JPEG 2000 Lossless | 1.2.840.10008.1.2.4.90 | Wavelet-based lossless compression |
| RLE Lossless | 1.2.840.10008.1.2.5 | Run-length encoding |

## Value Representations (VR)

VR specifies the data type and format of a value field. Common VRs:

| VR | Name | Description | Example |
|----|------|-------------|---------|
| **CS** | Code String | Short identifier | "CT", "MR" |
| **DA** | Date | YYYYMMDD format | "20240115" |
| **DS** | Decimal String | Floating point as string | "1.5", "-1024.25" |
| **IS** | Integer String | Integer as string | "256", "1" |
| **LO** | Long String | Character string (64 char max) | "Patient Description" |
| **PN** | Person Name | Person name with components | "Doe^John^M" |
| **SH** | Short String | Short character string (16 char max) | "CHEST" |
| **TM** | Time | HHMMSS.FFFFFF format | "143022.123" |
| **UI** | Unique Identifier | Dotted notation UID | "1.2.840.10008.5.1.4.1.1.2" |
| **US** | Unsigned Short | 16-bit unsigned integer | 512 |
| **UL** | Unsigned Long | 32-bit unsigned integer | 1048576 |
| **OB** | Other Byte | Binary data (byte stream) | Compressed pixel data |
| **OW** | Other Word | Binary data (16-bit words) | Uncompressed pixel data |
| **SQ** | Sequence | Nested dataset(s) | Icon image sequence |

## Tags and Data Elements

DICOM tags uniquely identify data elements using (Group, Element) pairs expressed in hexadecimal:

```
(0010,0010) = Patient Name
(0008,0060) = Modality
(7FE0,0010) = Pixel Data
```

### Tag Categories by Group

- **0x0002**: File Meta Information (Transfer Syntax, SOP Class)
- **0x0008**: Identification (Study Date, Modality, Institution)
- **0x0010**: Patient Information (Name, ID, Birth Date, Sex)
- **0x0018**: Acquisition Parameters (KVP, Slice Thickness, Scanner settings)
- **0x0020**: Relationship (Study/Series/Instance UIDs, Instance Number)
- **0x0028**: Image Presentation (Rows, Columns, Bits Allocated, Window/Level)
- **0x7FE0**: Pixel Data

### Private Tags

Tags with odd group numbers (e.g., 0x0009, 0x0019) are manufacturer-specific private tags.

## Information Object Definitions (IODs)

IODs define the structure and required attributes for specific types of DICOM objects. Each IOD specifies:

- Which modules are required
- Which attributes are mandatory vs optional
- Data type constraints

### Common IODs

| IOD | SOP Class UID | Description |
|-----|---------------|-------------|
| **CT Image** | 1.2.840.10008.5.1.4.1.1.2 | Computed Tomography images |
| **MR Image** | 1.2.840.10008.5.1.4.1.1.4 | Magnetic Resonance images |
| **DX Image** | 1.2.840.10008.5.1.4.1.1.1.1 | Digital Radiography (X-Ray) |
| **Ultrasound Image** | 1.2.840.10008.5.1.4.1.1.6.1 | Ultrasound images |

## Modules

Modules are collections of related attributes that describe a specific aspect of a DICOM object:

### Standard Modules

| Module | Purpose | Key Attributes |
|--------|---------|----------------|
| **Patient** | Patient demographics | Name, ID, Birth Date, Sex |
| **General Study** | Study information | Study Date, Description, Instance UID |
| **General Series** | Series information | Modality, Series Number, Description |
| **General Equipment** | Scanner/device info | Manufacturer, Model, Station Name |
| **Frame of Reference** | Spatial reference | Frame of Reference UID, Position Reference |
| **Image Plane** | Slice positioning | Image Position, Orientation, Pixel Spacing |
| **CT Image** | CT-specific parameters | KVP, Exposure Time, Reconstruction |
| **MR Image** | MR-specific parameters | Echo Time, Repetition Time, Flip Angle |
| **VOI LUT** | Window/Level presets | Window Center, Window Width |
| **SOP Common** | Instance identification | SOP Class UID, SOP Instance UID |

## Usage Examples

### Reading DICOM Files

#### Basic File Reading

```go
package main

import (
    "fmt"
    "log"

    "github.com/jpfielding/goxel/pkg/dicom"
)

func main() {
    // Read DICOM file
    ds, err := dicom.ReadFile("/path/to/file.dcm")
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Successfully loaded DICOM file\n")
    fmt.Printf("Transfer Syntax: %s\n", ds.TransferSyntax().Name())
}
```

#### Reading from Memory

```go
// Read from byte slice
data := []byte{...} // DICOM file contents
ds, err := dicom.ReadBuffer(data)
if err != nil {
    log.Fatal(err)
}
```

### Accessing Metadata

#### Using Dataset Methods

```go
// Patient information
patientName := dicom.GetPatientName(ds)
patientID := dicom.GetPatientID(ds)

// Study information
studyDate := dicom.GetStudyDate(ds)
studyDesc := dicom.GetStudyDescription(ds)

// Series information
modality := ds.Modality()
seriesDesc := dicom.GetSeriesDescription(ds)

// Image properties
rows := ds.Rows()
cols := ds.Columns()
numFrames := ds.NumberOfFrames()

fmt.Printf("Patient: %s (%s)\n", patientName, patientID)
fmt.Printf("Study: %s on %s\n", studyDesc, studyDate)
fmt.Printf("Modality: %s - %s\n", modality, seriesDesc)
fmt.Printf("Dimensions: %dx%d, %d frames\n", cols, rows, numFrames)
```

#### Low-Level Tag Access

```go
import "github.com/jpfielding/goxel/pkg/dicom/tag"

// Access any tag directly
elem, ok := ds.FindElement(tag.PatientName.Group, tag.PatientName.Element)
if ok {
    if name, ok := elem.GetString(); ok {
        fmt.Printf("Patient Name: %s\n", name)
    }
}

// Access numeric values
if elem, ok := ds.FindElement(tag.Rows.Group, tag.Rows.Element); ok {
    if rows, ok := elem.GetInt(); ok {
        fmt.Printf("Rows: %d\n", rows)
    }
}

// Access multi-valued attributes
if elem, ok := ds.FindElement(tag.ImagePositionPatient.Group, tag.ImagePositionPatient.Element); ok {
    if pos, ok := elem.GetFloats(); ok {
        fmt.Printf("Image Position: [%.2f, %.2f, %.2f]\n", pos[0], pos[1], pos[2])
    }
}
```

### Working with Pixel Data

#### Extracting Pixel Data

```go
// Get pixel data (automatically decompresses if needed)
pixelData, err := ds.GetPixelData()
if err != nil {
    log.Fatal(err)
}

// Get image dimensions
rows := ds.Rows()
cols := ds.Columns()
bitsAllocated := ds.BitsAllocated()

fmt.Printf("Pixel data: %d frames, %dx%d, %d bits\n",
    len(pixelData.Frames), cols, rows, bitsAllocated)

// Access individual frames
for i, frame := range pixelData.Frames {
    fmt.Printf("Frame %d: %d pixels\n", i, len(frame.Data))

    // First pixel value
    if len(frame.Data) > 0 {
        fmt.Printf("  First pixel: %d\n", frame.Data[0])
    }
}
```

#### Converting to Hounsfield Units (CT)

```go
// Get rescale parameters
intercept, slope := dicom.GetRescale(ds)

// Convert pixel values to HU
pixelData, _ := ds.GetPixelData()
for i, frame := range pixelData.Frames {
    huValues := make([]float64, len(frame.Data))
    for j, pixelValue := range frame.Data {
        huValues[j] = float64(pixelValue)*slope + intercept
    }
    fmt.Printf("Frame %d: HU range [%.1f, %.1f]\n", i,
        findMin(huValues), findMax(huValues))
}
```

#### Window/Level for Display

```go
// Get window parameters
windowCenter := dicom.GetWindowCenter(ds)
windowWidth := dicom.GetWindowWidth(ds)

fmt.Printf("Default Window: Center=%.1f, Width=%.1f\n", windowCenter, windowWidth)

// Apply window/level transformation for display
func applyWindow(pixelValue uint16, center, width float64) uint8 {
    val := float64(pixelValue)
    lower := center - width/2.0
    upper := center + width/2.0

    if val <= lower {
        return 0
    } else if val >= upper {
        return 255
    }

    return uint8(255.0 * (val - lower) / width)
}
```

### Creating DICOM Files

#### Creating a CT Image

```go
import (
    "time"

    "github.com/jpfielding/goxel/pkg/dicom"
    "github.com/jpfielding/goxel/pkg/dicom/module"
)

func createCTImage() error {
    // Create new CT image
    ct := dicom.NewCTImage()

    // Set patient information
    ct.Patient.SetPatientName("Doe", "John", "M", "", "")
    ct.Patient.PatientID = "12345"
    ct.Patient.PatientBirthDate = module.ParseDate("19800115")
    ct.Patient.PatientSex = "M"

    // Set study information
    ct.Study.StudyDescription = "Chest CT with contrast"
    ct.Study.AccessionNumber = "ACC123456"
    ct.Study.StudyID = "STU001"

    // Set series information
    ct.Series.Modality = "CT"
    ct.Series.SeriesDescription = "Chest Routine"
    ct.Series.SeriesNumber = "1"

    // Set equipment information
    ct.Equipment.Manufacturer = "Generic Medical Systems"
    ct.Equipment.ManufacturerModelName = "CT Scanner 3000"
    ct.Equipment.StationName = "CT01"

    // Set CT acquisition parameters
    ct.CTImageMod.KVP = 120.0
    ct.CTImageMod.ExposureTime = 1000.0 // milliseconds
    ct.CTImageMod.XRayTubeCurrent = 200.0 // mA

    // Set image plane information (slice positioning)
    ct.ImagePlane.PixelSpacing = []float64{0.5, 0.5} // mm
    ct.ImagePlane.SliceThickness = 2.5 // mm
    ct.ImagePlane.ImagePositionPatient = []float64{-125.0, -125.0, 0.0}
    ct.ImagePlane.ImageOrientationPatient = []float64{1, 0, 0, 0, 1, 0}

    // Create synthetic pixel data (512x512)
    rows, cols := 512, 512
    pixelData := make([]uint16, rows*cols)

    // Generate a simple test pattern
    for y := 0; y < rows; y++ {
        for x := 0; x < cols; x++ {
            // Create circular pattern
            dx := float64(x - cols/2)
            dy := float64(y - rows/2)
            dist := dx*dx + dy*dy

            if dist < 10000 {
                pixelData[y*cols+x] = 45000 // Soft tissue (~+200 HU)
            } else {
                pixelData[y*cols+x] = 32768 // Air (-1000 HU)
            }
        }
    }

    // Set pixel data
    ct.SetPixelData(rows, cols, pixelData)
    ct.Rows = rows
    ct.Columns = cols

    // Set window/level for display
    ct.VOILUT.AddWindow(40.0, 400.0, "Soft Tissue") // Center=40, Width=400

    // Write to file
    _, err := ct.Write("/path/to/output.dcm")
    return err
}
```

#### Creating an MR Image

```go
func createMRImage() error {
    // Create new MR image
    mr := dicom.NewMRImage()

    // Set patient info
    mr.Patient.SetPatientName("Smith", "Jane", "", "Dr.", "")
    mr.Patient.PatientID = "MR98765"

    // Set study info
    mr.Study.StudyDescription = "Brain MRI"

    // Set series info
    mr.Series.Modality = "MR"
    mr.Series.SeriesDescription = "T1 Axial"

    // Set MR-specific parameters
    mr.MRImageMod.EchoTime = 10.0 // ms
    mr.MRImageMod.RepetitionTime = 500.0 // ms
    mr.MRImageMod.FlipAngle = 90.0 // degrees
    mr.MRImageMod.MagneticFieldStrength = 3.0 // Tesla

    // Set image dimensions and pixel data
    rows, cols := 256, 256
    pixelData := make([]uint16, rows*cols)

    // ... fill pixelData ...

    mr.SetPixelData(rows, cols, pixelData)
    mr.Rows = rows
    mr.Columns = cols

    _, err := mr.Write("/path/to/mr_output.dcm")
    return err
}
```

### Compression Support

#### Creating Compressed DICOM

```go
func createCompressedCT() error {
    ct := dicom.NewCTImage()

    // ... set metadata ...

    // Set pixel data
    rows, cols := 512, 512
    pixelData := make([]uint16, rows*cols)
    // ... fill pixelData ...

    ct.SetPixelData(rows, cols, pixelData)
    ct.Rows = rows
    ct.Columns = cols

    // Enable compression
    ct.UseCompression = true
    ct.CompressionCodec = "jpeg-ls" // Options: "jpeg-ls", "jpeg-li", "rle", "jpeg-2000"

    _, err := ct.Write("/path/to/compressed.dcm")
    return err
}
```

#### Compression Codecs

| Codec | Description | Best For |
|-------|-------------|----------|
| `jpeg-ls` | JPEG-LS Lossless | General medical images, excellent compression ratio |
| `jpeg-li` | JPEG Lossless (Process 14) | Legacy compatibility |
| `rle` | Run-Length Encoding | Images with uniform regions |
| `jpeg-2000` | JPEG 2000 Lossless | High-quality compression, newer standard |

#### Reading Compressed DICOM

```go
// Decompression is automatic
ds, err := dicom.ReadFile("/path/to/compressed.dcm")
if err != nil {
    log.Fatal(err)
}

// Check if originally compressed
if ds.IsEncapsulated() {
    syntax := ds.TransferSyntax()
    fmt.Printf("Compressed with: %s\n", syntax.Name())
}

// Pixel data is automatically decompressed
pixelData, err := ds.GetPixelData()
// ... use pixelData normally ...
```

### Working with Different Modalities

#### Detecting Modality

```go
// Using helper functions
if dicom.IsCT(ds) {
    fmt.Println("This is a CT image")
    intercept, slope := dicom.GetRescale(ds)
    fmt.Printf("Rescale to HU: intercept=%.1f, slope=%.1f\n", intercept, slope)
}

if dicom.IsMR(ds) {
    fmt.Println("This is an MR image")
}

if dicom.IsDX(ds) {
    fmt.Println("This is a Digital Radiography image")
}

// Using modality string
modality := ds.Modality()
switch modality {
case "CT":
    // Handle CT
case "MR":
    // Handle MR
case "DX", "CR":
    // Handle X-Ray
}
```

#### Multi-frame Images

```go
// Check for multi-frame
numFrames := ds.NumberOfFrames()
if numFrames > 1 {
    fmt.Printf("Multi-frame image with %d frames\n", numFrames)
}

// Extract all frames
pixelData, err := ds.GetPixelData()
if err != nil {
    log.Fatal(err)
}

// Process each frame
for i, frame := range pixelData.Frames {
    sliceLocation := getSliceLocation(ds, i) // Custom function
    fmt.Printf("Frame %d at location %.2f mm\n", i, sliceLocation)

    // Process frame.Data ...
}
```

## API Reference

### Core Functions

```go
// Reading
func ReadFile(path string) (*Dataset, error)
func ReadBuffer(data []byte) (*Dataset, error)

// Modality detection
func IsCT(ds *Dataset) bool
func IsMR(ds *Dataset) bool
func IsDX(ds *Dataset) bool

// Dataset methods
func (ds *Dataset) Rows() int
func (ds *Dataset) Columns() int
func (ds *Dataset) NumberOfFrames() int
func (ds *Dataset) BitsAllocated() int
func (ds *Dataset) PixelRepresentation() int
func (ds *Dataset) Modality() string
func (ds *Dataset) TransferSyntax() TransferSyntax
func (ds *Dataset) IsEncapsulated() bool

// Metadata access
func GetPatientName(ds *Dataset) string
func GetPatientID(ds *Dataset) string
func GetStudyDate(ds *Dataset) string
func GetStudyDescription(ds *Dataset) string
func GetSeriesDescription(ds *Dataset) string
func GetInstanceNumber(ds *Dataset) int
func GetSliceLocation(ds *Dataset) float64

// Pixel data operations
func (ds *Dataset) GetPixelData() (*PixelData, error)
func GetRescale(ds *Dataset) (intercept, slope float64)
func GetWindowCenter(ds *Dataset) float64
func GetWindowWidth(ds *Dataset) float64

// Legacy helpers (deprecated)
func GetModality(ds *Dataset) string
func GetRows(ds *Dataset) int
func GetColumns(ds *Dataset) int
func GetNumberOfFrames(ds *Dataset) int
func GetBitsAllocated(ds *Dataset) int
func GetPixelRepresentation(ds *Dataset) int
func GetTransferSyntax(ds *Dataset) TransferSyntax
func IsEncapsulated(ds *Dataset) bool
```

### Dataset Builder

```go
// Create dataset with options
func NewDataset(opts ...Option) (*Dataset, error)

// Builder options
func WithElement(t tag.Tag, value interface{}) Option
func WithSequence(t tag.Tag, items ...*Dataset) Option
func WithFileMeta(sopClassUID, sopInstanceUID, transferSyntax string) Option
func WithModule(tags []module.IODElement) Option
func WithPixelData(rows, cols, bitsAllocated int, data []uint16, codec Codec) Option
func WithRawPixelData(pd *PixelData) Option
```

### IOD Constructors

```go
// Create IOD instances
func NewCTImage() *CTImage
func NewMRImage() *MRImage
func NewDXImage() *DXImage

// IOD methods
func (ct *CTImage) GetDataset() (*Dataset, error)
func (ct *CTImage) Write(path string) (int64, error)
func (ct *CTImage) WriteTo(w io.Writer) (int64, error)
func (ct *CTImage) SetPixelData(rows, cols int, data []uint16)
```

### Element Access

```go
// Dataset methods
func (ds *Dataset) FindElement(group, element uint16) (*Element, bool)

// Element value extraction
func (elem *Element) GetString() (string, bool)
func (elem *Element) GetUint16() (uint16, bool)
func (elem *Element) GetUint32() (uint32, bool)
func (elem *Element) GetInt() (int, bool)
func (elem *Element) GetInts() ([]int, bool)
func (elem *Element) GetFloats() ([]float64, bool)
func (elem *Element) GetPixelData() (*PixelData, bool)
```

### Transfer Syntax

```go
// Transfer syntax methods
func (s Syntax) IsExplicitVR() bool
func (s Syntax) IsLittleEndian() bool
func (s Syntax) IsEncapsulated() bool
func (s Syntax) IsJPEGLS() bool
func (s Syntax) IsJPEGLossless() bool
func (s Syntax) Name() string
```

## Best Practices

1. **Always check errors**: DICOM files can be malformed or use unsupported features
2. **Use high-level IODs**: Prefer `NewCTImage()` over manual dataset construction
3. **Validate input**: Use helper functions like `IsCT()` before assuming modality-specific attributes
4. **Handle multi-frame**: Always check `ds.NumberOfFrames()` before accessing pixel data
5. **Compression trade-offs**: JPEG-LS offers the best balance of compression and compatibility
6. **UID generation**: Use `GenerateUID()` for creating unique identifiers
7. **Window/Level**: Always provide sensible defaults for display
8. **Metadata completeness**: Fill in all required modules for DICOM compliance

## Further Reading

- [DICOM Standard](https://www.dicomstandard.org/) - Official specification
- [Part 3: Information Object Definitions](https://dicom.nema.org/medical/dicom/current/output/html/part03.html)
- [Part 5: Data Structures and Encoding](https://dicom.nema.org/medical/dicom/current/output/html/part05.html)
- [Part 6: Data Dictionary](https://dicom.nema.org/medical/dicom/current/output/html/part06.html)

## Related Packages

- `pkg/dicom/tag` - DICOM tag definitions
- `pkg/dicom/transfer` - Transfer syntax handling
- `pkg/dicom/vr` - Value Representation utilities
- `pkg/dicom/module` - DICOM Information Modules
- `pkg/compress/*` - Compression codec implementations (JPEG-LS, JPEG-LI, RLE, JPEG 2000)
