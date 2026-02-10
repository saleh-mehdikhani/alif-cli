package assets

import (
	"embed"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
)

//go:embed presets/**/*
var Presets embed.FS

// Extract recursively copies embedded presets to the destination.
func Extract(destDir string) error {
	return extractRecursive("presets", destDir)
}

func extractRecursive(embedPath, diskPath string) error {
	entries, err := Presets.ReadDir(embedPath)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(diskPath, 0755); err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := path.Join(embedPath, entry.Name()) // embed uses forward slash
		dstPath := filepath.Join(diskPath, entry.Name())

		if entry.IsDir() {
			if err := extractRecursive(srcPath, dstPath); err != nil {
				return err
			}
			continue
		}

		srcFile, err := Presets.Open(srcPath)
		if err != nil {
			return err
		}

		destFile, err := os.Create(dstPath)
		if err != nil {
			srcFile.Close()
			return err
		}

		if _, err := io.Copy(destFile, srcFile); err != nil {
			srcFile.Close()
			destFile.Close()
			return err
		}

		srcFile.Close()
		destFile.Close()
		fmt.Printf("Extracted: %s\n", dstPath)
	}
	return nil
}
