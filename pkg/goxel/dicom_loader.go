package goxel

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jpfielding/goxel/pkg/compress/jpegli"
	"github.com/jpfielding/goxel/pkg/compress/jpegls"
	"github.com/jpfielding/goxel/pkg/dicom"
	"github.com/jpfielding/goxel/pkg/dicom/tag"
)

// LoadDICOM loads a single DICOM file and returns a ScanCollection.
func LoadDICOM(path string) (*ScanCollection, error) {
	ds, err := dicom.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("parsing DICOM: %w", err)
	}

	return parseDICOMDataset(ds, path)
}

// dicomFileInfo holds metadata needed to sort DICOM files before loading
type dicomFileInfo struct {
	Path           string
	VolumeName     string
	InstanceNumber int
	SliceLocation  float64
	Orientation    string // ImageOrientationPatient as string key for grouping
}

// LoadDICOMDir loads all DICOM files from a directory (recursively) into a single ScanCollection.
// Files are sorted by InstanceNumber within each series before merging.
func LoadDICOMDir(dir string) (*ScanCollection, error) {
	// Collect .dcs and .dcm files recursively
	var files []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // Skip directories we can't read
		}
		if d.IsDir() {
			return nil
		}
		name := strings.ToLower(d.Name())
		if strings.HasSuffix(name, ".dcs") || strings.HasSuffix(name, ".dcm") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking directory: %w", err)
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no DICOM files found in %s", dir)
	}

	slog.Info("Found DICOM files", "count", len(files), "dir", dir)

	// First pass: extract metadata for sorting
	var fileInfos []dicomFileInfo
	for _, file := range files {
		info, err := extractDICOMFileInfo(file)
		if err != nil {
			slog.Warn("Failed to read DICOM metadata", "file", file, "error", err)
			continue
		}
		fileInfos = append(fileInfos, info)
	}

	// Sort by volume name, then by instance number (or slice location)
	sort.Slice(fileInfos, func(i, j int) bool {
		if fileInfos[i].VolumeName != fileInfos[j].VolumeName {
			return fileInfos[i].VolumeName < fileInfos[j].VolumeName
		}
		// Sort by instance number first, fall back to slice location
		if fileInfos[i].InstanceNumber != fileInfos[j].InstanceNumber {
			return fileInfos[i].InstanceNumber < fileInfos[j].InstanceNumber
		}
		return fileInfos[i].SliceLocation < fileInfos[j].SliceLocation
	})

	slog.Info("Sorted DICOM files by instance number")

	result := NewScanCollection()
	result.SourcePath = dir

	// Second pass: load files in sorted order
	for _, info := range fileInfos {
		scan, err := LoadDICOM(info.Path)
		if err != nil {
			slog.Warn("Failed to load DICOM file", "file", info.Path, "error", err)
			continue
		}
		mergeScan(result, scan)
	}

	if len(result.Volumes) == 0 {
		return nil, fmt.Errorf("no loadable DICOM files found in %s", dir)
	}

	// Calculate Z spacing from slice locations for each volume
	// Group fileInfos by volume name and compute average spacing
	volumeSliceLocations := make(map[string][]float64)
	for _, info := range fileInfos {
		volumeSliceLocations[info.VolumeName] = append(volumeSliceLocations[info.VolumeName], info.SliceLocation)
	}

	for name, locations := range volumeSliceLocations {
		vol, ok := result.Volumes[name]
		if !ok || len(locations) < 2 {
			continue
		}
		// Calculate average spacing between consecutive slices
		var totalSpacing float64
		for i := 1; i < len(locations); i++ {
			spacing := locations[i] - locations[i-1]
			if spacing < 0 {
				spacing = -spacing // Absolute value
			}
			totalSpacing += spacing
		}
		avgSpacing := totalSpacing / float64(len(locations)-1)
		if avgSpacing > 0 {
			vol.VoxelSizeZ = avgSpacing
			slog.Debug("Calculated Z spacing from slice locations", "volume", name, "spacing", avgSpacing)
		}
	}

	// Log final volume summary
	for name, vol := range result.Volumes {
		slog.Info("Loaded volume", "name", name, "width", vol.Width, "height", vol.Height, "depth", vol.Depth,
			"voxelX", vol.VoxelSizeX, "voxelY", vol.VoxelSizeY, "voxelZ", vol.VoxelSizeZ)
	}

	return result, nil
}

// extractDICOMFileInfo reads just the metadata needed for sorting
func extractDICOMFileInfo(path string) (dicomFileInfo, error) {
	ds, err := dicom.ReadFile(path)
	if err != nil {
		return dicomFileInfo{}, err
	}

	// Get orientation for grouping multi-planar series
	orientation := getOrientationKey(ds)

	// Include orientation in volume name for multi-planar series
	baseName := determineVolumeNameFromDataset(ds, path)
	volumeName := baseName
	if orientation != "" {
		volumeName = baseName + "_" + orientation
	}

	return dicomFileInfo{
		Path:           path,
		VolumeName:     volumeName,
		InstanceNumber: dicom.GetInstanceNumber(ds),
		SliceLocation:  dicom.GetSliceLocation(ds),
		Orientation:    orientation,
	}, nil
}

// getOrientationKey returns a short string identifying the image orientation
func getOrientationKey(ds *dicom.Dataset) string {
	orientation := dicom.GetImageOrientationPatient(ds)
	if len(orientation) < 6 {
		return ""
	}

	// Determine primary orientation from direction cosines
	// ImageOrientationPatient contains row and column direction cosines
	rowX, rowY, rowZ := orientation[0], orientation[1], orientation[2]
	colX, colY, colZ := orientation[3], orientation[4], orientation[5]

	// Calculate normal vector (cross product)
	normalX := rowY*colZ - rowZ*colY
	normalY := rowZ*colX - rowX*colZ
	normalZ := rowX*colY - rowY*colX

	// Determine primary axis
	absX, absY, absZ := abs(normalX), abs(normalY), abs(normalZ)

	if absZ >= absX && absZ >= absY {
		return "AX" // Axial (normal along Z)
	} else if absY >= absX && absY >= absZ {
		return "COR" // Coronal (normal along Y)
	} else {
		return "SAG" // Sagittal (normal along X)
	}
}

// Load loads DICOM data from a file or directory path.
func Load(path string) (*ScanCollection, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	if info.IsDir() {
		return LoadDICOMDir(path)
	}
	return LoadDICOM(path)
}

func parseDICOMDataset(ds *dicom.Dataset, sourcePath string) (*ScanCollection, error) {
	scan := NewScanCollection()
	scan.SourcePath = sourcePath

	// Extract DICOM Patient Module
	scan.PatientID = dicom.GetPatientID(ds)
	scan.PatientName = dicom.GetPatientName(ds)

	// Extract DICOM Study Module
	scan.StudyInstanceUID = getStringValue(ds, tag.StudyInstanceUID)
	scan.StudyDate = dicom.GetStudyDate(ds)
	scan.StudyTime = getStringValue(ds, tag.StudyTime)
	scan.StudyDescription = dicom.GetStudyDescription(ds)

	// Extract DICOM Series Module
	scan.SeriesInstanceUID = getStringValue(ds, tag.SeriesInstanceUID)
	scan.SeriesDescription = dicom.GetSeriesDescription(ds)
	scan.SeriesNumber = getIntValue(ds, tag.SeriesNumber)
	scan.Modality = dicom.GetModality(ds)

	// Extract DICOM General Equipment Module
	scan.Manufacturer = getStringValue(ds, tag.Manufacturer)
	scan.InstitutionName = getStringValue(ds, tag.InstitutionName)
	scan.ManufacturerModel = getStringValue(ds, tag.ManufacturerModelName)

	// Extract DICOM Image Module
	scan.AcquisitionDate = getStringValue(ds, tag.Tag{Group: 0x0008, Element: 0x0022}) // AcquisitionDate
	scan.AcquisitionTime = getStringValue(ds, tag.Tag{Group: 0x0008, Element: 0x0032}) // AcquisitionTime

	// Additional Info
	scan.OperatorName = getStringValue(ds, tag.Tag{Group: 0x0008, Element: 0x1070}) // OperatorsName

	rows := dicom.GetRows(ds)
	cols := dicom.GetColumns(ds)
	numFrames := dicom.GetNumberOfFrames(ds)
	pixelRep := dicom.GetPixelRepresentation(ds)
	bitsAllocated := dicom.GetBitsAllocated(ds)
	bitsStored := getIntValueDefault(ds, tag.BitsStored, bitsAllocated)

	slog.Debug("DICOM metadata",
		slog.Int("rows", rows),
		slog.Int("cols", cols),
		slog.Int("frames", numFrames),
		slog.Int("pixelRep", pixelRep),
		slog.Int("bitsAllocated", bitsAllocated),
		slog.Int("bitsStored", bitsStored))

	// Get pixel data
	pixelData, err := ds.GetPixelData()
	if err != nil {
		return nil, fmt.Errorf("getting pixel data: %w", err)
	}

	if pixelData == nil || len(pixelData.Frames) == 0 {
		return nil, fmt.Errorf("no frames in pixel data")
	}

	// Determine actual frame count and dimensions
	depth := len(pixelData.Frames)
	if numFrames > 1 && depth == 1 {
		// Multi-frame encoded as single frame
		depth = numFrames
	}

	// Convert frames to uint16 data
	var volumeData []uint16
	actualWidth, actualHeight := cols, rows

	for _, frame := range pixelData.Frames {
		// Native (uncompressed) frame
		if !pixelData.IsEncapsulated && len(frame.Data) > 0 {
			volumeData = append(volumeData, frame.Data...)
			continue
		}

		// Encapsulated (compressed) frame
		if len(frame.CompressedData) > 0 {
			slog.Debug("Got encapsulated frame", "dataLen", len(frame.CompressedData))

			// Log first few bytes to identify format
			if len(frame.CompressedData) >= 4 {
				slog.Debug("Frame header bytes",
					"b0", fmt.Sprintf("%02X", frame.CompressedData[0]),
					"b1", fmt.Sprintf("%02X", frame.CompressedData[1]),
					"b2", fmt.Sprintf("%02X", frame.CompressedData[2]),
					"b3", fmt.Sprintf("%02X", frame.CompressedData[3]))
			}

			// Try standard JPEG first (baseline/progressive)
			img, jpegErr := jpeg.Decode(bytes.NewReader(frame.CompressedData))
			if jpegErr != nil {
				slog.Debug("Standard JPEG decode failed", "error", jpegErr)
				// Try JPEG Lossless (T.81 SOF3) decoder - used by DICOM Transfer Syntax 1.2.840.10008.1.2.4.70
				var jpegliErr error
				img, jpegliErr = jpegli.Decode(bytes.NewReader(frame.CompressedData))
				if jpegliErr != nil {
					slog.Debug("JPEG Lossless (jpegli) decode failed", "error", jpegliErr)
					// Try JPEG-LS (T.87 SOF55) decoder
					var jpeglsErr error
					img, jpeglsErr = jpegls.Decode(bytes.NewReader(frame.CompressedData))
					if jpeglsErr != nil {
						slog.Warn("Failed to decode frame", "jpegErr", jpegErr, "jpegliErr", jpegliErr, "jpeglsErr", jpeglsErr)
						continue
					}
				}
			}

			bounds := img.Bounds()
			actualWidth = bounds.Dx()
			actualHeight = bounds.Dy()

			// Extract grayscale values from decoded image
			frameData := extractGrayscaleFromImage(img)
			volumeData = append(volumeData, frameData...)
			continue
		}

		slog.Warn("Skipping frame - neither native nor encapsulated")
	}

	if len(volumeData) == 0 {
		return nil, fmt.Errorf("no volume data extracted")
	}

	// Calculate actual depth from data
	sliceSize := actualWidth * actualHeight
	if sliceSize > 0 {
		depth = len(volumeData) / sliceSize
	}

	// Include orientation in volume name for multi-planar series
	baseName := determineVolumeNameFromDataset(ds, sourcePath)
	orientation := getOrientationKey(ds)
	volName := baseName
	if orientation != "" {
		volName = baseName + "_" + orientation
	}

	vol := &VolumeData{
		Name:       volName,
		SourcePath: sourcePath,
		Width:      actualWidth,
		Height:     actualHeight,
		Depth:      depth,
		Data:       volumeData,
		PixelRep:   pixelRep,
	}

	// Get rescale values
	intercept, slope := dicom.GetRescale(ds)
	vol.RescaleIntercept = intercept
	vol.RescaleSlope = slope

	// Get pixel spacing
	pixelSpacingRow, pixelSpacingCol := dicom.GetPixelSpacing(ds)
	vol.VoxelSizeY = pixelSpacingRow
	vol.VoxelSizeX = pixelSpacingCol
	vol.VoxelSizeZ = dicom.GetSliceThickness(ds)
	if vol.VoxelSizeZ == 0 {
		vol.VoxelSizeZ = 1.0
	}

	scan.Volumes[volName] = vol

	// Calculate window/level
	wl, ww := CalculateWindowFromData(vol.Data)
	scan.WindowLevel = wl
	scan.WindowWidth = ww

	slog.Info("Loaded DICOM",
		slog.String("path", sourcePath),
		slog.Int("width", vol.Width),
		slog.Int("height", vol.Height),
		slog.Int("depth", vol.Depth),
		slog.Float64("windowLevel", wl),
		slog.Float64("windowWidth", ww))

	return scan, nil
}

// determineVolumeNameFromDataset uses DICOM metadata to determine volume name.
// Uses SeriesDescription or SeriesInstanceUID for grouping files from the same series.
func determineVolumeNameFromDataset(ds *dicom.Dataset, path string) string {
	// Try SeriesDescription first (human readable)
	seriesDesc := dicom.GetSeriesDescription(ds)
	if seriesDesc != "" {
		// Use series description as volume name (simplified)
		return strings.ReplaceAll(seriesDesc, " ", "_")
	}

	// Try SeriesInstanceUID for grouping
	seriesUID := getStringValue(ds, tag.SeriesInstanceUID)
	if seriesUID != "" {
		// Use last part of UID to keep it short
		parts := strings.Split(seriesUID, ".")
		if len(parts) > 2 {
			return "series_" + parts[len(parts)-1]
		}
		return "series_" + seriesUID
	}

	// Default: use a generic name so all files merge into one volume
	return "volume"
}

func mergeScan(result, scan *ScanCollection) {
	// Merge DICOM Patient Module
	if result.PatientID == "" && scan.PatientID != "" {
		result.PatientID = scan.PatientID
	}
	if result.PatientName == "" && scan.PatientName != "" {
		result.PatientName = scan.PatientName
	}

	// Merge DICOM Study Module
	if result.StudyInstanceUID == "" && scan.StudyInstanceUID != "" {
		result.StudyInstanceUID = scan.StudyInstanceUID
	}
	if result.StudyDate == "" && scan.StudyDate != "" {
		result.StudyDate = scan.StudyDate
	}
	if result.StudyTime == "" && scan.StudyTime != "" {
		result.StudyTime = scan.StudyTime
	}
	if result.StudyDescription == "" && scan.StudyDescription != "" {
		result.StudyDescription = scan.StudyDescription
	}

	// Merge DICOM Series Module
	if result.SeriesInstanceUID == "" && scan.SeriesInstanceUID != "" {
		result.SeriesInstanceUID = scan.SeriesInstanceUID
	}
	if result.SeriesDescription == "" && scan.SeriesDescription != "" {
		result.SeriesDescription = scan.SeriesDescription
	}
	if result.SeriesNumber == 0 && scan.SeriesNumber != 0 {
		result.SeriesNumber = scan.SeriesNumber
	}
	if result.Modality == "" && scan.Modality != "" {
		result.Modality = scan.Modality
	}

	// Merge DICOM General Equipment Module
	if result.Manufacturer == "" && scan.Manufacturer != "" {
		result.Manufacturer = scan.Manufacturer
	}
	if result.InstitutionName == "" && scan.InstitutionName != "" {
		result.InstitutionName = scan.InstitutionName
	}
	if result.ManufacturerModel == "" && scan.ManufacturerModel != "" {
		result.ManufacturerModel = scan.ManufacturerModel
	}

	// Merge DICOM Image Module
	if result.AcquisitionDate == "" && scan.AcquisitionDate != "" {
		result.AcquisitionDate = scan.AcquisitionDate
	}
	if result.AcquisitionTime == "" && scan.AcquisitionTime != "" {
		result.AcquisitionTime = scan.AcquisitionTime
	}

	// Additional Info
	if result.OperatorName == "" && scan.OperatorName != "" {
		result.OperatorName = scan.OperatorName
	}

	// Merge volumes - if same name exists, append slices (stack in Z)
	for name, vol := range scan.Volumes {
		if existing, ok := result.Volumes[name]; ok {
			// Same dimensions? Stack the slices
			if existing.Width == vol.Width && existing.Height == vol.Height {
				existing.Data = append(existing.Data, vol.Data...)
				existing.Depth += vol.Depth

				// Propagate voxel sizes if not set in first file
				if existing.VoxelSizeX <= 0 && vol.VoxelSizeX > 0 {
					existing.VoxelSizeX = vol.VoxelSizeX
				}
				if existing.VoxelSizeY <= 0 && vol.VoxelSizeY > 0 {
					existing.VoxelSizeY = vol.VoxelSizeY
				}

				slog.Debug("Merged volume slice",
					slog.String("name", name),
					slog.Int("newDepth", existing.Depth))
			} else {
				// Different dimensions - keep as separate volume with unique name
				uniqueName := fmt.Sprintf("%s_%d", name, len(result.Volumes))
				result.Volumes[uniqueName] = vol
			}
		} else {
			result.Volumes[name] = vol
		}
	}

	result.Findings = append(result.Findings, scan.Findings...)

	for name, proj := range scan.Projections {
		result.Projections[name] = proj
	}

	if result.WindowLevel == 500 && scan.WindowLevel != 500 {
		result.WindowLevel = scan.WindowLevel
	}
	if result.WindowWidth == 2000 && scan.WindowWidth != 2000 {
		result.WindowWidth = scan.WindowWidth
	}
}

// Helper functions to extract DICOM tag values

func getStringValue(ds *dicom.Dataset, t tag.Tag) string {
	if elem, ok := ds.FindElement(t.Group, t.Element); ok {
		if s, ok := elem.GetString(); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func getIntValue(ds *dicom.Dataset, t tag.Tag) int {
	if elem, ok := ds.FindElement(t.Group, t.Element); ok {
		if v, ok := elem.GetInt(); ok {
			return v
		}
	}
	return 0
}

func getIntValueDefault(ds *dicom.Dataset, t tag.Tag, defaultVal int) int {
	if elem, ok := ds.FindElement(t.Group, t.Element); ok {
		if v, ok := elem.GetInt(); ok {
			return v
		}
	}
	return defaultVal
}

// extractGrayscaleFromImage extracts uint16 grayscale values from a decoded image
func extractGrayscaleFromImage(img image.Image) []uint16 {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	data := make([]uint16, width*height)

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			// Get RGBA values (16-bit each)
			r, g, b, _ := img.At(x+bounds.Min.X, y+bounds.Min.Y).RGBA()
			// Convert to grayscale using luminance formula
			// RGBA() returns 16-bit values, so divide by 256 to get 8-bit equivalent
			gray := (r*299 + g*587 + b*114) / 1000
			// Scale to 16-bit range for consistency with native DICOM data
			data[y*width+x] = uint16(gray)
		}
	}
	return data
}
