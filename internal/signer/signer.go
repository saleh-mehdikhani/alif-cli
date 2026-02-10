package signer

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"alif-cli/internal/config"
)

type Signer struct {
	Cfg *config.Config
}

func New(cfg *config.Config) *Signer {
	return &Signer{Cfg: cfg}
}

func (s *Signer) SignArtifact(projectDir, buildDir, binaryPath string, targetCore string) (string, error) {
	fmt.Println("DEBUG: Entering SignArtifact")
	cfgName := fmt.Sprintf("%s_cfg.json", getCoreName(targetCore))

	srcCfg := filepath.Join(projectDir, ".alif", cfgName)
	if _, err := os.Stat(srcCfg); os.IsNotExist(err) {
		// Fallback to global presets
		home, _ := os.UserHomeDir()
		fallbackCfg := filepath.Join(home, ".alif", "presets", "signing", cfgName)
		if _, err := os.Stat(fallbackCfg); err == nil {
			fmt.Printf("Using global signing preset: %s\n", fallbackCfg)
			srcCfg = fallbackCfg
		} else {
			return "", fmt.Errorf("signing config %s not found in .alif/ directory or ~/.alif/presets/signing", cfgName)
		}
	}

	// 1. Stage Config in toolkit ROOT
	toolkitCfg := filepath.Join(s.Cfg.AlifToolsPath, "alif-img.json")
	_ = os.Remove(toolkitCfg)
	if err := copyFile(srcCfg, toolkitCfg); err != nil {
		return "", fmt.Errorf("failed to stage config: %w", err)
	}
	defer os.Remove(toolkitCfg)

	// 2. Stage Binary in toolkit ROOT
	toolkitBin := filepath.Join(s.Cfg.AlifToolsPath, "alif-img.bin")
	_ = os.Remove(toolkitBin)
	_ = copyFile(binaryPath, toolkitBin) // Copy instead of symlink to be safe
	defer os.Remove(toolkitBin)

	// 3. Run tool from ROOT
	toolPath := filepath.Join(s.Cfg.AlifToolsPath, "app-gen-toc")
	cmd := exec.Command(toolPath, "-f", "alif-img.json", "-o", "AppTocPackage.bin")
	cmd.Dir = s.Cfg.AlifToolsPath
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Println("DEBUG: Running app-gen-toc from Toolkit Root...")
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("app-gen-toc failed: %w", err)
	}

	// 4. Retrieve ALL generated artifacts back to Project buildDir
	fmt.Println("Retrieving all generated artifacts...")

	imagesDir := filepath.Join(s.Cfg.AlifToolsPath, "build", "images")

	artifacts := []struct{ src, dst string }{
		{filepath.Join(s.Cfg.AlifToolsPath, "AppTocPackage.bin"), filepath.Join(buildDir, "AppTocPackage.bin")},
		{filepath.Join(s.Cfg.AlifToolsPath, "AppTocPackage.bin.sign"), filepath.Join(buildDir, "AppTocPackage.bin.sign")},
		{filepath.Join(s.Cfg.AlifToolsPath, "AppTocPackage.bin.crt"), filepath.Join(buildDir, "AppTocPackage.bin.crt")},
		{filepath.Join(imagesDir, "alif-img.bin.sign"), filepath.Join(buildDir, "alif-img.bin.sign")},
		{filepath.Join(imagesDir, "alif-img.bin.crt"), filepath.Join(buildDir, "alif-img.bin.crt")},
		{toolkitBin, filepath.Join(buildDir, "alif-img.bin")},
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

func getCoreName(target string) string {
	if strings.Contains(target, "HE") {
		return "M55_HE"
	}
	if strings.Contains(target, "HP") {
		return "M55_HP"
	}
	return "M55_HE"
}
