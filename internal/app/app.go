package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"videoup/internal/cleanup"
	"videoup/internal/ffmpeg"
	"videoup/internal/upscaler"
)

// ProcessVideo extracts frames from a video file
func ProcessVideo(videoPath string) (string, error) {
	// Create temp directory
	tempDir, err := ffmpeg.CreateTempDir(videoPath)
	if err != nil {
		return "", err
	}

	// Register the directory for cleanup in case of errors
	cleanup.RegisterDirectory(tempDir)

	// Extract frames
	err = ffmpeg.ExtractFrames(videoPath, tempDir)
	if err != nil {
		return "", err
	}

	return tempDir, nil
}

// UpscaleFrames upscales all frames in a directory
func UpscaleFrames(inputDir string, options upscaler.UpscalerOptions) (string, error) {
	// Create upscaled directory
	upscaledDir, err := upscaler.CreateUpscaledDir(inputDir)
	if err != nil {
		return "", err
	}

	// Register the directory for cleanup in case of errors
	cleanup.RegisterDirectory(upscaledDir)

	// Upscale frames
	err = upscaler.UpscaleFrames(inputDir, upscaledDir, options)
	if err != nil {
		return "", err
	}

	return upscaledDir, nil
}

// CombineFramesToVideo combines upscaled frames into a video
func CombineFramesToVideo(upscaledDir, videoPath string) (string, error) {
	// Get video info
	info, err := ffmpeg.GetVideoInfo(videoPath)
	if err != nil {
		return "", err
	}

	// Create output video path
	baseName := filepath.Base(videoPath)
	nameWithoutExt := strings.TrimSuffix(baseName, filepath.Ext(baseName))
	// Always use .mov extension for ProRes codec compatibility
	outputVideoPath := filepath.Join(filepath.Dir(videoPath), nameWithoutExt+"_upscaled.mov")

	// Combine frames into video
	err = ffmpeg.CombineFramesToVideo(upscaledDir, outputVideoPath, info)
	if err != nil {
		return "", err
	}

	return outputVideoPath, nil
}

// CleanupTempFiles removes temporary directories
func CleanupTempFiles(outputDir, upscaledDir string) error {
	// Wait a moment to ensure files are not in use
	time.Sleep(500 * time.Millisecond)

	// Remove the output directory with original frames
	err1 := os.RemoveAll(outputDir)
	if err1 == nil {
		cleanup.RemoveDirectory(outputDir)
	}

	// Remove the upscaled directory with upscaled frames
	err2 := os.RemoveAll(upscaledDir)
	if err2 == nil {
		cleanup.RemoveDirectory(upscaledDir)
	}

	// If either removal failed, return an error
	if err1 != nil {
		return err1
	}
	if err2 != nil {
		return err2
	}

	return nil
}

// CheckDependencies checks if all required dependencies are installed
func CheckDependencies() error {
	// Check if ffmpeg is installed
	if !ffmpeg.IsFFmpegInstalled() {
		return fmt.Errorf("ffmpeg is not installed or not in PATH")
	}

	// Check if ffprobe is installed
	if !ffmpeg.IsFFprobeInstalled() {
		return fmt.Errorf("ffprobe is not installed or not in PATH")
	}

	// Check if realesrgan is installed
	if !upscaler.IsRealesrganInstalled() {
		return fmt.Errorf("realesrgan-ncnn-vulkan is not installed or not found")
	}

	return nil
}
