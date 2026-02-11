package signer

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"alif-cli/internal/config"
	"alif-cli/internal/project"
)

type Signer struct {
	Cfg *config.Config
}

func New(cfg *config.Config) *Signer {
	return &Signer{Cfg: cfg}
}

func (s *Signer) SignArtifact(projectDir, buildDir, binaryPath string, targetCore string, configPathOverride string) (string, error) {
	fmt.Println("DEBUG: Entering SignArtifact")

	var srcCfg string
	if configPathOverride != "" {
		srcCfg = configPathOverride
		if _, err := os.Stat(srcCfg); err != nil {
			return "", fmt.Errorf("provided config file not found: %s", srcCfg)
		}
		// Convert to absolute path
		var err error
		srcCfg, err = filepath.Abs(srcCfg)
		if err != nil {
			return "", fmt.Errorf("failed to get absolute path for config: %w", err)
		}
		fmt.Printf("Using provided signing config: %s\n", srcCfg)
	} else {
		// Standard Lookup
		cfgName := fmt.Sprintf("%s_cfg.json", project.GetCoreName(targetCore))
		srcCfg = filepath.Join(projectDir, ".alif", cfgName)

		if _, err := os.Stat(srcCfg); os.IsNotExist(err) {
			return "", fmt.Errorf("signing config %s not found in .alif/ directory (presets removed, please provide local config)", cfgName)
		}
	}

	// 1. Load Config
	cfgBytes, err := os.ReadFile(srcCfg)
	if err != nil {
		return "", fmt.Errorf("failed to read signing config: %w", err)
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(cfgBytes, &cfg); err != nil {
		return "", fmt.Errorf("failed to parse signing config: %w", err)
	}

	// 2. Find the binary path in config and copy binary to that location
	var binaryPathInConfig string
	for _, v := range cfg {
		if sub, ok := v.(map[string]interface{}); ok {
			if binPath, exists := sub["binary"]; exists {
				if binPathStr, ok := binPath.(string); ok {
					binaryPathInConfig = binPathStr
					break
				}
			}
		}
	}

	if binaryPathInConfig == "" {
		return "", fmt.Errorf("could not find 'binary' field in config")
	}

	// Copy binary to the path specified in config (relative to toolkit directory)
	toolkitBinPath := filepath.Join(s.Cfg.AlifToolsPath, binaryPathInConfig)

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(toolkitBinPath), 0755); err != nil {
		return "", fmt.Errorf("failed to create directory for binary: %w", err)
	}

	_ = os.Remove(toolkitBinPath)
	if err := copyFile(binaryPath, toolkitBinPath); err != nil {
		return "", fmt.Errorf("failed to copy binary: %w", err)
	}
	// Don't remove the binary - app-write-mram needs it at this exact path

	// 3. Run tool from ROOT with original config (no patching needed)
	toolPath := filepath.Join(s.Cfg.AlifToolsPath, "app-gen-toc")
	cmd := exec.Command(toolPath, "-f", srcCfg, "-o", "build/AppTocPackage.bin")
	cmd.Dir = s.Cfg.AlifToolsPath
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Println("DEBUG: Running app-gen-toc from Toolkit Root...")
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("app-gen-toc failed: %w", err)
	}

	// 4. Retrieve ALL generated artifacts back to Project buildDir
	fmt.Println("Retrieving all generated artifacts...")

	artifacts := []struct{ src, dst string }{
		{filepath.Join(s.Cfg.AlifToolsPath, "build", "AppTocPackage.bin"), filepath.Join(buildDir, "AppTocPackage.bin")},
		{filepath.Join(s.Cfg.AlifToolsPath, "build", "AppTocPackage.bin.sign"), filepath.Join(buildDir, "AppTocPackage.bin.sign")},
		{filepath.Join(s.Cfg.AlifToolsPath, "build", "AppTocPackage.bin.crt"), filepath.Join(buildDir, "AppTocPackage.bin.crt")},
		{toolkitBinPath + ".sign", filepath.Join(buildDir, "alif-img.bin.sign")},
		{toolkitBinPath + ".crt", filepath.Join(buildDir, "alif-img.bin.crt")},
		{toolkitBinPath, filepath.Join(buildDir, "alif-img.bin")},
	}

	for _, a := range artifacts {
		if _, err := os.Stat(a.src); err == nil {
			_ = os.Remove(a.dst)
			if err := os.Rename(a.src, a.dst); err != nil {
				_ = copyFile(a.src, a.dst)
				_ = os.Remove(a.src)
			}
		}
	}

	return filepath.Join(buildDir, "AppTocPackage.bin"), nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
