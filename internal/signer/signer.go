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
	resolvedCfg, srcCfg, err := targets.ResolveTargetConfig(configPathOverride, projectDir, coreHint, projectHint)
	if err != nil {
		return "", fmt.Errorf("failed to resolve signing config: %w", err)
	}

	// Sync Toolkit Config to match the detected device
	if err := targets.SyncToolkitConfig(s.Cfg.AlifToolsPath, resolvedCfg.GetCPU()); err != nil {
		ui.Warn(fmt.Sprintf("Toolkit sync failed: %v", err))
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

	// 2. Find the application binary path in config (prioritize USER_APP or entries with mramAddress)
	var binaryPathInConfig string
	if userApp, ok := cfg["USER_APP"].(map[string]interface{}); ok {
		if bin, ok := userApp["binary"].(string); ok {
			binaryPathInConfig = bin
		}
	}

	if binaryPathInConfig == "" {
		for k, v := range cfg {
			if k == "DEVICE" {
				continue
			}
			if sub, ok := v.(map[string]interface{}); ok {
				if _, exists := sub["mramAddress"]; exists {
					if bin, ok := sub["binary"].(string); ok {
						binaryPathInConfig = bin
						break
					}
				}
			}
		}
	}

	if binaryPathInConfig == "" {
		return "", fmt.Errorf("could not find application binary field in config")
	}

	// Double Staging: app-gen-toc is picky about locations.
	// 1. Stage in toolkit root (legacy/internal reference)
	rootDst := filepath.Join(s.Cfg.AlifToolsPath, binaryPathInConfig)
	ui.Item("Staging", filepath.Base(rootDst))
	if err := os.MkdirAll(filepath.Dir(rootDst), 0755); err != nil {
		return "", fmt.Errorf("failed to create directory for binary: %w", err)
	}
	_ = os.Remove(rootDst)
	if err := copyFile(binaryPath, rootDst); err != nil {
		return "", fmt.Errorf("failed to copy binary to root: %w", err)
	}

	// 2. Stage in build/images/ (required for newer tool versions/specific configs)
	imagesDst := filepath.Join(s.Cfg.AlifToolsPath, "build", "images", binaryPathInConfig)
	if imagesDst != rootDst {
		if err := os.MkdirAll(filepath.Dir(imagesDst), 0755); err != nil {
			return "", fmt.Errorf("failed to create build/images directory: %w", err)
		}
		_ = os.Remove(imagesDst)
		if err := copyFile(binaryPath, imagesDst); err != nil {
			return "", fmt.Errorf("failed to copy binary to build/images: %w", err)
		}
	}

	// 3. Copy config to toolkit dir to ensure relative paths work (staging)
	stagedCfgPath := filepath.Join(s.Cfg.AlifToolsPath, "staged_config.json")
	if err := copyFile(srcCfg, stagedCfgPath); err != nil {
		return "", fmt.Errorf("failed to stage config file: %w", err)
	}
	defer os.Remove(stagedCfgPath)

	// 4. Run tool from ROOT with STAGED config
	toolPath := filepath.Join(s.Cfg.AlifToolsPath, "app-gen-toc")
	cmd := exec.Command(toolPath, "-f", "staged_config.json", "-o", "build/AppTocPackage.bin")
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
	artifacts := []struct{ src, dst string }{
		{filepath.Join(s.Cfg.AlifToolsPath, "build", "app-package-map.txt"), filepath.Join(buildDir, "app-package-map.txt")},
		{filepath.Join(s.Cfg.AlifToolsPath, "build", "AppTocPackage.bin"), filepath.Join(buildDir, "AppTocPackage.bin")},
		{filepath.Join(s.Cfg.AlifToolsPath, "build", "AppTocPackage.bin.sign"), filepath.Join(buildDir, "AppTocPackage.bin.sign")},
		{filepath.Join(s.Cfg.AlifToolsPath, "build", "AppTocPackage.bin.crt"), filepath.Join(buildDir, "AppTocPackage.bin.crt")},
		{rootDst + ".sign", filepath.Join(buildDir, "alif-img.bin.sign")},
		{rootDst + ".crt", filepath.Join(buildDir, "alif-img.bin.crt")},
		{rootDst, filepath.Join(buildDir, "alif-img.bin")},
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
