package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"alif-cli/internal/color"
	"alif-cli/internal/config"
	"alif-cli/internal/flasher"
	"alif-cli/internal/project"
	"alif-cli/internal/targets"

	"github.com/spf13/cobra"
)

// Variables for flags
var flashConfig string
var flashSlow bool
var flashMethod string
var flashVerbose bool
var flashNoErase bool

var flashCmd = &cobra.Command{
	Use:   "flash [solution_path or binary_file]",
	Short: "Flash a built project or a specific binary",
	Long: `Flashes the signed binary to the connected Alif board.

Modes:
1. Solution Mode (Default):
   alif flash [solution_path or solution_directory]
   Flashes the last built artifact from the solution found.

2. Binary Mode:
   alif flash myapp.bin [-c config.json]
   Signs and flashes the provided binary. 
   If -c is not provided, it attempts to valid config files in the directory.

Flags:
   --slow: Disable dynamic baud rate switching. Use this if you get "Target did not respond" errors.`,
	Run: func(cmd *cobra.Command, args []string) {
		path := ""
		if len(args) > 0 {
			path = args[0]
		}
		runFlash(path)
	},
}

func init() {
	flashCmd.Flags().StringVarP(&flashConfig, "config", "c", "", "Custom signing configuration file (JSON)")
	flashCmd.Flags().BoolVar(&flashSlow, "slow", false, "Disable dynamic baud rate switching (more stable)")
	flashCmd.Flags().StringVarP(&flashMethod, "method", "m", "ISP", "Loading method (ISP or JTAG)")
	flashCmd.Flags().BoolVarP(&flashVerbose, "verbose", "v", false, "Enable verbose output")
	flashCmd.Flags().BoolVar(&flashNoErase, "no-erase", false, "Skip automatic erase before flashing")
	rootCmd.AddCommand(flashCmd)
}

func runFlash(path string) {
	// 0. Determine Mode (Solution vs Binary)
	isBinary := false
	if path != "" {
		info, err := os.Stat(path)
		if err == nil && !info.IsDir() {
			isBinary = true
		}
	}

	var signedBinPath, tocPath, workingDir string
	var targetCore string

	if isBinary {
		// --- BINARY MODE ---
		binPath, _ := filepath.Abs(path)
		workingDir = filepath.Dir(binPath)

		cfg, _ := config.LoadConfig()
		if cfg == nil || cfg.AlifToolsPath == "" {
			color.Error("Error: Alif CLI not configured. Run 'alif setup' first.")
			os.Exit(1)
		}

		// Use simplified SETOOLS workflow
		color.Info("Binary Mode: Using simplified SETOOLS workflow")

		// 0. Retrieve configuration (Explicit or Auto-detected)
		// We pass flashConfig (could be empty) and workingDir to search in
		_, resolvedConfigPath, err := targets.ResolveTargetConfig(flashConfig, workingDir)
		if err != nil {
			color.Error("Configuration error: %v", err)
			os.Exit(1)
		}

		// 1. Copy binary to toolkit/build/images/ with original name
		configAbsPath, _ := filepath.Abs(resolvedConfigPath)
		binBaseName := filepath.Base(binPath)
		toolkitBinPath := filepath.Join(cfg.AlifToolsPath, "build", "images", binBaseName)

		os.MkdirAll(filepath.Dir(toolkitBinPath), 0755)
		color.Info("Copying binary to toolkit: %s", toolkitBinPath)

		srcFile, _ := os.Open(binPath)
		dstFile, _ := os.Create(toolkitBinPath)
		io.Copy(dstFile, srcFile)
		srcFile.Close()
		dstFile.Close()

		// 2. Run app-gen-toc from toolkit directory
		color.Info("Generating TOC with config: %s", configAbsPath)
		genTocCmd := exec.Command(filepath.Join(cfg.AlifToolsPath, "app-gen-toc"), "-f", configAbsPath)
		genTocCmd.Dir = cfg.AlifToolsPath
		genTocCmd.Stdout = os.Stdout
		genTocCmd.Stderr = os.Stderr
		if err := genTocCmd.Run(); err != nil {
			color.Error("app-gen-toc failed: %v", err)
			os.Exit(1)
		}

		// 3. Select port and update ISP config
		f := flasher.New(cfg)
		port, err := f.SelectPort()
		if err != nil {
			color.Error("Port selection failed: %v", err)
			os.Exit(1)
		}

		// Update ISP config file with selected port
		ispConfigPath := filepath.Join(cfg.AlifToolsPath, "isp_config_data.cfg")
		content, _ := os.ReadFile(ispConfigPath)
		lines := strings.Split(string(content), "\n")
		for i, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), "comport") {
				lines[i] = fmt.Sprintf("comport %s", port)
				break
			}
		}
		os.WriteFile(ispConfigPath, []byte(strings.Join(lines, "\n")), 0644)

		// 5. Run app-write-mram -p from toolkit directory
		color.Info("Flashing with app-write-mram...")
		flashCmd := exec.Command(filepath.Join(cfg.AlifToolsPath, "app-write-mram"), "-p")
		flashCmd.Dir = cfg.AlifToolsPath
		flashCmd.Stdout = os.Stdout
		flashCmd.Stderr = os.Stderr
		if err := flashCmd.Run(); err != nil {
			color.Error("Flashing failed: %v", err)
			os.Exit(1)
		}

		color.Success("Flashing complete!")
		return

	} else {
		// --- SOLUTION MODE ---
		solDir, err := project.IsSolutionRoot(path)
		if err != nil {
			color.Error("Error: %v", err)
			os.Exit(1)
		}

		stateFile := filepath.Join(solDir, ".alif_build_state")
		content, err := os.ReadFile(stateFile)
		if err != nil {
			color.Error("Error: No build state found in solution. Run 'alif build' first.")
			os.Exit(1)
		}
		lines := strings.Split(string(content), "\n")
		if len(lines) < 2 {
			color.Error("Error: Invalid build state.")
			os.Exit(1)
		}

		tocPath = lines[1]
		workingDir = filepath.Dir(tocPath)
		signedBinPath = filepath.Join(workingDir, "alif-img.bin")

		if len(lines) >= 3 {
			targetCore = lines[2]
		}

		// Auto-detect local config in .alif/ folder if not explicitly provided
		if flashConfig == "" && targetCore != "" {
			cfgName := fmt.Sprintf("%s_cfg.json", project.GetCoreName(targetCore))
			localCfg := filepath.Join(solDir, ".alif", cfgName)
			if _, err := os.Stat(localCfg); err == nil {
				flashConfig = localCfg
				color.Info("Detected local signing config: %s", localCfg)
			}
		}
	}

	// 3. Flasher Setup
	cfg, _ := config.LoadConfig()
	f := flasher.New(cfg)

	// 4. Port Selection
	port, err := f.SelectPort()
	if err != nil {
		color.Error("Error identifying port: %v", err)
		os.Exit(1)
	}

	// 5. Flash
	color.Info("Flashing artifacts from %s...", workingDir)
	// targetCore might be empty or default, flashConfig is passed
	if err := f.Flash(signedBinPath, tocPath, port, targetCore, flashConfig, flashSlow, flashMethod, flashVerbose, flashNoErase); err != nil {
		color.Error("Flash failed: %v", err)
		os.Exit(1)
	}
	color.Success("Flashing complete!")
}
