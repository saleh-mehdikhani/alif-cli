package flasher

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"alif-cli/internal/color"
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

// updateISPConfig updates the isp_config_data.cfg file with the selected port
func (f *Flasher) updateISPConfig(port string) error {
	configPath := filepath.Join(f.Cfg.AlifToolsPath, "isp_config_data.cfg")

	// Read the config file
	content, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read isp_config_data.cfg: %w", err)
	}

	// Replace comport line
	lines := strings.Split(string(content), "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "comport") {
			lines[i] = fmt.Sprintf("comport %s", port)
			break
		}
	}

	// Write back
	updated := strings.Join(lines, "\n")
	if err := os.WriteFile(configPath, []byte(updated), 0644); err != nil {
		return fmt.Errorf("failed to update isp_config_data.cfg: %w", err)
	}

	return nil
}

// flashViaJLink flashes using J-Link directly (JTAG method)
func (f *Flasher) flashViaJLink(binPath, tocPath, buildDir string) error {
	color.Info("Using J-Link for JTAG flashing...")

	// Get MRAM address from the signed binary's config
	mramAddr := "0x80000000" // Default
	tocAddr := "0x8057f0f0"  // Default TOC address

	// Generate J-Link script
	scriptPath := filepath.Join(buildDir, "flash_jlink.jlink")
	scriptContent := fmt.Sprintf(`si SWD
speed 4000
device Cortex-M55
connect
loadbin %s %s
loadbin %s %s
r
g
qc
`, binPath, mramAddr, tocPath, tocAddr)

	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0644); err != nil {
		return fmt.Errorf("failed to create J-Link script: %w", err)
	}

	color.Info("Generated J-Link script: %s", scriptPath)

	// Execute J-Link
	cmd := exec.Command("JLinkExe", "-CommandFile", scriptPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	color.Info("Starting J-Link flash...")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("J-Link flash failed: %w", err)
	}

	return nil
}

// eraseViaISP erases the application area using ISP
func (f *Flasher) eraseViaISP(verbose bool) error {
	args := []string{"-e", "APP"}
	if verbose {
		args = append(args, "-v")
	}

	cmd := exec.Command(filepath.Join(f.Cfg.AlifToolsPath, "app-write-mram"), args...)
	cmd.Dir = f.Cfg.AlifToolsPath
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// eraseViaJLink erases the application area using J-Link
func (f *Flasher) eraseViaJLink(buildDir string) error {
	scriptPath := filepath.Join(buildDir, "erase_jlink.jlink")
	scriptContent := `si SWD
speed 4000
device Cortex-M55
connect
erase 0x80000000 0x100000
r
g
qc
`

	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0644); err != nil {
		return fmt.Errorf("failed to create erase script: %w", err)
	}

	cmd := exec.Command("JLinkExe", "-CommandFile", scriptPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func (f *Flasher) Flash(binPath, tocPath, port, target, configPath string, noSwitch bool, method string, verbose bool, noErase bool) error {
	// 0. Update Flash Script based on config or target
	if err := f.updateScript(target, configPath); err != nil {
		return fmt.Errorf("failed to update flash script: %w", err)
	}

	buildDir := filepath.Dir(binPath)

	// ... (rest is same)
	color.Info("Staging files for flasher tool...")

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

	// 2. Stage TOC in toolkit build/ directory (where app-write-mram expects it)
	tocFiles := []string{"AppTocPackage.bin", "AppTocPackage.bin.sign", "AppTocPackage.bin.crt"}
	for _, fname := range tocFiles {
		src := filepath.Join(buildDir, fname)
		dst := filepath.Join(f.Cfg.AlifToolsPath, "build", fname)
		if _, err := os.Stat(src); err == nil {
			_ = copyFile(src, dst)
		}
	}

	// 3. Update ISP config with selected port
	if method == "ISP" {
		if err := f.updateISPConfig(port); err != nil {
			return fmt.Errorf("failed to update ISP config: %w", err)
		}
	}

	// 4. Erase application area before flashing (unless --no-erase)
	if !noErase {
		color.Info("Erasing application area...")
		switch method {
		case "ISP":
			if err := f.eraseViaISP(verbose); err != nil {
				color.Info("Warning: Erase failed (continuing anyway): %v", err)
			}
		case "JTAG":
			if err := f.eraseViaJLink(buildDir); err != nil {
				color.Info("Warning: Erase failed (continuing anyway): %v", err)
			}
		}
	}

	// 5. Run tool from toolkit ROOT
	color.Info("Flashing (%s) from %s onto %s...", method, buildDir, port)

	args := []string{}
	switch method {
	case "ISP":
		// Add pad flag (required for demos)
		args = append(args, "-p")
		if noSwitch {
			args = append(args, "-s")
		}
		// Use default baud rate (55000) - don't specify -b
		// Add verbose mode if requested
		if verbose {
			args = append(args, "-v")
		}
		// Let app-write-mram handle reset automatically
	case "JTAG":
		// JTAG not supported in v1.107.00 - use J-Link directly
		return f.flashViaJLink(binPath, tocPath, buildDir)
	}

	cmd := exec.Command(filepath.Join(f.Cfg.AlifToolsPath, "app-write-mram"), args...)
	cmd.Dir = f.Cfg.AlifToolsPath
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	color.Info("Starting execution...")
	return cmd.Run()
}

func (f *Flasher) updateScript(target, configPath string) error {
	scriptPath := filepath.Join(f.Cfg.AlifToolsPath, "bin", "application_package.ds")
	content, err := os.ReadFile(scriptPath)
	if err != nil {
		return err
	}

	appAddr := "0x80000000" // Default HE

	if configPath != "" {
		// Parse Config flexibly
		cfgBytes, err := os.ReadFile(configPath)
		if err == nil {
			var cfg map[string]interface{}
			if err := json.Unmarshal(cfgBytes, &cfg); err == nil {
				found := false
				for _, v := range cfg {
					if sub, ok := v.(map[string]interface{}); ok {
						if addr, exists := sub["mramAddress"]; exists {
							appAddr = fmt.Sprintf("%v", addr)
							color.Info("Using MRAM Address from Config: %s", appAddr)
							found = true
							break
						}
					}
				}
				if !found {
					color.Info("Warning: Could not find 'mramAddress' in config, using fallback logic.")
				}
			} else {
				color.Info("Warning: Failed to parse config JSON, using fallback logic.")
			}
		} else {
			color.Error("Warning: Could not read config file %s", configPath)
		}
	}

	// Fallback to heuristic if still default (or just overwrite if no config provided)
	if configPath == "" {
		if strings.Contains(target, "HP") {
			appAddr = "0x80200000" // HP Core Offset
			color.Info("Detected HP Target (%s): Flashing to 0x80200000", target)
		} else {
			color.Info("Detected HE Target (%s): Flashing to 0x80000000", target)
		}
	}

	// The TOC address is generally fixed at the end of MRAM partition for App
	tocAddr := "0x8057f0f0"

	// Construct the line
	newLine := fmt.Sprintf("set semihosting args ../build/images/alif-img.bin %s ../AppTocPackage.bin %s", appAddr, tocAddr)

	lines := strings.Split(string(content), "\n")
	if len(lines) > 0 {
		lines[0] = newLine
	}

	return os.WriteFile(scriptPath, []byte(strings.Join(lines, "\n")), 0644)
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
