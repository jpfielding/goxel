## DICOM Tag Value Types

DICOM tags can return values as different Go types depending on the VR (Value Representation):

| Tag | Expected Type | Actual Type (suyashkumar/dicom) |
|-----|---------------|--------------------------------|
| InstanceNumber (0020,0013) | int | `[]string` (VR: IS) |
| SliceLocation (0020,1041) | float64 | `[]string` (VR: DS) |
| PixelSpacing (0028,0030) | []float64 | `[]string` (VR: DS) |
| Rows/Columns | int | `[]int` (VR: US) |

**Solution**: Always handle both `[]int`/`[]float64` and `[]string` cases when extracting values.

## Multi-File Series Challenges

### Slice Ordering

Files within a series are NOT guaranteed to be in physical order. Must sort by:
1. **InstanceNumber** (0020,0013) - acquisition order
2. **SliceLocation** (0020,1041) - physical position (fallback)

### Multi-Planar Localizers

Some series like "3 Plane Localizer" (3_PL_LOC) contain slices in **multiple orientations** (axial, coronal, sagittal). These should NOT be stacked as a single 3D volume.

Detection: Check if SliceLocation values "reset" (non-monotonic) across instance numbers:
```
Instance 1: SliceLoc=-60  (axial set starts)
Instance 2: SliceLoc=-45
...
Instance 9: SliceLoc=60   (axial set ends)
Instance 10: SliceLoc=-60 (coronal set starts - RESET!)
```

**Solution**: Group slices by ImageOrientationPatient (0020,0037) to identify orientation changes.

#### Determining Orientation from ImageOrientationPatient

ImageOrientationPatient contains 6 floats: row direction cosines (3) + column direction cosines (3).
Calculate the normal vector using cross product to determine the primary orientation:

```go
func getOrientationKey(orientation []float64) string {
    rowX, rowY, rowZ := orientation[0], orientation[1], orientation[2]
    colX, colY, colZ := orientation[3], orientation[4], orientation[5]

    // Normal vector = row × col
    normalX := rowY*colZ - rowZ*colY
    normalY := rowZ*colX - rowX*colZ
    normalZ := rowX*colY - rowY*colX

    absX, absY, absZ := abs(normalX), abs(normalY), abs(normalZ)

    if absZ >= absX && absZ >= absY {
        return "AX"  // Axial (normal along Z)
    } else if absY >= absX && absY >= absZ {
        return "COR" // Coronal (normal along Y)
    }
    return "SAG"     // Sagittal (normal along X)
}
```

Include the orientation in the volume name: `SeriesDescription_AX`, `SeriesDescription_COR`, `SeriesDescription_SAG`

### Voxel Spacing Calculation

For multi-file series, Z spacing must be calculated from SliceLocation differences:

```go
// Calculate average spacing between consecutive slices
var totalSpacing float64
for i := 1; i < len(locations); i++ {
    spacing := math.Abs(locations[i] - locations[i-1])
    totalSpacing += spacing
}
avgSpacing := totalSpacing / float64(len(locations)-1)
```

**Caveat**: This only works for single-orientation series. Multi-planar series will give incorrect results.

## Frame Extraction

The `suyashkumar/dicom` library provides two frame types:

1. **NativeFrame** - Uncompressed pixel data with direct array access
2. **EncapsulatedFrame** - Compressed data (JPEG, etc.) requiring decode

Always try native first, then fall back to encapsulated:
```go
nativeFrame, err := frame.GetNativeFrame()
if err != nil {
    encFrame, err := frame.GetEncapsulatedFrame()
    // Decode with appropriate decoder based on Transfer Syntax
}
```

## Window/Level Calculation

DICOM files may or may not include WindowCenter/WindowWidth tags. For robust handling:
1. Try DICOM tags first (WindowCenter: 0028,1050, WindowWidth: 0028,1051)
2. Fall back to calculating from pixel data statistics (mean, std dev)
