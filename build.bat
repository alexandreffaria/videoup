@echo off
echo Building VideoUp for current platform...

:: Create output directory
if not exist "build" mkdir build

:: Detect architecture
reg Query "HKLM\Hardware\Description\System\CentralProcessor\0" | find /i "x86" > NUL && set ARCH=386 || set ARCH=amd64

:: Build for Windows with detected architecture
echo Building for Windows (%ARCH%)...
set GOOS=windows
set GOARCH=%ARCH%

if "%ARCH%"=="386" (
    go build -o build/videoup_windows_386.exe main.go
) else (
    go build -o build/videoup_windows_amd64.exe main.go
)

if %ERRORLEVEL% neq 0 goto :error

echo Build completed successfully!
goto :end

:error
echo Build failed with error code %ERRORLEVEL%
exit /b %ERRORLEVEL%

:end
echo Build process completed.

:: To build for all platforms, use:
:: @echo off
:: echo Building for all platforms...
:: echo.
:: echo Windows (64-bit)
:: set GOOS=windows
:: set GOARCH=amd64
:: go build -o build/videoup_windows_amd64.exe main.go
:: echo Windows (32-bit)
:: set GOOS=windows
:: set GOARCH=386
:: go build -o build/videoup_windows_386.exe main.go
:: echo macOS (64-bit)
:: set GOOS=darwin
:: set GOARCH=amd64
:: go build -o build/videoup_macos_amd64 main.go
:: echo macOS (ARM64)
:: set GOOS=darwin
:: set GOARCH=arm64
:: go build -o build/videoup_macos_arm64 main.go
:: echo Linux (64-bit)
:: set GOOS=linux
:: set GOARCH=amd64
:: go build -o build/videoup_linux_amd64 main.go
:: echo Linux (32-bit)
:: set GOOS=linux
:: set GOARCH=386
:: go build -o build/videoup_linux_386 main.go