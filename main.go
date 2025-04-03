package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func main() {
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

	// Run the videoup command
	cmd := exec.Command("cmd", "/c", filepath.Join("cmd", "videoup", "videoup.exe"))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	err = cmd.Run()
	if err != nil {
		fmt.Println("Error running videoup:", err)
		os.Exit(1)
	}
}
