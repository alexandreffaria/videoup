package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"videoup/internal/cleanup"
	"videoup/internal/ffmpeg"
	"videoup/internal/filepicker"
	"videoup/internal/ui"
	"videoup/internal/upscaler"

	tea "github.com/charmbracelet/bubbletea"
)

// UIModel represents the application UI state
type UIModel struct {
	filepicker      filepicker.Model
	state           string // "picking", "processing", "upscaling", "combining", "done", "error", "cleaning"
	videoPath       string
	outputDir       string
	upscaledDir     string
	outputVideoPath string
	upscalerOptions upscaler.UpscalerOptions
	err             error
	cleanupComplete bool
}

// NewUIModel creates a new UI model with default options
func NewUIModel() UIModel {
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

	return UIModel{
		filepicker:      filepicker.New(),
		state:           "picking",
		upscalerOptions: options,
		cleanupComplete: false,
	}
}

// Init initializes the UI model
func (m UIModel) Init() tea.Cmd {
	return m.filepicker.Init()
}

// Update handles UI events and state transitions
func (m UIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle global key events first
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		// Handle quit commands (ctrl+c, q, esc)
		if keyMsg.String() == "ctrl+c" || keyMsg.String() == "q" || keyMsg.String() == "esc" {
			// Always clean up before quitting, regardless of state
			fmt.Println(ui.FormatInfo("\nCleaning up before exit..."))
			cleanup.CleanupAll()
			return m, tea.Quit
		}
	}

	switch m.state {
	case "picking":
		return m.handlePickingState(msg)
	case "processing":
		return m.handleProcessingState(msg)
	case "upscaling":
		return m.handleUpscalingState(msg)
	case "combining":
		return m.handleCombiningState(msg)
	case "cleaning":
		return m.handleCleaningState(msg)
	case "done", "error":
		return m.handleFinalState(msg)
	}

	return m, nil
}

// View renders the UI based on the current state
func (m UIModel) View() string {
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

	case "cleaning":
		return ui.FormatTitle("VideoUp - Cleaning Up") + "\n\n" +
			ui.FormatInfo("Cleaning up temporary files...") + "\n" +
			ui.FormatInfo("Removing extracted frames and upscaled frames.") + "\n\n" +
			"Press Ctrl+C to cancel."

	case "done":
		return m.renderDoneView()

	case "error":
		return ui.FormatTitle("VideoUp - Error") + "\n\n" +
			ui.FormatError(fmt.Sprintf("Error: %v", m.err)) + "\n\n" +
			"Press Enter or q to exit."

	default:
		return "Unknown state"
	}
}

// Helper methods for Update

func (m UIModel) handlePickingState(msg tea.Msg) (tea.Model, tea.Cmd) {
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
			return m, processVideoCmd(m.videoPath)
		} else {
			// Not a video file, show error
			m.err = fmt.Errorf("selected file is not a video: %s", m.filepicker.Selected)
			m.state = "error"
			return m, nil
		}
	}

	// Check if quitting
	if m.filepicker.Quitting {
		// Clean up any directories that might have been created
		cleanup.CleanupAll()
		return m, tea.Quit
	}

	return m, cmd
}

func (m UIModel) handleProcessingState(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		return m, upscaleFramesCmd(m.outputDir, m.upscalerOptions)
	}
	return m, nil
}

func (m UIModel) handleUpscalingState(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		return m, combineFramesCmd(m.upscaledDir, m.videoPath)
	}
	return m, nil
}

func (m UIModel) handleCombiningState(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle combining state
	switch msg := msg.(type) {
	case errMsg:
		m.err = msg.err
		m.state = "error"
		return m, nil
	case combineResultMsg:
		m.outputVideoPath = msg.outputVideoPath
		m.state = "cleaning"
		return m, cleanupFilesCmd(m.outputDir, m.upscaledDir)
	}
	return m, nil
}

func (m UIModel) handleCleaningState(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle cleanup state
	switch msg := msg.(type) {
	case errMsg:
		// If cleanup fails, just log the error but still proceed to done state
		fmt.Printf("Warning: Failed to clean up temporary files: %v\n", msg.err)
		m.state = "done"
		return m, nil
	case cleanupResultMsg:
		m.cleanupComplete = true
		m.state = "done"
		return m, nil
	}
	return m, nil
}

func (m UIModel) handleFinalState(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle done or error state
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "enter" {
			// Always clean up before quitting
			if !m.cleanupComplete {
				cleanup.CleanupAll()
			}
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m UIModel) renderDoneView() string {
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
		ui.FormatInfo(fmt.Sprintf("Upscaled video: %s", m.outputVideoPath)) + "\n"

	// Only show temp directories if cleanup failed
	if !m.cleanupComplete {
		result += ui.FormatInfo(fmt.Sprintf("Original frames: %s", m.outputDir)) + "\n" +
			ui.FormatInfo(fmt.Sprintf("Upscaled frames: %s", m.upscaledDir)) + "\n"
	} else {
		result += ui.FormatSuccess("Temporary files have been cleaned up.") + "\n"
	}

	result += "\n"

	// Add video info if available
	if infoText != "" {
		result += ui.FormatInfo(infoText) + "\n\n"
	}

	result += "Press Enter or q to exit."
	return result
}
