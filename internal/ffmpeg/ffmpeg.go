package ffmpeg

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// VideoInfo contains information about a video file
type VideoInfo struct {
	FrameRate     float64 `json:"frame_rate"`
	Width         int     `json:"width"`
	Height        int     `json:"height"`
	TotalFrames   int     `json:"total_frames"`
	Duration      float64 `json:"duration"`
	FormatName    string  `json:"format_name"`
	CodecName     string  `json:"codec_name"`
	FileName      string  `json:"file_name"`
	FilePath      string  `json:"file_path"`
	OutputDir     string  `json:"output_dir"`
	ExtractedTime string  `json:"extracted_time"`
}

// ExtractFrames extracts all frames from a video file to a PNG sequence
func ExtractFrames(videoPath string, outputDir string) error {
	// Create output directory if it doesn't exist
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Construct the output pattern
	outputPattern := filepath.Join(outputDir, "frame_%04d.png")

	// Prepare the ffmpeg command to extract all frames
	// -i: input file
	// -q:v 1: highest quality for images
	cmd := exec.Command(
		"ffmpeg",
		"-i", videoPath,
		"-q:v", "1",
		outputPattern,
	)

	// Capture stdout and stderr
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Run the command
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg command failed: %w", err)
	}

	// Get video info and save it to the output directory
	info, err := GetVideoInfo(videoPath)
	if err != nil {
		return fmt.Errorf("failed to get video info: %w", err)
	}

	// Set output directory in the info
	info.OutputDir = outputDir

	// Save video info to a JSON file in the output directory
	infoPath := filepath.Join(outputDir, "video_info.json")
	infoData, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal video info: %w", err)
	}

	if err := os.WriteFile(infoPath, infoData, 0644); err != nil {
		return fmt.Errorf("failed to write video info file: %w", err)
	}

	return nil
}

// GetVideoInfo retrieves information about a video file using ffprobe
func GetVideoInfo(videoPath string) (*VideoInfo, error) {
	// Run ffprobe to get video information
	cmd := exec.Command(
		"ffprobe",
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height,r_frame_rate,codec_name,nb_frames",
		"-show_entries", "format=duration,format_name",
		"-of", "default=noprint_wrappers=1",
		videoPath,
	)

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe command failed: %w", err)
	}

	// Parse the output
	info := &VideoInfo{
		FileName: filepath.Base(videoPath),
		FilePath: videoPath,
	}

	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "width":
			info.Width, _ = strconv.Atoi(value)
		case "height":
			info.Height, _ = strconv.Atoi(value)
		case "r_frame_rate":
			// Parse frame rate (usually in the format "num/den")
			if strings.Contains(value, "/") {
				frParts := strings.Split(value, "/")
				if len(frParts) == 2 {
					num, _ := strconv.ParseFloat(frParts[0], 64)
					den, _ := strconv.ParseFloat(frParts[1], 64)
					if den > 0 {
						info.FrameRate = num / den
					}
				}
			} else {
				info.FrameRate, _ = strconv.ParseFloat(value, 64)
			}
		case "codec_name":
			info.CodecName = value
		case "nb_frames":
			info.TotalFrames, _ = strconv.Atoi(value)
		case "duration":
			info.Duration, _ = strconv.ParseFloat(value, 64)
		case "format_name":
			info.FormatName = value
		}
	}

	// If nb_frames is not available, estimate from duration and frame rate
	if info.TotalFrames == 0 && info.Duration > 0 && info.FrameRate > 0 {
		info.TotalFrames = int(info.Duration * info.FrameRate)
	}

	// Add current time
	info.ExtractedTime = fmt.Sprintf("%s", time.Now().Format(time.RFC3339))

	return info, nil
}

// CreateTempDir creates a temporary directory for frames based on the video filename
func CreateTempDir(videoPath string) (string, error) {
	// Get the base filename without extension
	videoBase := filepath.Base(videoPath)
	videoName := strings.TrimSuffix(videoBase, filepath.Ext(videoBase))

	// Create a temp directory in the current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current working directory: %w", err)
	}

	// Create a directory named "temp_frames_{videoName}"
	tempDir := filepath.Join(cwd, fmt.Sprintf("temp_frames_%s", videoName))

	// Create the directory if it doesn't exist
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}

	return tempDir, nil
}

// IsFFmpegInstalled checks if ffmpeg is installed and available in the PATH
func IsFFmpegInstalled() bool {
	_, err := exec.LookPath("ffmpeg")
	return err == nil
}

// IsFFprobeInstalled checks if ffprobe is installed and available in the PATH
func IsFFprobeInstalled() bool {
	_, err := exec.LookPath("ffprobe")
	return err == nil
}

// CombineFramesToVideo combines PNG frames into a video file
func CombineFramesToVideo(framesDir, outputPath string, info *VideoInfo) error {
	// Create output directory if it doesn't exist
	outputDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Construct the input pattern
	inputPattern := filepath.Join(framesDir, "frame_%04d.png")

	// Prepare the ffmpeg command
	// -framerate: set the frame rate
	// -i: input file pattern
	// -c:v: video codec (prores_ks is compatible with Adobe products)
	// -profile:v: ProRes profile (3 is ProRes HQ, good balance of quality and size)
	// -pix_fmt: pixel format (yuv422p10le for ProRes)
	// -vendor: vendor string
	cmd := exec.Command(
		"ffmpeg",
		"-framerate", fmt.Sprintf("%.2f", info.FrameRate),
		"-i", inputPattern,
		"-c:v", "prores_ks",
		"-profile:v", "3",
		"-pix_fmt", "yuv422p10le",
		"-vendor", "ap10",
		outputPath,
	)

	// Capture stdout and stderr
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Run the command
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg command failed: %w", err)
	}

	return nil
}
