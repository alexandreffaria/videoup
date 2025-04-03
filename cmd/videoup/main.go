package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"videoup/internal/ffmpeg"
	"videoup/internal/filepicker"
	"videoup/internal/ui"
	"videoup/internal/upscaler"

	tea "github.com/charmbracelet/bubbletea"
)

type model struct {
	filepicker      filepicker.Model
	state           string // "picking", "processing", "upscaling", "combining", "done", "error"
	videoPath       string
	outputDir       string
	upscaledDir     string
	outputVideoPath string
	upscalerOptions upscaler.UpscalerOptions
	err             error
}

func initialModel() model {
	// Get default options
	options := upscaler.DefaultOptions()

	// Prompt for batch size
	fmt.Println(ui.FormatTitle("VideoUp - Configuration"))
	fmt.Println(ui.FormatInfo("Enter batch size for upscaling (higher values use more GPU memory):"))
	fmt.Println(ui.FormatInfo("Recommended values: 5-10 for 4GB GPU, 10-20 for 8GB GPU, 20-30 for 16GB+ GPU"))
	fmt.Printf("Batch size [%d]: ", options.BatchSize)

	var input string
	fmt.Scanln(&input)

	// Parse batch size
	if input != "" {
		var batchSize int
		_, err := fmt.Sscanf(input, "%d", &batchSize)
		if err == nil && batchSize > 0 {
			options.BatchSize = batchSize
		}
	}

	fmt.Println(ui.FormatInfo(fmt.Sprintf("Using batch size: %d", options.BatchSize)))
	fmt.Println()

	return model{
		filepicker:      filepicker.New(),
		state:           "picking",
		upscalerOptions: options,
	}
}

func (m model) Init() tea.Cmd {
	return m.filepicker.Init()
}

// Custom command to process video
func processVideo(videoPath string) tea.Cmd {
	return func() tea.Msg {
		// Create temp directory
		tempDir, err := ffmpeg.CreateTempDir(videoPath)
		if err != nil {
			return errMsg{err}
		}

		// Extract frames
		err = ffmpeg.ExtractFrames(videoPath, tempDir)
		if err != nil {
			return errMsg{err}
		}

		return processResultMsg{outputDir: tempDir}
	}
}

// Custom command to combine frames into a video
func combineFramesToVideo(upscaledDir, videoPath string, outputDir string) tea.Cmd {
	return func() tea.Msg {
		// Get video info
		info, err := ffmpeg.GetVideoInfo(videoPath)
		if err != nil {
			return errMsg{err}
		}

		// Create output video path
		baseName := filepath.Base(videoPath)
		ext := filepath.Ext(baseName)
		nameWithoutExt := strings.TrimSuffix(baseName, ext)
		outputVideoPath := filepath.Join(filepath.Dir(videoPath), nameWithoutExt+"_upscaled"+ext)

		// Combine frames into video
		err = ffmpeg.CombineFramesToVideo(upscaledDir, outputVideoPath, info)
		if err != nil {
			return errMsg{err}
		}

		return combineResultMsg{outputVideoPath: outputVideoPath}
	}
}

// Custom command to upscale frames
func upscaleFrames(inputDir string, options upscaler.UpscalerOptions) tea.Cmd {
	return func() tea.Msg {
		// Create upscaled directory
		upscaledDir, err := upscaler.CreateUpscaledDir(inputDir)
		if err != nil {
			return errMsg{err}
		}

		// Upscale frames
		err = upscaler.UpscaleFrames(inputDir, upscaledDir, options)
		if err != nil {
			return errMsg{err}
		}

		return upscaleResultMsg{upscaledDir: upscaledDir}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.state {
	case "picking":
		// Handle file picking state
		var cmd tea.Cmd
		newFilepicker, cmd := m.filepicker.Update(msg)
		m.filepicker = newFilepicker

		// Check if a file was selected
		if m.filepicker.Selected != "" {
			// Verify it's a video file
			if filepicker.VideoFileFilter(m.filepicker.Selected) {
				m.videoPath = m.filepicker.Selected
				m.state = "processing"

				// Start processing the video
				return m, processVideo(m.videoPath)
			} else {
				// Not a video file, show error
				m.err = fmt.Errorf("selected file is not a video: %s", m.filepicker.Selected)
				m.state = "error"
				return m, nil
			}
		}

		// Check if quitting
		if m.filepicker.Quitting {
			return m, tea.Quit
		}

		return m, cmd

	case "processing":
		// Handle processing state
		switch msg := msg.(type) {
		case errMsg:
			m.err = msg.err
			m.state = "error"
			return m, nil
		case processResultMsg:
			m.outputDir = msg.outputDir
			m.state = "upscaling"

			// Start upscaling the frames
			return m, upscaleFrames(m.outputDir, m.upscalerOptions)
		case tea.KeyMsg:
			if msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
		}

	case "upscaling":
		// Handle upscaling state
		switch msg := msg.(type) {
		case errMsg:
			m.err = msg.err
			m.state = "error"
			return m, nil
		case upscaleResultMsg:
			m.upscaledDir = msg.upscaledDir
			m.state = "combining"

			// Start combining frames into video
			return m, combineFramesToVideo(m.upscaledDir, m.videoPath, m.outputDir)
		case tea.KeyMsg:
			if msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
		}

	case "combining":
		// Handle combining state
		switch msg := msg.(type) {
		case errMsg:
			m.err = msg.err
			m.state = "error"
			return m, nil
		case combineResultMsg:
			m.outputVideoPath = msg.outputVideoPath
			m.state = "done"
			return m, nil
		case tea.KeyMsg:
			if msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
		}

	case "done", "error":
		// Handle done or error state
		switch msg := msg.(type) {
		case tea.KeyMsg:
			if msg.String() == "q" || msg.String() == "ctrl+c" || msg.String() == "enter" {
				return m, tea.Quit
			}
		}
	}

	return m, nil
}

func (m model) View() string {
	switch m.state {
	case "picking":
		return ui.FormatTitle("VideoUp - Select a Video File") + "\n\n" +
			m.filepicker.View()

	case "processing":
		return ui.FormatTitle("VideoUp - Processing Video") + "\n\n" +
			ui.FormatInfo("Extracting frames from video...") + "\n" +
			ui.FormatInfo("This may take a while depending on the video size.") + "\n\n" +
			"Press Ctrl+C to cancel."

	case "upscaling":
		return ui.FormatTitle("VideoUp - Upscaling Frames") + "\n\n" +
			ui.FormatInfo("Upscaling frames using Real-ESRGAN...") + "\n" +
			ui.FormatInfo(fmt.Sprintf("Using model: %s with scale: %d", m.upscalerOptions.Model, m.upscalerOptions.Scale)) + "\n" +
			ui.FormatInfo("This may take a while depending on the number of frames and your GPU.") + "\n\n" +
			"Press Ctrl+C to cancel."

	case "combining":
		return ui.FormatTitle("VideoUp - Creating Video") + "\n\n" +
			ui.FormatInfo("Combining upscaled frames into a video file...") + "\n" +
			ui.FormatInfo("Using ProRes codec for Adobe compatibility.") + "\n" +
			ui.FormatInfo("This may take a while depending on the number of frames.") + "\n\n" +
			"Press Ctrl+C to cancel."

	case "done":
		// Try to read the video info file
		infoText := ""
		infoPath := filepath.Join(m.outputDir, "video_info.json")
		if infoData, err := os.ReadFile(infoPath); err == nil {
			var info ffmpeg.VideoInfo
			if err := json.Unmarshal(infoData, &info); err == nil {
				infoText = fmt.Sprintf(
					"Video Information:\n"+
						"  Resolution: %dx%d\n"+
						"  Frame Rate: %.2f fps\n"+
						"  Duration: %.2f seconds\n"+
						"  Total Frames: %d\n"+
						"  Format: %s\n"+
						"  Codec: %s\n",
					info.Width, info.Height,
					info.FrameRate,
					info.Duration,
					info.TotalFrames,
					info.FormatName,
					info.CodecName,
				)
			}
		}

		result := ui.FormatTitle("VideoUp - Processing Complete") + "\n\n" +
			ui.FormatSuccess("Successfully extracted, upscaled, and combined frames!") + "\n\n" +
			ui.FormatInfo(fmt.Sprintf("Original video: %s", m.videoPath)) + "\n" +
			ui.FormatInfo(fmt.Sprintf("Upscaled video: %s", m.outputVideoPath)) + "\n" +
			ui.FormatInfo(fmt.Sprintf("Original frames: %s", m.outputDir)) + "\n" +
			ui.FormatInfo(fmt.Sprintf("Upscaled frames: %s", m.upscaledDir)) + "\n\n"

		// Add video info if available
		if infoText != "" {
			result += ui.FormatInfo(infoText) + "\n\n"
		}

		result += "Press Enter or q to exit."
		return result

	case "error":
		return ui.FormatTitle("VideoUp - Error") + "\n\n" +
			ui.FormatError(fmt.Sprintf("Error: %v", m.err)) + "\n\n" +
			"Press Enter or q to exit."

	default:
		return "Unknown state"
	}
}

// Custom message types
type errMsg struct {
	err error
}

func (e errMsg) Error() string {
	return e.err.Error()
}

type processResultMsg struct {
	outputDir string
}

type upscaleResultMsg struct {
	upscaledDir string
}

type combineResultMsg struct {
	outputVideoPath string
}

func main() {
	// Check if ffmpeg is installed
	if !ffmpeg.IsFFmpegInstalled() {
		fmt.Println(ui.FormatError("Error: ffmpeg is not installed or not in PATH"))
		fmt.Println(ui.FormatInfo("Please install ffmpeg and make sure it's in your PATH"))
		os.Exit(1)
	}

	// Check if ffprobe is installed
	if !ffmpeg.IsFFprobeInstalled() {
		fmt.Println(ui.FormatError("Error: ffprobe is not installed or not in PATH"))
		fmt.Println(ui.FormatInfo("Please install ffprobe and make sure it's in your PATH"))
		os.Exit(1)
	}

	// Check if realesrgan is installed
	if !upscaler.IsRealesrganInstalled() {
		fmt.Println(ui.FormatError("Error: realesrgan-ncnn-vulkan.exe is not found"))
		fmt.Println(ui.FormatInfo("Please make sure realesrgan-ncnn-vulkan.exe is in the realesrgan_win directory"))
		os.Exit(1)
	}

	p := tea.NewProgram(initialModel())

	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running program: %v\n", err)
		os.Exit(1)
	}
}
