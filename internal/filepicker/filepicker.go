package filepicker

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/filepicker"
	tea "github.com/charmbracelet/bubbletea"
)

// VideoFileFilter filters for video file extensions
func VideoFileFilter(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	videoExts := []string{
		".mp4", ".avi", ".mkv", ".mov", ".wmv", ".flv", ".webm", ".m4v", ".mpg", ".mpeg", ".3gp",
	}

	for _, videoExt := range videoExts {
		if ext == videoExt {
			return true
		}
	}

	return false
}

// Model represents the file picker model
type Model struct {
	Picker   filepicker.Model
	Selected string
	Quitting bool
	Err      error
}

// New creates a new file picker model
func New() Model {
	fp := filepicker.New()

	// Get the current working directory to use as default
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}

	// Configure the file picker
	fp.CurrentDirectory = cwd
	fp.ShowHidden = false

	return Model{
		Picker:   fp,
		Selected: "",
		Quitting: false,
		Err:      nil,
	}
}

// Init initializes the model
func (m Model) Init() tea.Cmd {
	return m.Picker.Init()
}

// Update handles messages and updates the model
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.Quitting = true
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.Picker, cmd = m.Picker.Update(msg)

	// When a file is selected
	if didSelect, path := m.Picker.DidSelectFile(msg); didSelect {
		// Verify it's a video file
		if VideoFileFilter(path) {
			m.Selected = path
			return m, tea.Quit
		}
	}

	return m, cmd
}

// View renders the model
func (m Model) View() string {
	if m.Quitting {
		return "Quitting...\n"
	}

	if m.Selected != "" {
		return "Selected video: " + m.Selected + "\n"
	}

	return "Select a video file:\n\n" + m.Picker.View() + "\n\nPress q to quit.\n"
}
