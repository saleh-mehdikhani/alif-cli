package flasher

import (
	"bufio"
	"bytes"
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

func (f *Flasher) UpdateISPConfig(port string) error {
	configPath := filepath.Join(f.Cfg.AlifToolsPath, "isp_config_data.cfg")
	content, err := os.ReadFile(configPath)

	if os.IsNotExist(err) {
		// Create default config if missing
		defaultConfig := fmt.Sprintf("comport %s\nbaudrate 115200\n", port)
		if err := os.WriteFile(configPath, []byte(defaultConfig), 0644); err != nil {
			return fmt.Errorf("failed to create isp_config_data.cfg: %w", err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to read isp_config_data.cfg: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	found := false
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "comport") {
			lines[i] = fmt.Sprintf("comport %s", port)
			found = true
			break
		}
	}

	if !found {
		// Append comport if not present in existing file
		lines = append(lines, fmt.Sprintf("comport %s", port))
	}

	updated := strings.Join(lines, "\n")
	if err := os.WriteFile(configPath, []byte(updated), 0644); err != nil {
		return fmt.Errorf("failed to update isp_config_data.cfg: %w", err)
	}
	return nil
}

func (f *Flasher) flashViaJLink(binPath, tocPath, buildDir, device, scriptPathOverride string) error {
	ui.Info("Using J-Link for JTAG flashing...")

	// Resolve addrs from map file
	mramAddr, err := f.resolveBinaryAddress(buildDir)
	if err != nil {
		return err
	}
	tocAddr, err := f.resolveTOCAddress(buildDir)
	if err != nil {
		return err
	}

	scriptPath := filepath.Join(buildDir, "flash_jlink.jlink")
	scriptContent := fmt.Sprintf(`si SWD
speed 4000
device %s
connect
loadbin %s %s
loadbin %s %s
r
g
qc
`, device, binPath, mramAddr, tocPath, tocAddr)

	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0644); err != nil {
		return fmt.Errorf("failed to create J-Link script: %w", err)
	}

	args := []string{"-CommandFile", scriptPath}
	if scriptPathOverride != "" {
		args = append([]string{"-JLinkScriptFile", scriptPathOverride}, args...)
	}

	cmd := exec.Command("JLinkExe", args...)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	sp := ui.StartSpinner(fmt.Sprintf("Flashing %s via J-Link...", device))
	if err := cmd.Run(); err != nil {
		sp.Fail("J-Link failed")
		fmt.Println("\n" + output.String())
		return fmt.Errorf("J-Link flash failed: %w", err)
	}
	sp.Succeed("Flashed successfully via JTAG")
	return nil
}

func (f *Flasher) EraseViaISP(verbose bool) error {
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

func (f *Flasher) Flash(binPath, tocPath, port, target, configPath string, noSwitch bool, method string, verbose bool, doErase bool) error {
	buildDir := filepath.Dir(binPath)

	ui.Item("Method", method)
	// ui.Item("Port", port) // Already printed by SelectPort? No, SelectPort called before.
	// If caller prints header, we print items.

	// 1. Stage Image inside toolkit (bundled Python in app-write-mram needs files in toolkit)
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

	// 2. Stage TOC to root and build/ directory (different tools expect different locations)
	tocFiles := []string{"AppTocPackage.bin", "AppTocPackage.bin.sign", "AppTocPackage.bin.crt"}
	buildDestDir := filepath.Join(f.Cfg.AlifToolsPath, "build")
	for _, fname := range tocFiles {
		src := filepath.Join(buildDir, fname)
		// Stage to root
		dstRoot := filepath.Join(f.Cfg.AlifToolsPath, fname)
		if _, err := os.Stat(src); err == nil {
			_ = copyFile(src, dstRoot)
		}
		// Stage to build/
		dstBuild := filepath.Join(buildDestDir, fname)
		if _, err := os.Stat(src); err == nil {
			_ = copyFile(src, dstBuild)
		}
	}

	// 3. Update ISP config
	if method == "ISP" {
		if err := f.UpdateISPConfig(port); err != nil {
			return fmt.Errorf("failed to update ISP config: %w", err)
		}

		// 3b. Erase if requested
		if doErase {
			if err := f.EraseViaISP(verbose); err != nil {
				// We warn but continue, as the -p command might still work if erase failed
				ui.Warn(fmt.Sprintf("Automatic erase failed: %v", err))
			}
		}
	}

	// 5. Flash
	if method == "JTAG" {
		device, script := f.resolveJLinkConfig(buildDir, target)
		return f.flashViaJLink(binPath, tocPath, buildDir, device, script)
	}

	// 4. Flash (app-write-mram uses the script located in bin/application_package.ds)
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

func (f *Flasher) resolveJLinkConfig(buildDir, target string) (string, string) {
	device := "Cortex-M55"
	script := ""

	// Find project root (look for .alif)
	curr := buildDir
	var alifDir string
	for curr != "/" && curr != "." {
		info, err := os.Stat(filepath.Join(curr, ".alif"))
		if err == nil && info.IsDir() {
			alifDir = filepath.Join(curr, ".alif")
			break
		}
		curr = filepath.Dir(curr)
	}

	if alifDir == "" {
		return device, script
	}

	xmlPath := filepath.Join(alifDir, "JLinkDevices.xml")
	xmlContent, err := os.ReadFile(xmlPath)
	if err == nil {
		// Normalize target (AE722F80F55D5LS:M55_HE -> AE722F80F55D5LS_M55_HE)
		normTarget := strings.ReplaceAll(target, ":", "_")

		// Scan lines for Aliases containing normTarget
		scanner := bufio.NewScanner(bytes.NewReader(xmlContent))
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, normTarget) {
				// Try to extract Name
				if parts := strings.Split(line, "Name=\""); len(parts) > 1 {
					name := strings.Split(parts[1], "\"")[0]
					if name != "" {
						device = name
					}
				}
				// Try to extract Script File
				if parts := strings.Split(line, "JLinkScriptFile=\""); len(parts) > 1 {
					scriptName := strings.Split(parts[1], "\"")[0]
					if scriptName != "" {
						script = filepath.Join(alifDir, scriptName)
					}
				}
				break
			}
		}
	}

	return device, script
}

func (f *Flasher) resolveBinaryAddress(buildDir string) (string, error) {
	mapPath := filepath.Join(buildDir, "app-package-map.txt")
	// Fallback to toolkit build dir if not in project build dir
	if _, err := os.Stat(mapPath); err != nil {
		mapPath = filepath.Join(f.Cfg.AlifToolsPath, "build", "app-package-map.txt")
	}

	content, err := os.ReadFile(mapPath)
	if err != nil {
		return "", fmt.Errorf("could not find package map file (app-package-map.txt). Please ensure the project is built correctly")
	}

	scanner := bufio.NewScanner(bytes.NewReader(content))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "alif-img.bin") {
			fields := strings.Fields(line)
			if len(fields) > 1 {
				return fields[0], nil
			}
		}
	}
	return "", fmt.Errorf("failed to extract binary MRAM address from package map. Please check the content of %s", mapPath)
}

func (f *Flasher) resolveTOCAddress(buildDir string) (string, error) {
	mapPath := filepath.Join(buildDir, "app-package-map.txt")
	// Fallback to toolkit build dir if not in project build dir
	if _, err := os.Stat(mapPath); err != nil {
		mapPath = filepath.Join(f.Cfg.AlifToolsPath, "build", "app-package-map.txt")
	}

	mapContent, err := os.ReadFile(mapPath)
	if err != nil {
		return "", fmt.Errorf("could not find package map file (app-package-map.txt) in build directory. Please ensure the project is built correctly")
	}

	scanner := bufio.NewScanner(bytes.NewReader(mapContent))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "APP Package Start Address:") {
			parts := strings.Split(line, ":")
			if len(parts) > 1 {
				addr := strings.TrimSpace(parts[1])
				if addr != "" {
					return addr, nil
				}
			}
		}
	}

	return "", fmt.Errorf("failed to extract TOC address from package map. Please check the content of %s", mapPath)
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
