package upscaler

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

// UpscalerOptions contains options for the upscaler
type UpscalerOptions struct {
	// Scale factor (2, 3, or 4)
	Scale int
	// Model to use (e.g., "realesrgan-x4plus", "realesrgan-x4plus-anime")
	Model string
	// Number of threads to use (0 for auto)
	Threads int
	// GPU ID to use (-1 for CPU)
	GPUID int
	// Batch size for processing (number of frames to process in parallel)
	BatchSize int
}

// DefaultOptions returns default upscaler options
func DefaultOptions() UpscalerOptions {
	return UpscalerOptions{
		Scale:     4,
		Model:     "realesrgan-x4plus-anime",
		Threads:   0, // Auto
		GPUID:     0, // First GPU
		BatchSize: 10,
	}
}

// UpscaleFrames upscales all frames in the input directory and saves them to the output directory
func UpscaleFrames(inputDir, outputDir string, options UpscalerOptions) error {
	// Get the path to the realesrgan executable
	exePath, err := getRealesrganPath()
	if err != nil {
		return err
	}

	// Create output directory if it doesn't exist
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Get all PNG files in the input directory
	files, err := filepath.Glob(filepath.Join(inputDir, "*.png"))
	if err != nil {
		return fmt.Errorf("failed to list input files: %w", err)
	}

	if len(files) == 0 {
		return fmt.Errorf("no PNG files found in input directory: %s", inputDir)
	}

	// Process files in batches to avoid memory issues
	batchSize := options.BatchSize
	if batchSize <= 0 {
		batchSize = 10 // Default to 10 if invalid batch size
	}
	totalFiles := len(files)

	fmt.Printf("Found %d frames to upscale\n", totalFiles)
	fmt.Printf("Using model: %s with scale: %d\n", options.Model, options.Scale)
	fmt.Printf("Processing in batches of %d files\n", batchSize)

	for i := 0; i < totalFiles; i += batchSize {
		end := i + batchSize
		if end > totalFiles {
			end = totalFiles
		}

		batch := files[i:end]
		fmt.Printf("Processing batch %d/%d (files %d-%d of %d)\n",
			(i/batchSize)+1, (totalFiles+batchSize-1)/batchSize, i+1, end, totalFiles)

		// Process batch
		var wg sync.WaitGroup
		for _, file := range batch {
			wg.Add(1)
			go func(inputFile string) {
				defer wg.Done()

				// Get the base filename
				baseName := filepath.Base(inputFile)
				outputFile := filepath.Join(outputDir, baseName)

				// Prepare the command
				cmd := exec.Command(
					exePath,
					"-i", inputFile,
					"-o", outputFile,
					"-n", options.Model,
					"-s", fmt.Sprintf("%d", options.Scale),
					"-t", fmt.Sprintf("%d", options.Threads),
					"-g", fmt.Sprintf("%d", options.GPUID),
				)

				// Run the command
				output, err := cmd.CombinedOutput()
				if err != nil {
					fmt.Printf("Error upscaling %s: %v\n%s\n", baseName, err, string(output))
				}
			}(file)
		}

		// Wait for all files in the batch to be processed
		wg.Wait()
	}

	return nil
}

// getRealesrganPath returns the path to the realesrgan executable
func getRealesrganPath() (string, error) {
	// Check if the executable is in the realesrgan_win directory
	exePath := filepath.Join("realesrgan_win", "realesrgan-ncnn-vulkan.exe")
	if _, err := os.Stat(exePath); err == nil {
		absPath, err := filepath.Abs(exePath)
		if err != nil {
			return "", fmt.Errorf("failed to get absolute path: %w", err)
		}
		return absPath, nil
	}

	// Check if the executable is in the PATH
	path, err := exec.LookPath("realesrgan-ncnn-vulkan.exe")
	if err == nil {
		return path, nil
	}

	return "", fmt.Errorf("realesrgan-ncnn-vulkan.exe not found in realesrgan_win directory or PATH")
}

// IsRealesrganInstalled checks if realesrgan is installed
func IsRealesrganInstalled() bool {
	_, err := getRealesrganPath()
	return err == nil
}

// GetAvailableModels returns a list of available models
func GetAvailableModels() []string {
	// These are the standard models that come with realesrgan-ncnn-vulkan
	return []string{
		"realesrgan-x4plus",
		"realesrgan-x4plus-anime",
		"realesrgan-animevideov3",
		"realesr-animevideov3",
	}
}

// CreateUpscaledDir creates a directory for upscaled frames
func CreateUpscaledDir(framesDir string) (string, error) {
	// Create a directory named "upscaled" inside the frames directory
	upscaledDir := filepath.Join(framesDir, "upscaled")

	// Create the directory if it doesn't exist
	if err := os.MkdirAll(upscaledDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create upscaled directory: %w", err)
	}

	return upscaledDir, nil
}
