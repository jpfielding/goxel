package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httputil"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"fyne.io/fyne/v2/app"
	"github.com/jpfielding/goxel/pkg/dicom"
	"github.com/jpfielding/goxel/pkg/goxel"
	"github.com/jpfielding/goxel/pkg/logging"
	"github.com/jpfielding/goxel/pkg/volume"
	"github.com/spf13/cobra"
)

const defaultHTTPTimeout = 30 * time.Second

var (
	GitSHA string = "NA"
)

func main() {
	// register sigterm for graceful shutdown
	ctx, cnc := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cnc()
	go func() {
		defer cnc() // this cnc is from notify and removes the signal so subsequent ctrl-c will restore kill functions
		<-ctx.Done()
	}()
	slog.SetDefault(logging.Logger(os.Stdout, false, slog.LevelInfo))
	ctx = logging.AppendCtx(ctx,
		slog.Group("goxel",
			slog.String("name", "ctl"),
			slog.String("git", GitSHA),
		))
	NewRoot(ctx, GitSHA).Execute()
}

func NewRoot(ctx context.Context, gitsha string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dicomctl",
		Short: "a CLI for DICOM image viewing and processing",
		Long:  "the long story",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			logLevel, _ := cmd.Flags().GetString("log-level")

			// Parse log level
			var level slog.Level
			if err := level.UnmarshalText([]byte(strings.ToUpper(logLevel))); err != nil {
				level = slog.LevelInfo
			}
			slog.SetDefault(logging.Logger(os.Stdout, false, level))

			if err := level.UnmarshalText([]byte(strings.ToUpper(logLevel))); err != nil {
				slog.WarnContext(ctx, "Invalid log level, defaulting to INFO", "level", logLevel, "error", err)
			}

		},
		Run: func(cmd *cobra.Command, args []string) {
			printCommandTree(cmd, 0)
		},
	}
	cmd.AddCommand(
		NewVersionCmd(ctx, gitsha),
		NewDecodeCmd(ctx),
		NewGoxelCmd(ctx),
		NewMergeCmd(ctx),
	)
	pf := cmd.PersistentFlags()
	pf.String("log-level", "INFO", "Log level (DEBUG, INFO, WARN, ERROR)")
	return cmd
}

func NewVersionCmd(ctx context.Context, gitsha string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "git sha for this build",
		Long:  "git sha for this build",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(gitsha)
		},
	}
	return cmd
}

func printCommandTree(cmd *cobra.Command, indent int) {
	fmt.Println(strings.Repeat("\t", indent), cmd.Use+":", cmd.Short)
	for _, subCmd := range cmd.Commands() {
		printCommandTree(subCmd, indent+1)
	}
}

// NewDecodeCmd is a command to decode DICOM files
func NewDecodeCmd(ctx context.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "decode",
		Short: "DICOM decode",
		Long:  "DICOM decode",
		RunE: func(cmd *cobra.Command, args []string) error {
			var in io.Reader
			dcsPath, _ := cmd.Flags().GetString("uri")
			dcsPath = strings.TrimPrefix(dcsPath, "file://")
			switch {
			case dcsPath == "-":
				in = os.Stdin
			case strings.HasPrefix(dcsPath, "http"):
				cl := &http.Client{
					Timeout: defaultHTTPTimeout,
					Transport: &http.Transport{
						TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
					},
				}
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, dcsPath, nil)
				if err != nil {
					return fmt.Errorf("failed to create request: %v", err)
				}
				resp, err := cl.Do(req)
				if err != nil {
					return fmt.Errorf("failed to download: %v", err)
				}
				verbose, _ := cmd.Flags().GetBool("verbose")
				if verbose {
					reqDump, _ := httputil.DumpRequest(req, true)
					os.Stderr.Write(reqDump)
					resDump, _ := httputil.DumpResponse(resp, false)
					os.Stderr.Write(resDump)
				}
				in = resp.Body
				defer resp.Body.Close()
			default:
				f, err := os.Open(dcsPath)
				if err != nil {
					return fmt.Errorf("failed to open file: %v", err)
				}
				in = f
				defer f.Close()
			}
			dataset, err := dicom.Parse(in)
			if err != nil {
				return fmt.Errorf("failed to parse DICOM: %v", err)
			}
			switch fType, _ := cmd.Flags().GetString("format"); fType {
			case "text": // Dataset will nicely print the DICOM dataset data out of the box.
				fmt.Println(dataset.String())
			default: // Dataset is also JSON serializable out of the box.
				j, _ := json.Marshal(dataset)
				os.Stdout.Write(j)
			}
			return nil
		},
	}
	pf := cmd.PersistentFlags()
	pf.StringP("uri", "u", "", "DICOM URI to fetch certificates from")
	pf.StringP("format", "f", "json", "output format (text|json)")
	return cmd
}

// NewMergeCmd creates a command to merge multiple DICOM slices into a single multi-frame file
func NewMergeCmd(ctx context.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "merge",
		Short: "Merge DICOM slices into multi-frame file",
		Long:  `Merge multiple DICOM slice files into a single multi-frame DICOM file with optional compression`,
		RunE: func(cmd *cobra.Command, args []string) error {
			inputDir, _ := cmd.Flags().GetString("input")
			outputPath, _ := cmd.Flags().GetString("output")
			compress, _ := cmd.Flags().GetBool("compress")
			codec, _ := cmd.Flags().GetString("codec")
			mode, _ := cmd.Flags().GetString("mode")

			if inputDir == "" || outputPath == "" {
				return fmt.Errorf("both --input and --output are required")
			}

			slog.InfoContext(ctx, "Merging DICOM slices", "input", inputDir, "output", outputPath, "compress", compress, "mode", mode)

			// Load all slices from directory using goxel loader
			scan, err := goxel.LoadDICOMDir(inputDir)
			if err != nil {
				return fmt.Errorf("failed to load DICOM directory: %w", err)
			}

			// Get the primary volume
			primary := scan.PrimaryVolume()
			if primary == nil {
				return fmt.Errorf("no volume data found")
			}

			mergedVolume := primary
			switch mode {
			case "", "primary":
				// Keep legacy behavior.
			case "all-max":
				mergedVolume, err = buildAllMaxComposite(scan, primary)
				if err != nil {
					return fmt.Errorf("failed to build all-max composite: %w", err)
				}
			case "all-blend":
				mergedVolume, err = buildAllBlendComposite(scan, primary)
				if err != nil {
					return fmt.Errorf("failed to build all-blend composite: %w", err)
				}
			default:
				return fmt.Errorf("invalid mode %q (expected primary, all-max, or all-blend)", mode)
			}

			slog.InfoContext(ctx, "Using merged volume",
				"name", mergedVolume.Name,
				"width", mergedVolume.Width,
				"height", mergedVolume.Height,
				"depth", mergedVolume.Depth,
				"voxelX", mergedVolume.VoxelSizeX,
				"voxelY", mergedVolume.VoxelSizeY,
				"voxelZ", mergedVolume.VoxelSizeZ,
				"volumes", len(scan.Volumes))

			// Create MR Image IOD
			mr := dicom.NewMRImage()
			if compress {
				mr.Codec = dicom.CodecByName(codec)
				if mr.Codec == nil {
					mr.Codec = dicom.CodecJPEGLS // default to JPEG-LS
				}
			}

			// Copy metadata from first slice
			firstSlice, err := loadFirstSlice(inputDir)
			if err != nil {
				slog.WarnContext(ctx, "Could not load first slice for metadata", "error", err)
			} else {
				mr.CopyMetadataFrom(firstSlice)
			}

			// Override series description to indicate merged
			mr.Series.SeriesDescription = mergedVolume.Name + " (merged)"

			// Set pixel data
			mr.SetPixelData(mergedVolume.Height, mergedVolume.Width, mergedVolume.Data)

			// Set image plane info
			mr.ImagePlane.PixelSpacing = [2]float64{mergedVolume.VoxelSizeY, mergedVolume.VoxelSizeX}
			mr.ImagePlane.SliceThickness = mergedVolume.VoxelSizeZ
			mr.ImagePlane.SpacingBetweenSlices = mergedVolume.VoxelSizeZ

			// Write output
			n, err := mr.Write(outputPath)
			if err != nil {
				return fmt.Errorf("failed to write output: %w", err)
			}

			slog.InfoContext(ctx, "Wrote multi-frame DICOM",
				"path", outputPath,
				"frames", mr.NumberOfFrames,
				"bytes", n,
				"compressed", compress)

			return nil
		},
	}
	pf := cmd.PersistentFlags()
	pf.StringP("input", "i", "", "Input directory containing DICOM slices")
	pf.StringP("output", "o", "", "Output file path for merged DICOM")
	pf.BoolP("compress", "c", true, "Enable JPEG-LS compression")
	pf.String("codec", "jpeg-ls", "Compression codec (jpeg-ls, jpeg-li, rle)")
	pf.String("mode", "primary", "Merge mode (primary|all-max|all-blend)")
	return cmd
}

// loadFirstSlice loads the first DICOM file found recursively in a directory.
func loadFirstSlice(dir string) (*dicom.Dataset, error) {
	var firstPath string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // Skip unreadable entries.
		}
		if d.IsDir() {
			return nil
		}
		name := strings.ToLower(d.Name())
		if strings.HasSuffix(name, ".dcm") || strings.HasSuffix(name, ".dcs") {
			firstPath = path
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if firstPath == "" {
		return nil, fmt.Errorf("no DICOM files found in %s", dir)
	}
	return dicom.ReadFile(firstPath)
}

func buildAllMaxComposite(scan *goxel.ScanCollection, reference *goxel.VolumeData) (*goxel.VolumeData, error) {
	if scan == nil || reference == nil {
		return nil, fmt.Errorf("scan/reference volume required")
	}
	if len(scan.Volumes) == 0 {
		return nil, fmt.Errorf("no volumes in scan")
	}

	totalVoxels := reference.Width * reference.Height * reference.Depth
	if totalVoxels <= 0 {
		return nil, fmt.Errorf("invalid reference dimensions")
	}

	composite := make([]uint16, totalVoxels)

	// Stable ordering gives reproducible output.
	names := make([]string, 0, len(scan.Volumes))
	for name := range scan.Volumes {
		names = append(names, name)
	}
	sort.Strings(names)

	refSliceSize := reference.Width * reference.Height
	const targetMaxDensity = 30000.0
	// Match renderer defaults: clip low normalized densities.
	const densityThreshold = 0.03
	for _, name := range names {
		vol := scan.Volumes[name]
		if vol == nil || vol.Width <= 0 || vol.Height <= 0 || vol.Depth <= 0 || len(vol.Data) == 0 {
			continue
		}

		wl, ww := goxel.CalculateWindowFromData(vol.Data)
		if ww <= 0 {
			continue
		}
		windowMin := wl - ww/2

		xMap := buildIndexMap(reference.Width, vol.Width)
		yMap := buildIndexMap(reference.Height, vol.Height)
		zMap := buildIndexMap(reference.Depth, vol.Depth)

		srcSliceSize := vol.Width * vol.Height

		for z := 0; z < reference.Depth; z++ {
			sz := zMap[z]
			srcBaseZ := sz * srcSliceSize
			refBaseZ := z * refSliceSize

			for y := 0; y < reference.Height; y++ {
				sy := yMap[y]
				srcBaseY := srcBaseZ + sy*vol.Width
				refBaseY := refBaseZ + y*reference.Width

				for x := 0; x < reference.Width; x++ {
					sx := xMap[x]
					srcVal := vol.Data[srcBaseY+sx]
					normF := (float64(srcVal) + vol.RescaleIntercept - windowMin) / ww
					normF = clamp01(normF)
					if normF < densityThreshold {
						continue
					}
					norm := uint16(normF*targetMaxDensity + 0.5)
					dstIdx := refBaseY + x
					if norm > composite[dstIdx] {
						composite[dstIdx] = norm
					}
				}
			}
		}
	}

	return &goxel.VolumeData{
		Name:             "ALL_VOLUMES_MAX_COMPOSITE",
		SourcePath:       reference.SourcePath,
		Width:            reference.Width,
		Height:           reference.Height,
		Depth:            reference.Depth,
		VoxelSizeX:       reference.VoxelSizeX,
		VoxelSizeY:       reference.VoxelSizeY,
		VoxelSizeZ:       reference.VoxelSizeZ,
		Data:             composite,
		PixelRep:         reference.PixelRep,
		RescaleIntercept: reference.RescaleIntercept,
		RescaleSlope:     reference.RescaleSlope,
	}, nil
}

func buildAllBlendComposite(scan *goxel.ScanCollection, reference *goxel.VolumeData) (*goxel.VolumeData, error) {
	if scan == nil || reference == nil {
		return nil, fmt.Errorf("scan/reference volume required")
	}
	if len(scan.Volumes) == 0 {
		return nil, fmt.Errorf("no volumes in scan")
	}

	totalVoxels := reference.Width * reference.Height * reference.Depth
	if totalVoxels <= 0 {
		return nil, fmt.Errorf("invalid reference dimensions")
	}

	// Keep memory bounded: fixed-point [0..65535] for both accumulated signal and alpha.
	blend := make([]uint16, totalVoxels)
	accumAlpha := make([]uint16, totalVoxels)

	// Match viewer defaults (opacity slider and per-layer alpha are both 0.5 by default).
	const globalAlpha = 0.5
	const perVolumeAlpha = 0.5
	const densityThreshold = 0.03
	const targetMaxDensity = 30000.0
	const u16Max = uint64(65535)
	const u16Squared = u16Max * u16Max
	alphaScale := globalAlpha * perVolumeAlpha

	cfg := volume.GetConfig()
	bands := volume.GetColorBandsFromConfig(cfg, "DEFAULT")
	tf := volume.CreateTransferFunctionFromBandsWithGradient(cfg, "DEFAULT", bands)
	tfMax := len(tf) - 1

	names := make([]string, 0, len(scan.Volumes))
	for name := range scan.Volumes {
		names = append(names, name)
	}
	sort.Strings(names)

	refSliceSize := reference.Width * reference.Height
	for _, name := range names {
		vol := scan.Volumes[name]
		if vol == nil || vol.Width <= 0 || vol.Height <= 0 || vol.Depth <= 0 || len(vol.Data) == 0 {
			continue
		}

		wl, ww := goxel.CalculateWindowFromData(vol.Data)
		if ww <= 0 {
			continue
		}
		windowMin := wl - ww/2

		xMap := buildIndexMap(reference.Width, vol.Width)
		yMap := buildIndexMap(reference.Height, vol.Height)
		zMap := buildIndexMap(reference.Depth, vol.Depth)
		srcSliceSize := vol.Width * vol.Height

		for z := 0; z < reference.Depth; z++ {
			sz := zMap[z]
			srcBaseZ := sz * srcSliceSize
			refBaseZ := z * refSliceSize

			for y := 0; y < reference.Height; y++ {
				sy := yMap[y]
				srcBaseY := srcBaseZ + sy*vol.Width
				refBaseY := refBaseZ + y*reference.Width

				for x := 0; x < reference.Width; x++ {
					sx := xMap[x]
					srcVal := vol.Data[srcBaseY+sx]
					normF := (float64(srcVal) + vol.RescaleIntercept - windowMin) / ww
					normF = clamp01(normF)
					if normF < densityThreshold {
						continue
					}

					// Transfer-function alpha (same source table as renderer).
					tfIdx := int(normF * float64(tfMax))
					if tfIdx < 0 {
						tfIdx = 0
					} else if tfIdx > tfMax {
						tfIdx = tfMax
					}
					tfAlpha := float64(tf[tfIdx].A) / 255.0
					sampleA := math.Pow(tfAlpha, 1.5) * alphaScale
					if sampleA <= 0 {
						continue
					}
					if sampleA > 1 {
						sampleA = 1
					}

					sampleAQ := uint16(sampleA*65535 + 0.5)
					sampleVQ := uint16(normF*65535 + 0.5)

					dstIdx := refBaseY + x
					aIn := accumAlpha[dstIdx]
					oneMinusA := uint16(65535 - aIn)
					if oneMinusA == 0 {
						continue
					}

					// Front-to-back alpha blend in fixed-point:
					// C_out = C_in + (1-A_in) * C_s * A_s
					// A_out = A_in + (1-A_in) * A_s
					deltaV := uint16((uint64(oneMinusA)*uint64(sampleVQ)*uint64(sampleAQ) + (u16Squared / 2)) / u16Squared)
					deltaA := uint16((uint64(oneMinusA)*uint64(sampleAQ) + 32767) / 65535)

					newV := uint32(blend[dstIdx]) + uint32(deltaV)
					if newV > 65535 {
						newV = 65535
					}
					blend[dstIdx] = uint16(newV)

					newA := uint32(aIn) + uint32(deltaA)
					if newA > 65535 {
						newA = 65535
					}
					accumAlpha[dstIdx] = uint16(newA)
				}
			}
		}
	}

	// Map from fixed-point [0..65535] into the transfer-function-friendly [0..30000] density range.
	out := make([]uint16, len(blend))
	for i, v := range blend {
		out[i] = uint16((float64(v)/65535.0)*targetMaxDensity + 0.5)
	}

	return &goxel.VolumeData{
		Name:             "ALL_VOLUMES_ALPHA_BLEND_COMPOSITE",
		SourcePath:       reference.SourcePath,
		Width:            reference.Width,
		Height:           reference.Height,
		Depth:            reference.Depth,
		VoxelSizeX:       reference.VoxelSizeX,
		VoxelSizeY:       reference.VoxelSizeY,
		VoxelSizeZ:       reference.VoxelSizeZ,
		Data:             out,
		PixelRep:         reference.PixelRep,
		RescaleIntercept: reference.RescaleIntercept,
		RescaleSlope:     reference.RescaleSlope,
	}, nil
}

func buildIndexMap(dstSize, srcSize int) []int {
	idx := make([]int, dstSize)
	if dstSize <= 0 {
		return idx
	}
	if dstSize == 1 || srcSize <= 1 {
		return idx
	}
	scale := float64(srcSize-1) / float64(dstSize-1)
	for i := range idx {
		mapped := int(math.Round(float64(i) * scale))
		if mapped < 0 {
			mapped = 0
		}
		if mapped >= srcSize {
			mapped = srcSize - 1
		}
		idx[i] = mapped
	}
	return idx
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// NewGoxelCmd creates the goxel command for viewing DICOM images.
func NewGoxelCmd(ctx context.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "goxel",
		Short: "Analyze DICOM images",
		Long:  `Simple DICOM viewer`,
		Run: func(cmd *cobra.Command, args []string) {
			path, _ := cmd.Flags().GetString("path")

			a := app.New()
			a.SetIcon(goxel.ResourceIconPng)

			ui := goxel.NewViewer(a)

			// Load DICOM files if path provided
			if path != "" {
				scan, err := goxel.Load(path)
				if err != nil {
					slog.ErrorContext(ctx, "Failed to load", "path", path, "error", err)
				} else {
					ui.SetScanData(scan)
				}
			}

			ui.LoadKeys()
			ui.Run()
		},
	}
	pf := cmd.PersistentFlags()
	pf.StringP("path", "p", "", "DICOM file or directory")
	return cmd
}
