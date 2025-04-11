# VideoUp

A powerful video upscaling tool that uses Real-ESRGAN neural networks to enhance video quality.

## Overview

VideoUp is a command-line application that upscales videos using the Real-ESRGAN neural network models. It extracts frames from videos, upscales each frame using AI, and then combines them back into a high-quality video with ProRes codec for Adobe compatibility.

## Features

- Upscale videos using state-of-the-art AI models
- Batch processing to optimize GPU memory usage
- Terminal-based UI with file picker
- ProRes codec output for Adobe compatibility
- Multiple upscaling models and scale factors
- Automatic cleanup of temporary files

## Requirements

- FFmpeg and FFprobe installed and added to PATH
- NVIDIA GPU with Vulkan support (for optimal performance)
- Sufficient disk space for temporary frame extraction
- Supported operating systems:
  - Windows
  - macOS
  - Linux

## Installation

1. Download the latest release or clone this repository
2. Ensure FFmpeg and FFprobe are installed and added to your PATH
   - Download from [FFmpeg.org](https://ffmpeg.org/download.html) or install via package manager
   - Verify installation by running `ffmpeg -version` and `ffprobe -version` in your terminal
3. The Real-ESRGAN executables and models are included in the following directories:
   - Windows: `realesrgan_win` directory
   - macOS: `realesrgan_mac` directory
   - Linux: `realesrgan_linux` directory

The application will automatically detect your operating system and use the appropriate Real-ESRGAN executable.

## Adding to PATH (Optional)

To run VideoUp from any directory, add the application directory to your PATH:

### Windows
1. Press Win + X and select "System"
2. Click on "Advanced system settings"
3. Click on "Environment Variables"
4. Under "System variables", find the "Path" variable and click "Edit"
5. Click "New" and add the full path to the VideoUp directory
6. Click "OK" to close all dialogs

### macOS
1. Open Terminal
2. Edit your shell profile file (e.g., `~/.zshrc` or `~/.bash_profile`)
3. Add the following line: `export PATH="$PATH:/path/to/videoup"`
4. Save the file and run `source ~/.zshrc` (or your profile file)

### Linux
1. Open Terminal
2. Edit your shell profile file (e.g., `~/.bashrc`)
3. Add the following line: `export PATH="$PATH:/path/to/videoup"`
4. Save the file and run `source ~/.bashrc`

## Usage

1. Run the VideoUp executable:
   - Windows: `videoup.exe` from the command line or by double-clicking
   - macOS/Linux: `./videoup` from the terminal
2. Enter the batch size when prompted (higher values use more GPU memory)
   - Recommended: 5-10 for 4GB GPU, 10-20 for 8GB GPU, 20-30 for 16GB+ GPU
3. Use the file picker to navigate to and select a video file
4. The application will process the video in these steps:
   - Extract frames from the video
   - Upscale each frame using Real-ESRGAN
   - Combine the upscaled frames into a new video
   - Clean up temporary files
5. The upscaled video will be saved in the same directory as the original with "_upscaled" added to the filename

## Available Models

- `realesrgan-x4plus`: General purpose model (4x upscaling)
- `realesrgan-x4plus-anime`: Optimized for anime/cartoon content (4x upscaling)
- `realesrgan-animevideov3`: Optimized for anime videos (2x, 3x, or 4x upscaling)
- `realesr-animevideov3`: Alternative model for anime videos

## Troubleshooting

- **FFmpeg/FFprobe not found**: Ensure they are installed and added to your PATH
- **Real-ESRGAN not found**: Make sure the appropriate Real-ESRGAN directory for your OS is in the same directory as the executable:
  - Windows: `realesrgan_win`
  - macOS: `realesrgan_mac`
  - Linux: `realesrgan_linux`
- **Out of memory errors**: Reduce the batch size when prompted at startup
- **Slow processing**: Video upscaling is computationally intensive. Processing time depends on your GPU, video length, and resolution

## Technical Details

VideoUp is built with Go and uses several key components:

- **FFmpeg**: For video frame extraction and recombination
- **Real-ESRGAN**: Neural network model for upscaling
- **Bubble Tea**: Terminal UI framework
- **LipGloss**: Terminal styling

The application follows this workflow:
1. Extract frames from the input video using FFmpeg
2. Process frames in batches using Real-ESRGAN
3. Combine upscaled frames into a new video with ProRes codec
4. Clean up temporary files

## Disclaimer

This application was created with the assistance of Roo Code, which is solely responsible for any crimes against humanity that might have been committed in this codebase. The human author was but an instructor to the chaos.

## License

This project is open source and available under the MIT License.