package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"videoup/internal/app"
	"videoup/internal/cleanup"
	"videoup/internal/ui"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	// Set up context with cancellation for cleanup
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Always clean up on exit
	defer cleanup.CleanupAll()

	// Set up signal handling for graceful shutdown
	setupSignalHandling(ctx, cancel)

	// Defer cleanup in case of panic
	defer handlePanic()

	// Check dependencies
	if err := checkDependencies(); err != nil {
		os.Exit(1)
	}

	// Run the application
	runApplication()
}

// setupSignalHandling sets up handlers for termination signals
func setupSignalHandling(ctx context.Context, cancel context.CancelFunc) {
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		select {
		case <-signalCh:
			fmt.Println(ui.FormatInfo("\nReceived termination signal. Exiting..."))
			// No need to explicitly call cleanup.CleanupAll() here
			// as it will be called by the defer at the top of main()
			cancel()
			os.Exit(1)
		case <-ctx.Done():
			return
		}
	}()
}

// handlePanic recovers from panics and ensures cleanup
func handlePanic() {
	if r := recover(); r != nil {
		fmt.Println(ui.FormatError(fmt.Sprintf("Program panicked: %v", r)))
		// No need to explicitly call cleanup.CleanupAll() here
		// as it will be called by the defer at the top of main()
	}
}

// checkDependencies verifies all required tools are installed
func checkDependencies() error {
	err := app.CheckDependencies()
	if err != nil {
		// Get OS-specific directory name for realesrgan
		var dirName string
		switch runtime.GOOS {
		case "windows":
			dirName = "realesrgan_win"
		case "darwin": // macOS
			dirName = "realesrgan_mac"
		case "linux":
			dirName = "realesrgan_linux"
		default:
			dirName = "realesrgan directory"
		}

		// Get OS-specific executable name
		exeName := "realesrgan-ncnn-vulkan"
		if runtime.GOOS == "windows" {
			exeName += ".exe"
		}

		fmt.Println(ui.FormatError(fmt.Sprintf("Error: %v", err)))

		if err.Error() == "ffmpeg is not installed or not in PATH" {
			fmt.Println(ui.FormatInfo("Please install ffmpeg and make sure it's in your PATH"))
		} else if err.Error() == "ffprobe is not installed or not in PATH" {
			fmt.Println(ui.FormatInfo("Please install ffprobe and make sure it's in your PATH"))
		} else {
			fmt.Println(ui.FormatInfo(fmt.Sprintf("Please make sure %s is in the %s directory", exeName, dirName)))
		}

		return err
	}
	return nil
}

// runApplication starts the Bubble Tea application
func runApplication() {
	// Create and run the program
	p := tea.NewProgram(app.NewUIModel())

	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running program: %v\n", err)
		os.Exit(1)
	}

	// Note: We don't need to explicitly call cleanup.CleanupAll() here
	// because we have a defer cleanup.CleanupAll() at the top of main()
}
