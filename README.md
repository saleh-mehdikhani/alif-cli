# Alif CLI

**Alif CLI** is a powerful command-line interface designed to streamline the development workflow for Alif Semiconductor devices (e.g., AK-E7-AIML). It abstracts the complexity of underlying tools—such as CMSIS Toolbox and Alif Security Toolkit—providing a unified interface for building, signing, and flashing applications.

## Installation

### Quick Install (macOS/Linux)
```bash
./install.sh
```
See the `scripts/` directory for advanced packaging options.

## Commands

### `alif build`
**Builds and packages your application.**

This command compiles the project source code using the CMSIS build system (`cbuild`). It resolves the build context (Target + Build Type) and prepares the binary artifacts. The CLI also supports cleaning the project before building.

**Usage:**
```bash
alif build -p <project_name> [flags]
```
- `-p, --project`: Specify the project name or build context (e.g., `blinky` or `blinky.debug+E7-HE`).
- `--clean`: Clean artifacts before building.

**About Build Contexts:**
The build context name follows the format `<project>.<build-type>+<target>` (e.g., `blinky.debug+E7-HE`). These are automatically read from your solution's `*.csolution.yml` file.

You can provide a partial name (e.g., `-p blinky`) to filter:
- If a single match is found, it is automatically selected.
- If multiple matches are found, the CLI will list all possible contexts for you to choose from interactively.

---

### `alif flash`
**Programs the firmware to the device.**

This command handles the end-to-end flashing process. It:
1.  Verifies the build artifacts.
2.  **Automatically creates the bootable image (TOC)** if missing or outdated.
3.  Detects the connected Alif devkit via Serial/USB.
4.  Programs the signed image to the device's MRAM.

**Usage:**
```bash
alif flash -p <project_name> [flags]
```
- `-p, --project`: Specify the project to flash.
- `--no-erase`: Skip the erase step (optional).

## Example Workflow

The following visual guide demonstrates the workflow for building and flashing the **Blinky** project (from [Alif Samples](https://github.com/saleh-mehdikhani/alif_samples)) to an **AK-E7-AIML (HW: D3)** devkit.

### 1. Build Completed
Run `alif build -p blinky` to compile the project. The CLI resolves the context (e.g., `blinky.debug+E7-HE`) and generates the binary.

![Build Completed](.img/build.png)

### 2. Flashing Progress
Run `alif flash -p blinky`. The tool checks for the bootable image (regenerating it if needed), connects to the device, and begins the flash operation.

![Flash Progress](.img/flash_progress.png)

### 3. Flash Successful
Upon completion, the CLI confirms that the firmware has been successfully erased and programmed to the device.

![Flash Done](.img/flash.png)
