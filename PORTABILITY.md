# Alif CLI - Cross-Platform Portability

This document summarizes the changes made to ensure the `alif-cli` tool is fully portable across different operating systems and users.

## Changes Made

### 1. **setup.go** - Dynamic Path Detection
**Previous Issues:**
- Hardcoded paths like `/Users/saleh/Projects/alif/...`
- macOS-specific locations only
- Comments with user-specific examples

**Improvements:**
- ✅ Platform-aware search paths using `runtime.GOOS`
- ✅ Searches 10+ common directories per platform:
  - **macOS**: `~/Projects`, `~/Developer`, `/Applications`, `/opt/homebrew`, `/usr/local`
  - **Linux**: `~/projects`, `~/workspace`, `~/dev`, `/opt`, `/usr/local`, `/usr/share`
  - **Windows**: `C:\Program Files`, `C:\tools`, `C:\dev`
- ✅ Deep search (2 levels) in subdirectories
- ✅ Smart pattern matching for ARM toolchains
- ✅ Platform-specific CMSIS pack locations
- ✅ Windows executable detection (`.exe` extensions)

### 2. **builder.go** - Cross-Platform PATH Handling
**Previous Issues:**
- Hardcoded Unix PATH separator (`:`)
- Hardcoded `/opt/homebrew/bin` in PATH

**Improvements:**
- ✅ Runtime-detected PATH separator:
  - Unix/Linux/macOS: `:`
  - Windows: `;`
- ✅ Removed hardcoded Homebrew path
- ✅ Dynamic PATH construction from config

### 3. **signer.go** - Portable Executable Invocation
**Previous Issues:**
- Used `./app-gen-toc` which doesn't work on Windows

**Improvements:**
- ✅ Uses `filepath.Join()` to construct full executable path
- ✅ Works on all platforms

## Platform Support Matrix

| Feature | macOS | Linux | Windows |
|---------|-------|-------|---------|
| Auto-detect Alif Toolkit | ✅ | ✅ | ✅ |
| Auto-detect CMSIS Toolbox | ✅ | ✅ | ✅ |
| Auto-detect GCC Toolchain | ✅ | ✅ | ✅ |
| Auto-detect CMSIS Packs | ✅ | ✅ | ✅ |
| Build projects | ✅ | ✅ | ✅* |
| Sign artifacts | ✅ | ✅ | ✅* |
| Flash to device | ✅ | ✅ | ✅* |

*Note: Windows support assumes the Alif Security Toolkit has Windows binaries available.

## Distribution

The tool can now be distributed as:
1. **Single binary** per platform (no external dependencies except Go runtime)
2. **Source code** that compiles on any platform
3. **No hardcoded paths** - works for any user in any directory structure

## Usage

### First-Time Setup
```bash
alif setup
```
The tool will auto-detect all paths. If auto-detection fails, it will prompt for manual entry.

### Build & Flash
```bash
alif build -b <target>
alif flash
```

## Testing Cross-Platform Compatibility

To verify the code works on a new platform:
1. Run `go build -o alif`
2. Run `./alif setup`
3. Verify it detects tools in your environment
4. Try `alif build -b <target>` in a project directory
5. Try `alif flash` on a connected device

## Files Modified

1. `cmd/setup.go` - Platform-aware path detection
2. `internal/builder/builder.go` - Cross-platform PATH handling
3. `internal/signer/signer.go` - Portable executable invocation
4. `.gitignore` - Added standard Go project exclusions

## No Remaining Hardcoded Paths

All user-specific and platform-specific paths have been removed or made dynamic:
- ❌ No `/Users/saleh/...` paths
- ❌ No `/opt/homebrew` assumptions
- ❌ No Unix-only path separators
- ❌ No `./` executable prefixes
- ✅ All paths dynamically detected or user-configured
- ✅ All file operations use `filepath.Join()`
- ✅ All platform differences handled via `runtime.GOOS`
