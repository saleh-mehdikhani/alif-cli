package project

import (
	"errors"
	"os"
	"path/filepath"
)

// IsProjectRoot checks if the given directory (or current dir if empty)
// looks like a valid Alif project (CMSIS csolution based).
func IsProjectRoot(dir string) (string, error) {
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return "", err
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}

	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".yml" {
			// Basic check for .csolution.yml or similar
			// You could parse file content to be stricter
			if len(entry.Name()) > 10 { // simplistic check
				return dir, nil
			}
		}
	}

	return "", errors.New("no project found in this directory (looking for *.yml solution files)")
}

// FindCsolution finds the .csolution.yml file in the directory
func FindCsolution(dir string) (string, error) {
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return "", err
		}
	}

	files, err := filepath.Glob(filepath.Join(dir, "*.csolution.yml"))
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "", errors.New("no .csolution.yml file found")
	}
	// Return the first one found
	return files[0], nil
}
