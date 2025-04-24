package cleanup

import (
	"fmt"
	"os"
	"sync"
	"videoup/internal/ui"
)

// Global variables to track created directories for cleanup
var (
	createdDirs      []string
	createdDirsMutex sync.Mutex
)

// RegisterDirectory adds a directory to the list of directories to clean up
func RegisterDirectory(dir string) {
	createdDirsMutex.Lock()
	defer createdDirsMutex.Unlock()
	createdDirs = append(createdDirs, dir)
}

// RemoveDirectory removes a directory from the registry
func RemoveDirectory(dir string) {
	createdDirsMutex.Lock()
	defer createdDirsMutex.Unlock()

	for i, registeredDir := range createdDirs {
		if registeredDir == dir {
			createdDirs = append(createdDirs[:i], createdDirs[i+1:]...)
			break
		}
	}
}

// CleanupAll removes all registered directories
func CleanupAll() {
	createdDirsMutex.Lock()
	defer createdDirsMutex.Unlock()

	fmt.Println(ui.FormatInfo("Cleaning up temporary directories..."))
	for _, dir := range createdDirs {
		// Check if directory exists before attempting to remove
		if _, err := os.Stat(dir); err == nil {
			fmt.Printf("Removing directory: %s\n", dir)
			err := os.RemoveAll(dir)
			if err != nil {
				fmt.Printf("Warning: Failed to remove directory %s: %v\n", dir, err)
			}
		}
	}
	// Clear the list after cleanup
	createdDirs = []string{}
}

// HasDirectories returns true if there are directories registered for cleanup
func HasDirectories() bool {
	createdDirsMutex.Lock()
	defer createdDirsMutex.Unlock()
	return len(createdDirs) > 0
}
