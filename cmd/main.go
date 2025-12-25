package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"fyne.io/fyne/v2/app"
	"github.com/jpfielding/goxel/pkg/dicom"
	"github.com/jpfielding/goxel/pkg/goxel"
	"github.com/jpfielding/goxel/pkg/logging"
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
		Short: "a CLI to manage clearscan configuration/validation",
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

			if inputDir == "" || outputPath == "" {
				return fmt.Errorf("both --input and --output are required")
			}

			slog.InfoContext(ctx, "Merging DICOM slices", "input", inputDir, "output", outputPath, "compress", compress)

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

			slog.InfoContext(ctx, "Loaded volume",
				"name", primary.Name,
				"width", primary.Width,
				"height", primary.Height,
				"depth", primary.Depth,
				"voxelX", primary.VoxelSizeX,
				"voxelY", primary.VoxelSizeY,
				"voxelZ", primary.VoxelSizeZ)

			// Create MR Image IOD
			mr := dicom.NewMRImage()
			mr.UseCompression = compress
			mr.CompressionCodec = codec

			// Copy metadata from first slice
			firstSlice, err := loadFirstSlice(inputDir)
			if err != nil {
				slog.WarnContext(ctx, "Could not load first slice for metadata", "error", err)
			} else {
				mr.CopyMetadataFrom(firstSlice)
			}

			// Override series description to indicate merged
			mr.Series.SeriesDescription = primary.Name + " (merged)"

			// Set pixel data
			mr.SetPixelData(primary.Height, primary.Width, primary.Data)

			// Set image plane info
			mr.ImagePlane.PixelSpacing = [2]float64{primary.VoxelSizeY, primary.VoxelSizeX}
			mr.ImagePlane.SliceThickness = primary.VoxelSizeZ
			mr.ImagePlane.SpacingBetweenSlices = primary.VoxelSizeZ

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
	return cmd
}

// loadFirstSlice loads the first DICOM file from a directory for metadata extraction
func loadFirstSlice(dir string) (*dicom.Dataset, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.ToLower(entry.Name())
		if strings.HasSuffix(name, ".dcm") || strings.HasSuffix(name, ".dcs") {
			return dicom.ReadFile(dir + "/" + entry.Name())
		}
	}
	return nil, fmt.Errorf("no DICOM files found in %s", dir)
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
