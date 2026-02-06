package flasher

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"alif-cli/internal/config"

	"go.bug.st/serial/enumerator"
)

type Flasher struct {
	Cfg *config.Config
}

func New(cfg *config.Config) *Flasher {
	return &Flasher{Cfg: cfg}
}

func (f *Flasher) SelectPort() (string, error) {
	ports, err := enumerator.GetDetailedPortsList()
	if err != nil {
		return "", fmt.Errorf("failed to list ports: %w", err)
	}

	if len(ports) == 0 {
		return "", fmt.Errorf("no serial ports found")
	}

	var candidates []*enumerator.PortDetails
	fmt.Println("Detected Serial Ports:")
	idx := 1
	for _, p := range ports {
		name := strings.ToLower(p.Name)
		vid := strings.ToLower(p.VID)
		isCandidate := false

		if strings.Contains(name, "usbmodem") || strings.Contains(name, "jlink") || strings.Contains(name, "mbed") {
			isCandidate = true
		}

		fmt.Printf("[%d] %s (VID:%s PID:%s Serial:%s)\n", idx, p.Name, vid, p.PID, p.SerialNumber)
		if isCandidate {
			candidates = append(candidates, p)
		}
		idx++
	}

	if len(candidates) == 1 {
		p := candidates[0].Name
		fmt.Printf("Auto-selecting only candidate: %s\n", p)
		return p, nil
	}

	fmt.Print("Select port number: ")
	var selected int
	_, err = fmt.Scanln(&selected)
	if err != nil || selected < 1 || selected > len(candidates) {
		return "", fmt.Errorf("invalid selection")
	}

	return candidates[selected-1].Name, nil
}

func (f *Flasher) Flash(binPath, tocPath, port string) error {
	buildDir := filepath.Dir(binPath)

	// Simple staging: Copy to toolkit in expected locations
	fmt.Println("Staging files for flasher tool...")

	// 1. Stage Image in toolkit/build/images
	imagesDir := filepath.Join(f.Cfg.AlifToolsPath, "build", "images")
	_ = os.MkdirAll(imagesDir, 0755)

	imageFiles := []string{"alif-img.bin", "alif-img.bin.sign", "alif-img.bin.crt"}
	for _, fname := range imageFiles {
		src := filepath.Join(buildDir, fname)
		dst := filepath.Join(imagesDir, fname)
		if _, err := os.Stat(src); err == nil {
			_ = copyFile(src, dst)
		}
	}

	// 2. Stage TOC in toolkit root
	tocFiles := []string{"AppTocPackage.bin", "AppTocPackage.bin.sign", "AppTocPackage.bin.crt"}
	for _, fname := range tocFiles {
		src := filepath.Join(buildDir, fname)
		dst := filepath.Join(f.Cfg.AlifToolsPath, fname)
		if _, err := os.Stat(src); err == nil {
			_ = copyFile(src, dst)
		}
	}

	// 3. Run tool from toolkit ROOT
	fmt.Printf("Flashing from %s to port %s...\n", buildDir, port)
	cmd := exec.Command(filepath.Join(f.Cfg.AlifToolsPath, "app-write-mram"))
	cmd.Dir = f.Cfg.AlifToolsPath
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Println("Starting execution...")
	return cmd.Run()
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
