package flasher

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"alif-cli/internal/config"
	"alif-cli/internal/ui"

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
	// ui.Header("Select Serial Port") // Flash command usually handles header "Flash Target"

	idx := 1
	for _, p := range ports {
		name := strings.ToLower(p.Name)
		isCandidate := false

		if strings.Contains(name, "usbmodem") || strings.Contains(name, "jlink") || strings.Contains(name, "mbed") {
			isCandidate = true
		}

		if isCandidate {
			candidates = append(candidates, p)
		}
		idx++
	}

	// Fallback if no "candidate" found, show all?
	if len(candidates) == 0 {
		candidates = ports
	}

	if len(candidates) == 1 {
		p := candidates[0].Name
		ui.Item("Port", p) // Auto-selected
		return p, nil
	}

	fmt.Println("Detected Serial Ports:")
	for i, p := range candidates {
		fmt.Printf("[%d] %s (VID:%s PID:%s Serial:%s)\n", i+1, p.Name, p.VID, p.PID, p.SerialNumber)
	}

	fmt.Print("Select port number: ")
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	selection, err := strconv.Atoi(strings.TrimSpace(input))
	if err != nil || selection < 1 || selection > len(candidates) {
		return "", fmt.Errorf("invalid selection")
	}

	selectedPort := candidates[selection-1].Name
	ui.Item("Port", selectedPort)
	return selectedPort, nil
}

func (f *Flasher) updateISPConfig(port string) error {
	configPath := filepath.Join(f.Cfg.AlifToolsPath, "isp_config_data.cfg")
	content, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read isp_config_data.cfg: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "comport") {
			lines[i] = fmt.Sprintf("comport %s", port)
			break
		}
	}

	updated := strings.Join(lines, "\n")
	if err := os.WriteFile(configPath, []byte(updated), 0644); err != nil {
		return fmt.Errorf("failed to update isp_config_data.cfg: %w", err)
	}
	return nil
}

func (f *Flasher) flashViaJLink(binPath, tocPath, buildDir string) error {
	ui.Info("Using J-Link for JTAG flashing...")
	// ... logic similar to before, strictly script generation ...
	// Since JTAG is not primary focus and complicated to suppress interactive usage of JLinkExe?
	// JLinkExe is usually interactive or script driven.
	// For now, I'll allow JLink stdout to pass through if JTAG selected, or suppress.
	// Suppressing JLink might hide important connectivity errors.
	// But sticking to consistency: Suppress unless error.

	// Default addrs
	mramAddr := "0x80000000"
	tocAddr := "0x8057f0f0"

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

	cmd := exec.Command("JLinkExe", "-CommandFile", scriptPath)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	sp := ui.StartSpinner("Flashing via J-Link...")
	if err := cmd.Run(); err != nil {
		sp.Fail("J-Link failed")
		fmt.Println("\n" + output.String())
		return fmt.Errorf("J-Link flash failed: %w", err)
	}
	sp.Succeed("Flashed successfully via JTAG")
	return nil
}

func (f *Flasher) eraseViaISP(verbose bool) error {
	args := []string{"-e", "APP"}
	if verbose {
		args = append(args, "-v")
	}

	cmd := exec.Command(filepath.Join(f.Cfg.AlifToolsPath, "app-write-mram"), args...)
	cmd.Dir = f.Cfg.AlifToolsPath
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	sp := ui.StartSpinner("Erasing application area...")
	if err := cmd.Run(); err != nil {
		sp.Fail("Erase failed")
		if verbose {
			fmt.Println("\n" + output.String())
		}
		return err
	}
	sp.Succeed("Erased successfully")
	return nil
}

func (f *Flasher) Flash(binPath, tocPath, port, target, configPath string, noSwitch bool, method string, verbose bool, noErase bool) error {
	// 0. Update Flash Script based on config or target
	if err := f.updateScript(target, configPath); err != nil {
		return fmt.Errorf("failed to update flash script: %w", err)
	}

	buildDir := filepath.Dir(binPath)

	ui.Item("Method", method)
	// ui.Item("Port", port) // Already printed by SelectPort? No, SelectPort called before.
	// If caller prints header, we print items.

	// 1. Stage Image
	// Simple copy, silent
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

	// 2. Stage TOC
	tocFiles := []string{"AppTocPackage.bin", "AppTocPackage.bin.sign", "AppTocPackage.bin.crt"}
	for _, fname := range tocFiles {
		src := filepath.Join(buildDir, fname)
		dst := filepath.Join(f.Cfg.AlifToolsPath, "build", fname)
		if _, err := os.Stat(src); err == nil {
			_ = copyFile(src, dst)
		}
	}

	// 3. Update ISP config
	if method == "ISP" {
		if err := f.updateISPConfig(port); err != nil {
			return fmt.Errorf("failed to update ISP config: %w", err)
		}
	}

	// 4. Erase
	if !noErase && method == "ISP" { // Only ISP erase implemented cleanly
		if err := f.eraseViaISP(verbose); err != nil {
			// warning logged inside
		}
	}

	// 5. Flash
	if method == "JTAG" {
		return f.flashViaJLink(binPath, tocPath, buildDir)
	}

	// ISP Flash
	args := []string{"-p"}
	if noSwitch {
		args = append(args, "-s")
	}
	if verbose {
		args = append(args, "-v")
	}

	cmd := exec.Command(filepath.Join(f.Cfg.AlifToolsPath, "app-write-mram"), args...)
	cmd.Dir = f.Cfg.AlifToolsPath
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	sp := ui.StartSpinner(fmt.Sprintf("Flashing %s...", target))
	if err := cmd.Run(); err != nil {
		sp.Fail("Flash failed")
		fmt.Println("\n" + output.String())
		return err
	}
	sp.Succeed("Flash complete!")
	return nil
}

func (f *Flasher) updateScript(target, configPath string) error {
	scriptPath := filepath.Join(f.Cfg.AlifToolsPath, "bin", "application_package.ds")
	content, err := os.ReadFile(scriptPath)
	if err != nil {
		return err
	}

	appAddr := "0x80000000" // Default HE

	if configPath != "" {
		cfgBytes, err := os.ReadFile(configPath)
		if err == nil {
			var cfg map[string]interface{}
			if err := json.Unmarshal(cfgBytes, &cfg); err == nil {
				found := false
				for _, v := range cfg {
					if sub, ok := v.(map[string]interface{}); ok {
						if addr, exists := sub["mramAddress"]; exists {
							appAddr = fmt.Sprintf("%v", addr)
							found = true
							break
						}
					}
				}
				if !found {
					// Silent fallback logic
				}
			}
		}
	}

	if configPath == "" {
		if strings.Contains(target, "HP") {
			appAddr = "0x80200000"
		}
	}

	tocAddr := "0x8057f0f0"
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
