package app

import (
	"videoup/internal/upscaler"

	tea "github.com/charmbracelet/bubbletea"
)

// Message types for Bubble Tea
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

type cleanupResultMsg struct {
	success bool
}

// Command functions for Bubble Tea

// processVideoCmd creates a command to process a video
func processVideoCmd(videoPath string) tea.Cmd {
	return func() tea.Msg {
		outputDir, err := ProcessVideo(videoPath)
		if err != nil {
			return errMsg{err}
		}
		return processResultMsg{outputDir: outputDir}
	}
}

// upscaleFramesCmd creates a command to upscale frames
func upscaleFramesCmd(inputDir string, options upscaler.UpscalerOptions) tea.Cmd {
	return func() tea.Msg {
		upscaledDir, err := UpscaleFrames(inputDir, options)
		if err != nil {
			return errMsg{err}
		}
		return upscaleResultMsg{upscaledDir: upscaledDir}
	}
}

// combineFramesCmd creates a command to combine frames into a video
func combineFramesCmd(upscaledDir, videoPath string) tea.Cmd {
	return func() tea.Msg {
		outputVideoPath, err := CombineFramesToVideo(upscaledDir, videoPath)
		if err != nil {
			return errMsg{err}
		}
		return combineResultMsg{outputVideoPath: outputVideoPath}
	}
}

// cleanupFilesCmd creates a command to clean up temporary files
func cleanupFilesCmd(outputDir, upscaledDir string) tea.Cmd {
	return func() tea.Msg {
		err := CleanupTempFiles(outputDir, upscaledDir)
		if err != nil {
			return errMsg{err}
		}
		return cleanupResultMsg{success: true}
	}
}
