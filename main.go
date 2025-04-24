package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"
)

func main() {
	// Set up signal handling for graceful shutdown
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)

	// Get the original working directory (where the command was run from)
	originalDir, err := os.Getwd()
	if err != nil {
		fmt.Println("Error getting current directory:", err)
		os.Exit(1)
	}

	// Get the path to the executable
	exePath, err := os.Executable()
	if err != nil {
		fmt.Println("Error getting executable path:", err)
		os.Exit(1)
	}

	// Get the directory containing the executable
	exeDir := filepath.Dir(exePath)

	// Change to the directory containing the executable
	err = os.Chdir(exeDir)
	if err != nil {
		fmt.Println("Error changing to executable directory:", err)
		os.Exit(1)
	}

	// Run the videoup command with the original directory as an environment variable
	var cmd *exec.Cmd

	// Create the command based on the operating system
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", filepath.Join("cmd", "videoup", "videoup.exe"))
	case "darwin", "linux":
		execPath := filepath.Join("cmd", "videoup", "videoup")
		// Make sure the file is executable
		os.Chmod(execPath, 0755)
		cmd = exec.Command(execPath)
	default:
		fmt.Println("Unsupported operating system:", runtime.GOOS)
		os.Exit(1)
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	// Set the original directory as an environment variable
	cmd.Env = append(os.Environ(), "VIDEOUP_ORIGINAL_DIR="+originalDir)

	// Handle termination signals
	go func() {
		<-signalCh
		fmt.Println("Received termination signal. Forwarding to child process...")
		// Forward the signal to the child process
		// The child process will handle cleanup
		if cmd.Process != nil {
			cmd.Process.Signal(os.Interrupt)
			// Give the child process a moment to clean up
			time.Sleep(500 * time.Millisecond)
		}
		// Exit after forwarding the signal
		os.Exit(1)
	}()

	err = cmd.Run()
	if err != nil {
		// Check if it's just a signal-related exit
		if _, ok := err.(*exec.ExitError); ok {
			// The program exited with a non-zero exit code, which is expected for signal termination
			fmt.Println("VideoUp was terminated.")
			os.Exit(1)
		}
		fmt.Println("Error running videoup:", err)
		os.Exit(1)
	}
}
