#!/bin/bash
set -e

# Version - Should ideally come from argument or go.mod
VERSION=$(git describe --tags --always 2>/dev/null || echo "dev")
APP_NAME="alif"
OUTPUT_DIR="dist"

echo "Packaging ${APP_NAME} version ${VERSION}..."

rm -rf "${OUTPUT_DIR}"
mkdir -p "${OUTPUT_DIR}"

# 1. Targets
TARGETS=(
    "darwin/arm64"
    "darwin/amd64"
    "linux/amd64"
    "windows/amd64"
)

for target in "${TARGETS[@]}"; do
    IFS="/" read -r OS ARCH <<< "${target}"
    
    echo "Building for ${OS}/${ARCH}..."
    export GOOS=${OS}
    export GOARCH=${ARCH}
    
    if [[ "${OS}" == "darwin" ]]; then
        export CGO_ENABLED=1
    else
        export CGO_ENABLED=0
    fi
    
    FOLDER="${APP_NAME}-${VERSION}-${OS}-${ARCH}"
    mkdir -p "${OUTPUT_DIR}/${FOLDER}"
    
    BINARY_NAME="${APP_NAME}"
    if [ "${OS}" == "windows" ]; then
        BINARY_NAME="${APP_NAME}.exe"
    fi
    
    go build -o "${OUTPUT_DIR}/${FOLDER}/${BINARY_NAME}" .
    
    # 2. Add Install Scripts and Docs
    echo " > Adding toolkit..."
    TOOLKIT_SRC=""
    if [[ "${OS}" == "darwin" ]]; then
        TOOLKIT_SRC="tools/setool/macos"
    elif [[ "${OS}" == "linux" ]]; then
        TOOLKIT_SRC="tools/setool/linux"
    elif [[ "${OS}" == "windows" ]]; then
        TOOLKIT_SRC="tools/setool/windows"
    fi
    
    if [ -d "${TOOLKIT_SRC}" ]; then
        mkdir -p "${OUTPUT_DIR}/${FOLDER}/toolkit"
        cp -r "${TOOLKIT_SRC}/"* "${OUTPUT_DIR}/${FOLDER}/toolkit/"
        if [[ "${OS}" != "windows" ]]; then
             chmod +x "${OUTPUT_DIR}/${FOLDER}/toolkit/app-gen-toc" "${OUTPUT_DIR}/${FOLDER}/toolkit/app-write-mram" 2>/dev/null || true
        fi
    else
        echo "Warning: Toolkit missing for ${OS} at ${TOOLKIT_SRC}"
    fi

    cp README.md "${OUTPUT_DIR}/${FOLDER}/"
    if [ "${OS}" != "windows" ]; then
        cp install.sh "${OUTPUT_DIR}/${FOLDER}/"
        chmod +x "${OUTPUT_DIR}/${FOLDER}/install.sh"
    else
        # Could generate install.bat for windows
        echo "@echo off" > "${OUTPUT_DIR}/${FOLDER}/install.bat"
        echo "echo Installing Alif CLI..." >> "${OUTPUT_DIR}/${FOLDER}/install.bat"
        echo "copy ${BINARY_NAME} %USERPROFILE%\\AppData\\Local\\Microsoft\\WindowsApps\\" >> "${OUTPUT_DIR}/${FOLDER}/install.bat"
        echo "echo Done!" >> "${OUTPUT_DIR}/${FOLDER}/install.bat"
    fi
    
    # 3. Archive
    if [ "${OS}" == "windows" ]; then
        # ZIP
        cd "${OUTPUT_DIR}"
        zip -r "${FOLDER}.zip" "${FOLDER}" >/dev/null
        cd ..
    else
        # TAR.GZ
        tar -czf "${OUTPUT_DIR}/${FOLDER}.tar.gz" -C "${OUTPUT_DIR}" "${FOLDER}"
    fi
    
    # Cleaning up folder
    rm -rf "${OUTPUT_DIR}/${FOLDER}"
    
    echo " > Created ${OUTPUT_DIR}/${FOLDER}"
done

echo "--------------------------------------------------"
echo "Release packaging complete in ${OUTPUT_DIR}/"
ls -lh "${OUTPUT_DIR}"
echo "--------------------------------------------------"
