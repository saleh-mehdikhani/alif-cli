#!/bin/bash
set -e

APP_NAME="alif"
INSTALL_DIR="/usr/local/bin"

# Check OS
OS="$(uname -s)"
case "${OS}" in
    Linux*)     OS=linux;;
    Darwin*)    OS=darwin;;
    CYGWIN*)    OS=windows;;
    MINGW*)     OS=windows;;
    *)          OS="UNKNOWN:${OS}"
esac

# Function to check command existence
check_cmd() {
    command -v "$1" >/dev/null 2>&1
}

echo "Installing ${APP_NAME} for ${OS}..."

# 1. Determine Binary Source
BINARY=""
if [ -f "./${APP_NAME}" ]; then
    echo "Found pre-built binary: ./${APP_NAME}"
    BINARY="./${APP_NAME}"
elif [ -f "./${APP_NAME}.exe" ]; then
    echo "Found pre-built binary: ./${APP_NAME}.exe"
    BINARY="./${APP_NAME}.exe"
elif [ -f "go.mod" ]; then
    echo "Building from source..."
    if ! check_cmd go; then
        echo "Error: 'go' is not installed. Cannot build from source."
        exit 1
    fi
    go build -o ${APP_NAME}
    if [ $? -ne 0 ]; then
        echo "Build failed."
        exit 1
    fi
    BINARY="./${APP_NAME}"
else
    echo "Error: distinct binary not found and not a Go project root."
    exit 1
fi

# 2. Install
if [ "$OS" = "windows" ]; then
    echo "Windows installation not fully automated via bash script."
    echo "Please copy '${BINARY}' to your PATH manually."
    exit 0
fi

echo "Installing to ${INSTALL_DIR} (requires sudo)..."
if [ ! -d "${INSTALL_DIR}" ]; then
    echo "Directory ${INSTALL_DIR} does not exist. Creating..."
    sudo mkdir -p "${INSTALL_DIR}"
fi

sudo cp "${BINARY}" "${INSTALL_DIR}/${APP_NAME}"
sudo chmod +x "${INSTALL_DIR}/${APP_NAME}"

# 3. Install Toolkit & Configure
USER_HOME=$(eval echo ~${SUDO_USER})
ALIF_DIR="${USER_HOME}/.alif"
TOOLKIT_DEST="${ALIF_DIR}/toolkit"
CONFIG_FILE="${ALIF_DIR}/config.yaml"

echo "Setting up Alif Toolkit..."

# Determine source directory for tools
REPO_ROOT="$(cd "$(dirname "$0")" && pwd)"
TOOLKIT_SRC=""

# Check for local 'toolkit' folder (Distribution Mode)
if [ -d "${REPO_ROOT}/toolkit" ]; then
    TOOLKIT_SRC="${REPO_ROOT}/toolkit"
else
    # Fallback to Repo Source Mode
    if [ "$OS" = "darwin" ]; then
        TOOLKIT_SRC="${REPO_ROOT}/tools/setool/macos"
    elif [ "$OS" = "linux" ]; then
        TOOLKIT_SRC="${REPO_ROOT}/tools/setool/linux"
    elif [ "$OS" = "windows" ]; then
        TOOLKIT_SRC="${REPO_ROOT}/tools/setool/windows"
    fi
fi

if [ -d "$TOOLKIT_SRC" ]; then
    echo "Copying toolkit from ${TOOLKIT_SRC} to ${TOOLKIT_DEST}..."
    rm -rf "${TOOLKIT_DEST}"
    mkdir -p "${TOOLKIT_DEST}"
    cp -r "${TOOLKIT_SRC}/"* "${TOOLKIT_DEST}/"
    
    # Make executables executable (mac/linux)
    if [ "$OS" != "windows" ]; then
        chmod +x "${TOOLKIT_DEST}/app-gen-toc" "${TOOLKIT_DEST}/app-write-mram" 2>/dev/null || true
    fi
else
    echo "Warning: Toolkit source not found at ${TOOLKIT_SRC}. Skipping toolkit setup."
fi

# Generate Config if not exists
if [ ! -f "${CONFIG_FILE}" ]; then
    echo "Creating default configuration at ${CONFIG_FILE}..."
    mkdir -p "${ALIF_DIR}"
    echo "alif_tools_path: ${TOOLKIT_DEST}" > "${CONFIG_FILE}"
    echo "cmsis_toolbox: \"\"" >> "${CONFIG_FILE}"
    echo "gcc_toolchain: \"\"" >> "${CONFIG_FILE}"
    echo "cmsis_pack_root: \"\"" >> "${CONFIG_FILE}"
else
    # Update existing config to point to new toolkit?
    # Keeping user config is safer, but we want to enforce bundled toolkit?
    # Let's update alif_tools_path only if it's missing or we want to enforce.
    # User said "config the alif-cli to use that by default".
    # I'll append/update alif_tools_path.
    if grep -q "alif_tools_path" "${CONFIG_FILE}"; then
        # sed to replace
        if [ "$OS" = "darwin" ]; then
            sed -i '' "s|alif_tools_path:.*|alif_tools_path: ${TOOLKIT_DEST}|" "${CONFIG_FILE}"
        else
            sed -i "s|alif_tools_path:.*|alif_tools_path: ${TOOLKIT_DEST}|" "${CONFIG_FILE}"
        fi
    else
        echo "alif_tools_path: ${TOOLKIT_DEST}" >> "${CONFIG_FILE}"
    fi
fi

# 4. Configure Packs (if cpackget available)
echo "Configuring CMSIS Packs..."
if check_cmd cpackget; then
    echo "Found cpackget. Initializing packs..."
    cpackget init --pack-root "${ALIF_DIR}/packs" https://www.keil.com/pack/index.pidx
    
    echo "Adding Alif CMSIS Packs Index..."
    cpackget add https://raw.githubusercontent.com/saleh-mehdikhani/alif_cmsis_packs/main/AlifSemiconductor.pidx || true
    cpackget update-index || true
    
    # Update config to point to this pack root
    if [ "$OS" = "darwin" ]; then
        sed -i '' "s|cmsis_pack_root:.*|cmsis_pack_root: ${ALIF_DIR}/packs|" "${CONFIG_FILE}"
    else
        sed -i "s|cmsis_pack_root:.*|cmsis_pack_root: ${ALIF_DIR}/packs|" "${CONFIG_FILE}"
    fi
    echo "✅ CMSIS Packs configured at ${ALIF_DIR}/packs"
else
    echo "⚠️  cpackget not found. Skipping automatic pack configuration."
    echo "   (Install CMSIS Toolbox first, then run 'alif setup' to configure packs)"
fi

# 5. Verify & Instructions
if check_cmd ${APP_NAME}; then
    echo "--------------------------------------------------"
    echo "✅ ${APP_NAME} installed successfully!"
    echo "✅ Toolkit installed to ${TOOLKIT_DEST}"
    echo "✅ Configuration updated at ${CONFIG_FILE}"
    echo ""
    echo "To complete your setup, please install the following dependencies:"
    echo ""
    echo "1. CMSIS Toolbox:"
    echo "   Download the latest release for your OS from:"
    echo "   https://github.com/Open-CMSIS-Pack/cmsis-toolbox/releases"
    echo "   Extract it and configure Alif CLI to use it:"
    echo "   $ alif setup --cmsis /path/to/cmsis-toolbox/bin"
    echo ""
    echo "2. ARM GNU Toolchain (GCC):"
    if [ "$OS" = "darwin" ]; then
        echo "   Install via Homebrew:"
        echo "   $ brew install --cask gcc-arm-embedded"
        echo "   Then configure:"
        echo "   $ alif setup --gcc /Applications/ArmGNUToolchain/*/bin"
    elif [ "$OS" = "linux" ]; then
        echo "   Install via APT (Ubuntu/Debian):"
        echo "   $ sudo apt install gcc-arm-none-eabi"
        echo "   Then configure:"
        echo "   $ alif setup --gcc /usr/bin"
    elif [ "$OS" = "windows" ]; then
        echo "   Download installer from:"
        echo "   https://developer.arm.com/downloads/-/arm-gnu-toolchain-downloads"
        echo "   Then configure:"
        echo "   $ alif setup --gcc \"C:\\Program Files (x86)\\Arm GNU Toolchain\\...\\bin\""
    fi
    echo ""
    echo "You can verify your configuration by running:"
    echo "   $ alif setup --check"
    echo "--------------------------------------------------"
else
    echo "Error: Installation appeared to succeed but '${APP_NAME}' not found in PATH."
    exit 1
fi
