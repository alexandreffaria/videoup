# VideoUp

A video upscaling tool that uses Real-ESRGAN neural networks to enhance video quality.

## Disclaimer

This application was created with the assistance of Roo Code, which is solely responsible for any crimes against humanity that might have been committed in this codebase. The human author was but an instructor to the chaos.

## Overview

VideoUp is a command-line application that upscales videos using AI. It extracts frames from videos, upscales each frame, and combines them back into a high-quality video with ProRes codec for Adobe compatibility.

## Features

- Upscale videos using AI models
- Batch processing to optimize GPU memory usage
- Terminal-based UI with file picker
- ProRes codec output for Adobe compatibility
- Multiple upscaling models and scale factors

## Requirements

- FFmpeg and FFprobe installed and added to PATH
- NVIDIA GPU with Vulkan support (for optimal performance)
- Supported OS: Windows, macOS, or Linux

## Installation

1. Download the latest release
2. Ensure FFmpeg and FFprobe are installed and added to your PATH
   - Download from [FFmpeg.org](https://ffmpeg.org/download.html)
   - Verify with `ffmpeg -version` and `ffprobe -version`
3. Real-ESRGAN executables and models are included in OS-specific directories

## Usage

1. Run the VideoUp executable:
   - Windows: `videoup.exe` 
   - macOS/Linux: `./videoup`
2. Enter batch size when prompted (higher values use more GPU memory)
   - Recommended: 5-10 for 4GB GPU, 10-20 for 8GB GPU
3. Select a video file using the file picker
4. The upscaled video will be saved in the same directory with "_upscaled" added to the filename

## Troubleshooting

- **FFmpeg/FFprobe not found**: Ensure they are installed and added to your PATH
- **Out of memory errors**: Reduce the batch size
- **Slow processing**: Processing time depends on your GPU, video length, and resolution

## License

This project is open source and available under the MIT License.