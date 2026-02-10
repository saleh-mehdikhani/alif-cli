package project

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// IsSolutionRoot checks if the given directory (or current dir if empty)
// looks like a valid Alif solution (CMSIS csolution based).
func IsSolutionRoot(dir string) (string, error) {
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return "", err
		}
	}

	// Make absolute path
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}

	entries, err := os.ReadDir(absDir)
	if err != nil {
		return "", err
	}

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".csolution.yml") {
			return absDir, nil
		}
	}

	return "", errors.New("no .csolution.yml file found in this directory")
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
