# Alif CLI

A command-line interface tool for building, signing, and flashing applications to Alif Semiconductor boards. Inspired by Zephyr's `west` tool, `alif` simplifies the development workflow for Alif devkits.

## Features

- ✅ **Build** - Compile projects using CMSIS Toolbox
- ✅ **Sign** - Automatically sign binaries with Alif Security Toolkit
- ✅ **Flash** - Flash signed firmware to connected devkit
- ✅ **Auto-detect** - Automatically find installed tools
- ✅ **Cross-platform** - Works on macOS, Linux, and Windows
- ✅ **In-place builds** - All artifacts stay in your project directory

## Quick Start

### Prerequisites

1. **Go** (version 1.20 or later)
   - macOS: `brew install go`
   - Linux: `sudo apt install golang` or download from [golang.org](https://golang.org/dl/)
   - Windows: Download installer from [golang.org](https://golang.org/dl/)

2. **Alif Development Tools** (installed separately):
   - Alif Security Toolkit (`app-write-mram`, `app-gen-toc`, etc.)
   - CMSIS Toolbox (`cbuild`)
   - ARM GNU Toolchain (`arm-none-eabi-gcc`)

### Building the Project

1. **Clone or navigate to the project**:
   ```bash
   cd /path/to/alif-cli
   ```

2. **Download Go dependencies**:
   ```bash
   go mod download
   ```

3. **Build the binary**:
   ```bash
   go build -o alif
   ```

4. **(Optional) Install globally**:
   
   **macOS/Linux:**
   ```bash
   sudo cp alif /usr/local/bin/alif
   ```
   Or for user-only installation:
   ```bash
   mkdir -p ~/.local/bin
   cp alif ~/.local/bin/alif
   # Add to PATH if needed
   echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.bashrc
   ```

   **Windows:**
   ```powershell
   copy alif.exe C:\Windows\System32\
   ```
   Or add to a directory in your PATH.

### First-Time Setup

Run the setup command to configure tool paths:

```bash
./alif setup
```

The tool will auto-detect installed Alif tools. If auto-detection fails, it will prompt you to enter paths manually.

Configuration is saved to `~/.alif/config.yaml`.

## Usage

### Building a Project

Navigate to your Alif project directory and run:

```bash
alif build -b <target>
```

**Examples:**
```bash
alif build -b E7-HE                    # Build for E7-HE target
alif build -b blinky.debug+E7-HE      # Build specific context
```

The tool will:
1. Compile using `cbuild`
2. Sign the binary with `app-gen-toc`
3. Save all artifacts in `out/<project>/<target>/<build-type>/`

### Flashing to Device

Connect your Alif devkit via USB and run:

```bash
alif flash
```

The tool will:
1. Auto-detect connected device
2. Stage files to Alif Security Toolkit
3. Flash firmware using `app-write-mram`

### Updating Configuration

Re-run setup at any time:

```bash
alif setup
```

## Project Structure

```
alif-cli/
├── main.go              # Entry point
├── cmd/                 # Commands
│   ├── root.go         # Root command
│   ├── setup.go        # Setup command
│   ├── build.go        # Build command
│   └── flash.go        # Flash command
├── internal/           # Internal packages
│   ├── builder/        # Build logic
│   ├── signer/         # Signing logic
│   ├── flasher/        # Flashing logic
│   ├── config/         # Configuration management
│   └── project/        # Project validation
├── go.mod              # Go module definition
└── go.sum              # Dependency checksums
```

## Development

### Running Tests

```bash
go test ./...
```

### Building for Specific Platforms

**Cross-compile for Linux:**
```bash
GOOS=linux GOARCH=amd64 go build -o alif-linux
```

**Cross-compile for Windows:**
```bash
GOOS=windows GOARCH=amd64 go build -o alif.exe
```

**Cross-compile for macOS (Apple Silicon):**
```bash
GOOS=darwin GOARCH=arm64 go build -o alif-macos-arm64
```

### Adding Dependencies

```bash
go get <package>
go mod tidy
```

## Troubleshooting

### "no serial ports found"
- Ensure your devkit is connected via USB
- On Linux, you may need to add your user to the `dialout` group:
  ```bash
  sudo usermod -a -G dialout $USER
  ```
  Then log out and back in.

### "cbuild: command not found"
- Run `alif setup` to configure CMSIS Toolbox path
- Ensure CMSIS Toolbox `bin` directory is specified correctly

### "Signing failed"
- Verify Alif Security Toolkit path is correct
- Check that signing config exists in `<project>/.alif/<core>_cfg.json`

### Build fails with "no solution file found"
- Ensure you're running `alif build` from a directory containing a `*.csolution.yml` file

## Configuration File

Config is stored at `~/.alif/config.yaml`:

```yaml
alif_tools_path: /path/to/app-release-exec-macos
cmsis_toolbox: /path/to/cmsis-toolbox/bin
gcc_toolchain: /path/to/arm-gnu-toolchain/bin
cmsis_pack_root: ~/.cache/arm/packs
signing_key_path: /path/to/app-release-exec-macos/cert
```

## License

[Add your license here]

## Contributing

Contributions are welcome! Please submit issues or pull requests.

## Acknowledgments

- Built for use with Alif Semiconductor devkits
- Inspired by Zephyr's `west` tool
