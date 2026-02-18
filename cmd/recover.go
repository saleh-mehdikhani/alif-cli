package cmd

import (
	"bufio"
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"alif-cli/internal/config"
	"alif-cli/internal/ui"

	"github.com/spf13/cobra"
)

var recoverDevice string

// XML structures for parsing JLinkDevices.xml
type JLinkDataBase struct {
	XMLName xml.Name      `xml:"DataBase"`
	Devices []JLinkDevice `xml:"Device"`
}

type JLinkDevice struct {
	ChipInfo JLinkChipInfo `xml:"ChipInfo"`
}

type JLinkChipInfo struct {
	Name    string `xml:"Name,attr"`
	Aliases string `xml:"Aliases,attr"`
}

var recoverCmd = &cobra.Command{
	Use:   "recover",
	Short: "Recover a locked or unresponsive device via J-Link JTAG",
	Long: `Clears the boot signatures in MRAM using a J-Link debugger. 
This forces the ROM bootloader back into ISP mode by zeroing out the TOC and Application entry points.`,
	Run: func(cmd *cobra.Command, args []string) {
		runEmergencyRecover()
	},
}

func init() {
	recoverCmd.Flags().StringVarP(&recoverDevice, "device", "d", "", "Target J-Link device name (e.g. AE722F80F55D5LS_M55_HE)")
	rootCmd.AddCommand(recoverCmd)
}

func findJLinkDevicesXML(cfg *config.Config) string {
	cwd, _ := os.Getwd()
	// 1. Check current and parents for .alif/JLinkDevices.xml or JLinkDevices.xml
	curr := cwd
	for {
		paths := []string{
			filepath.Join(curr, ".alif", "JLinkDevices.xml"),
			filepath.Join(curr, "JLinkDevices.xml"),
		}
		for _, p := range paths {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
		parent := filepath.Dir(curr)
		if parent == curr || parent == "/" {
			break
		}
		curr = parent
	}

	// 2. Check the Toolkit directory (from config)
	if cfg != nil && cfg.AlifToolsPath != "" {
		p := filepath.Join(cfg.AlifToolsPath, "JLinkDevices.xml")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	// 3. Fallback to Home config dir
	home, _ := os.UserHomeDir()
	if home != "" {
		p := filepath.Join(home, ".alif", "JLinkDevices.xml")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	return ""
}

func extractCandidateAddresses(cfg *config.Config) []string {
	addrs := map[string]bool{
		"0x80000000": true, // Base Application MRAM
		"0x80010000": true, // Secondary application offset
		"0x8057f0e0": true, // Common E7 Package Start
		"0x8057f0f0": true, // Alif specific header address
		"0x8057ff90": true, // Common E7 TOC Start
		"0x8057bff0": true, // Address from application_package.ds
	}

	// 1. Extract from Toolkit's application_package.ds
	dsPath := filepath.Join(cfg.AlifToolsPath, "bin", "application_package.ds")
	if content, err := os.ReadFile(dsPath); err == nil {
		lines := strings.Split(string(content), "\n")
		for _, line := range lines {
			if strings.Contains(line, "semihosting args") {
				fields := strings.Fields(line)
				for _, f := range fields {
					if strings.HasPrefix(f, "0x") {
						addrs[f] = true
					}
				}
			}
		}
	}

	// 2. Extract from ALL local app-package-map.txt files found
	cwd, _ := os.Getwd()
	filepath.Walk(cwd, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.Name() == "app-package-map.txt" {
			if content, err := os.ReadFile(path); err == nil {
				lines := strings.Split(string(content), "\n")
				for _, l := range lines {
					if strings.Contains(l, "Address:") {
						parts := strings.Split(l, ":")
						if len(parts) > 1 {
							addr := strings.TrimSpace(parts[1])
							if strings.HasPrefix(addr, "0x") {
								fields := strings.Fields(addr)
								if len(fields) > 0 && strings.HasPrefix(fields[0], "0x") {
									addrs[fields[0]] = true
								}
							}
						}
					}
				}
			}
		}
		return nil
	})

	var result []string
	for a := range addrs {
		result = append(result, a)
	}
	return result
}

func runEmergencyRecover() {
	cfg, err := config.LoadConfig()
	if err != nil || cfg.AlifToolsPath == "" {
		ui.Error("Alif CLI not configured or config missing. Run 'alif setup' first.")
		os.Exit(1)
	}

	ui.Header("Hardware Recovery")

	// 0. Resolve Device if not provided
	if recoverDevice == "" {
		xmlPath := findJLinkDevicesXML(cfg)
		if xmlPath == "" {
			ui.Warn("No JLinkDevices.xml found. Please provide device name with -d flag.")
			fmt.Print("Enter J-Link Device Name: ")
			reader := bufio.NewReader(os.Stdin)
			input, _ := reader.ReadString('\n')
			recoverDevice = strings.TrimSpace(input)
			if recoverDevice == "" {
				ui.Error("Device name is required.")
				os.Exit(1)
			}
		} else {
			ui.Item("Device DB", filepath.Base(xmlPath))
			data, err := os.ReadFile(xmlPath)
			if err != nil {
				ui.Error(fmt.Sprintf("Failed to read device database: %v", err))
				os.Exit(1)
			}

			var db JLinkDataBase
			if err := xml.Unmarshal(data, &db); err != nil {
				ui.Error(fmt.Sprintf("Failed to parse device database: %v", err))
				os.Exit(1)
			}

			var options []string
			for _, d := range db.Devices {
				if d.ChipInfo.Name != "" {
					options = append(options, d.ChipInfo.Name)
				}
				if d.ChipInfo.Aliases != "" {
					aliases := strings.Split(d.ChipInfo.Aliases, ";")
					for _, a := range aliases {
						trimmed := strings.TrimSpace(a)
						if trimmed != "" {
							options = append(options, trimmed)
						}
					}
				}
			}

			if len(options) == 0 {
				ui.Error("No devices found in database.")
				os.Exit(1)
			}

			// Filter for Alif-like names to reduce noise if needed, but here we just show all from the Alif-provided XML
			fmt.Println("Select Target Device:")
			for i, op := range options {
				fmt.Printf("[%d] %s\n", i+1, op)
			}
			fmt.Print("Select number: ")
			reader := bufio.NewReader(os.Stdin)
			input, _ := reader.ReadString('\n')
			selection, err := strconv.Atoi(strings.TrimSpace(input))
			if err != nil || selection < 1 || selection > len(options) {
				ui.Error("Invalid selection.")
				os.Exit(1)
			}
			recoverDevice = options[selection-1]
		}
	}

	ui.Item("Selected", recoverDevice)

	// 1. Resolve Candidate Addresses
	candidateAddrs := extractCandidateAddresses(cfg)

	// 2. Create J-Link command file on the fly
	commands := []string{
		"si 1",       // SWD Mode
		"speed 2000", // Slower speed for more stability
		"connect",
		"halt",
	}

	for _, addr := range candidateAddrs {
		ui.Item("Targeting", addr)
		// Write 64 bytes of zeros (16 x 32-bit words) to kill multiple possible headers
		for i := 0; i < 16; i++ {
			if val, err := strconv.ParseUint(strings.TrimPrefix(addr, "0x"), 16, 64); err == nil {
				targetAddr := fmt.Sprintf("0x%x", val+uint64(i*4))
				commands = append(commands, fmt.Sprintf("w4 %s 0x00000000", targetAddr))
			}
		}
	}
	commands = append(commands, "reset", "q")

	jlinkFile := filepath.Join(os.TempDir(), "alif_recover.jlink")
	if err := os.WriteFile(jlinkFile, []byte(strings.Join(commands, "\n")+"\n"), 0644); err != nil {
		ui.Error(fmt.Sprintf("Failed to create recovery script: %v", err))
		os.Exit(1)
	}
	defer os.Remove(jlinkFile)

	// 2. Prepare J-Link command arguments
	args := []string{
		"-Device", recoverDevice,
		"-If", "JTAG",
		"-Speed", "4000",
		"-AutoConnect", "1",
		"-CommandFile", jlinkFile,
	}

	// Try to find a J-Link reset script in the current directory .alif folder
	cwd, _ := os.Getwd()
	localScript := filepath.Join(cwd, ".alif", "E7_Series_Reset.jlinkscript")
	if _, err := os.Stat(localScript); err == nil {
		args = append([]string{"-JLinkScriptFile", localScript}, args...)
	}

	// 3. Run JLinkExe
	jlinkExec := "JLinkExe"
	if runtime.GOOS == "windows" {
		jlinkExec = "JLink.exe"
	}

	cmd := exec.Command(jlinkExec, args...)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	sp := ui.StartSpinner(fmt.Sprintf("Recovering device %s via J-Link...", recoverDevice))
	err = cmd.Run()
	outStr := output.String()

	if err != nil || !strings.Contains(outStr, "Connected successfully") {
		sp.Fail("Recovery failed")
		fmt.Println("\n" + outStr)
		ui.Warn("Check J-Link connection, power, and target device name.")
		ui.Info("Suggestion: Put the board in ISP mode manually (Reset button while holding ISP button) if JTAG fails.")
		os.Exit(1)
	}

	sp.Succeed("Boot signatures cleared successfully.")
	ui.Info("Please Power Cycle the board to enter ISP mode.")
}
