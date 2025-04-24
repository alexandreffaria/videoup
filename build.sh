#!/bin/bash
echo "Building VideoUp for current platform..."

# Create output directory
mkdir -p build

# Detect OS
OS=$(uname -s)
ARCH=$(uname -m)

# Map architecture to Go architecture
if [ "$ARCH" = "x86_64" ]; then
    GOARCH="amd64"
elif [ "$ARCH" = "i386" ] || [ "$ARCH" = "i686" ]; then
    GOARCH="386"
elif [ "$ARCH" = "arm64" ] || [ "$ARCH" = "aarch64" ]; then
    GOARCH="arm64"
elif [[ "$ARCH" == arm* ]]; then
    GOARCH="arm"
else
    echo "Unsupported architecture: $ARCH"
    exit 1
fi

# Build for detected OS and architecture
if [ "$OS" = "Darwin" ]; then
    # macOS
    echo "Building for macOS ($GOARCH)..."
    GOOS=darwin GOARCH=$GOARCH go build -o build/videoup_macos_$GOARCH main.go
elif [ "$OS" = "Linux" ]; then
    # Linux
    echo "Building for Linux ($GOARCH)..."
    GOOS=linux GOARCH=$GOARCH go build -o build/videoup_linux_$GOARCH main.go
else
    echo "Unsupported OS: $OS"
    exit 1
fi

if [ $? -ne 0 ]; then
    echo "Build failed!"
    exit 1
fi

echo "Build completed successfully!"

# To build for all platforms, uncomment the following:
# echo "Building for all platforms..."
#
# # Windows (64-bit)
# echo "Building for Windows (64-bit)..."
# GOOS=windows GOARCH=amd64 go build -o build/videoup_windows_amd64.exe main.go
#
# # Windows (32-bit)
# echo "Building for Windows (32-bit)..."
# GOOS=windows GOARCH=386 go build -o build/videoup_windows_386.exe main.go
#
# # macOS (64-bit)
# echo "Building for macOS (64-bit)..."
# GOOS=darwin GOARCH=amd64 go build -o build/videoup_macos_amd64 main.go
#
# # macOS (ARM64 - for M1/M2 Macs)
# echo "Building for macOS (ARM64)..."
# GOOS=darwin GOARCH=arm64 go build -o build/videoup_macos_arm64 main.go
#
# # Linux (64-bit)
# echo "Building for Linux (64-bit)..."
# GOOS=linux GOARCH=amd64 go build -o build/videoup_linux_amd64 main.go
#
# # Linux (32-bit)
# echo "Building for Linux (32-bit)..."
# GOOS=linux GOARCH=386 go build -o build/videoup_linux_386 main.go