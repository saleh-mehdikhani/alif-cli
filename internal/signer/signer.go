package signer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"alif-cli/internal/config"
	"alif-cli/internal/targets"
	"alif-cli/internal/ui"
)

type Signer struct {
	Cfg *config.Config
}

func New(cfg *config.Config) *Signer {
	return &Signer{Cfg: cfg}
}

// SignArtifact creates a bootable image.
func (s *Signer) SignArtifact(projectDir, buildDir, binaryPath string, coreHint, projectHint, configPathOverride string) (string, error) {
	ui.Header("Create Bootable Image")

	// Use ResolveTargetConfig to find the config file with hints
	// ResolveTargetConfig handles its own UI items (Config Selection)
	_, srcCfg, err := targets.ResolveTargetConfig(configPathOverride, projectDir, coreHint, projectHint)
	if err != nil {
		return "", fmt.Errorf("failed to resolve signing config: %w", err)
	}

	// 1. Load Config (to find 'binary' path mapping)
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
	ui.Item("Staging", filepath.Base(toolkitBinPath))

	if err := os.MkdirAll(filepath.Dir(toolkitBinPath), 0755); err != nil {
		return "", fmt.Errorf("failed to create directory for binary: %w", err)
	}

	_ = os.Remove(toolkitBinPath)
	if err := copyFile(binaryPath, toolkitBinPath); err != nil {
		return "", fmt.Errorf("failed to copy binary: %w", err)
	}

	// 3. Run tool from ROOT with original config (no patching needed)
	toolPath := filepath.Join(s.Cfg.AlifToolsPath, "app-gen-toc")
	cmd := exec.Command(toolPath, "-f", srcCfg, "-o", "build/AppTocPackage.bin")
	cmd.Dir = s.Cfg.AlifToolsPath

	// Capture output
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	sp := ui.StartSpinner("Running app-gen-toc...")
	if err := cmd.Run(); err != nil {
		sp.Fail("TOC generation failed")
		fmt.Println("\n" + output.String()) // Print full tool output
		return "", fmt.Errorf("app-gen-toc failed: %w", err)
	}
	sp.Succeed("TOC generated successfully")

	// 4. Retrieve ALL generated artifacts back to Project buildDir
	// ui.Info("Retrieving artifacts...")

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

	// Report result
	finalToc := filepath.Join(buildDir, "AppTocPackage.bin")
	// ui.Item("Output", filepath.Base(finalToc))

	return finalToc, nil
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
