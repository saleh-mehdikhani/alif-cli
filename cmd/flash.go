package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"alif-cli/internal/builder"
	"alif-cli/internal/color"
	"alif-cli/internal/config"
	"alif-cli/internal/flasher"
	"alif-cli/internal/project"
	"alif-cli/internal/signer"
	"alif-cli/internal/targets"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Variables for flags
var flashConfig string
var flashSlow bool
var flashMethod string
var flashVerbose bool
var flashNoErase bool
var flashProject string

var flashCmd = &cobra.Command{
	Use:   "flash [binary_file]",
	Short: "Flash a built project or a specific binary",
	Long: `Flashes the signed binary to the connected Alif board.

Modes:
1. Project Mode (Default):
   alif flash [-p project_name]
   Uses the same logic as 'alif build' to resolve the project context.
   Finds the corresponding build configuration and flashes the artifact.
   Automatically creates/updates the bootable image (alif-img.bin) if needed.
   
2. Binary Mode:
   alif flash myapp.bin [-c config.json]
   Creates bootable image (if needed) and flashes the provided binary file.

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
	flashCmd.Flags().StringVarP(&flashProject, "project", "p", "", "Project name or context filter")
	rootCmd.AddCommand(flashCmd)
}

func runFlash(path string) {
	// 0. Determine Mode
	isBinary := false
	if path != "" {
		info, err := os.Stat(path)
		if err != nil {
			color.Error("Error: %v", err)
			os.Exit(1)
		}
		if info.IsDir() {
			color.Error("Error: Directory argument not supported. Use -p to select project, or provide path to a binary file.")
			os.Exit(1)
		}
		isBinary = true
	}

	var signedBinPath, tocPath, workingDir string
	var targetCore string

	cfg, _ := config.LoadConfig()
	if cfg == nil || cfg.AlifToolsPath == "" {
		color.Error("Error: Alif CLI not configured. Run 'alif setup' first.")
		os.Exit(1)
	}

	if isBinary {
		// --- BINARY MODE ---
		binPath, _ := filepath.Abs(path)
		workingDir = filepath.Dir(binPath)

		color.Info("Binary Mode: Using simplified SETOOLS workflow")

		// 0. Retrieve configuration (Explicit or Auto-detected)
		// No hints available in binary mode
		_, resolvedConfigPath, err := targets.ResolveTargetConfig(flashConfig, workingDir, "", "")
		if err != nil {
			color.Error("Configuration error: %v", err)
			os.Exit(1)
		}

		// 1. Copy binary to toolkit
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

		// 2. Run app-gen-toc
		color.Info("Generating TOC with config: %s", configAbsPath)
		genTocCmd := exec.Command(filepath.Join(cfg.AlifToolsPath, "app-gen-toc"), "-f", configAbsPath)
		genTocCmd.Dir = cfg.AlifToolsPath
		genTocCmd.Stdout = os.Stdout
		genTocCmd.Stderr = os.Stderr
		if err := genTocCmd.Run(); err != nil {
			color.Error("app-gen-toc failed: %v", err)
			os.Exit(1)
		}

		// 3. Select port
		f := flasher.New(cfg)
		port, err := f.SelectPort()
		if err != nil {
			color.Error("Port selection failed: %v", err)
			os.Exit(1)
		}

		// Update ISP com port
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

		// 5. Run app-write-mram
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
		// --- PROJECT MODE (Solution/Context) ---
		// Find Solution Root
		cwd, _ := os.Getwd()
		solDir, err := project.IsSolutionRoot(cwd)
		if err != nil {
			color.Error("Error: Could not find solution (.csolution.yml) in current directory or parents.")
			os.Exit(1)
		}

		// Resolve Context using Builder logic
		b := builder.New(cfg)
		selectedContext, err := b.ResolveContext(solDir, "", flashProject)
		if err != nil {
			color.Error("Error resolving context: %v", err)
			os.Exit(1)
		}

		color.Info("Selected context: %s", selectedContext)

		// Find corresponding .cbuild.yml file recursively
		targetFile := selectedContext + ".cbuild.yml"
		var selectedFile string

		err = filepath.Walk(solDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() {
				name := info.Name()
				if name == ".git" || name == "packs" || name == "tools" || name == "node_modules" || name == "out" || name == "tmp" {
					return filepath.SkipDir
				}
				return nil
			}
			if info.Name() == targetFile {
				selectedFile = path
				return errors.New("found")
			}
			return nil
		})

		if selectedFile == "" {
			color.Error("Error: Build configuration file '%s' not found.", targetFile)
			color.Error("Ensure you have built the project successfully.")
			os.Exit(1)
		}

		color.Info("Reading build configuration: %s", filepath.Base(selectedFile))

		// Parse YAML
		v := viper.New()
		v.SetConfigFile(selectedFile)
		if err := v.ReadInConfig(); err != nil {
			color.Error("Error reading build config: %v", err)
			os.Exit(1)
		}

		// Parse Hints (Device Core and Project Name)
		deviceStr := v.GetString("build.device") // e.g. "Alif Semiconductor::AE722F80F55D5LS:M55_HE"
		var coreHint string
		if parts := strings.Split(deviceStr, ":"); len(parts) > 0 {
			coreHint = parts[len(parts)-1]
		}

		projectHint := flashProject
		if projectHint == "" {
			// Extract from context: blinky.debug... -> blinky
			if idx := strings.Index(selectedContext, "."); idx != -1 {
				projectHint = selectedContext[:idx]
			} else {
				projectHint = selectedContext
			}
		}

		// Extract Output Directory
		outRel := v.GetString("build.output-dirs.outdir")
		if outRel == "" {
			color.Error("Error: 'output-dirs.outdir' missing in build config.")
			os.Exit(1)
		}

		// Find Binary Name
		binName := ""
		outputs := v.Get("build.output").([]interface{})
		for _, o := range outputs {
			omap := o.(map[string]interface{})
			if omap["type"] == "bin" {
				binName = omap["file"].(string)
				break
			}
		}
		if binName == "" {
			color.Error("Error: 'bin' output not found in build config.")
			os.Exit(1)
		}

		// Construct paths
		cbuildDir := filepath.Dir(selectedFile)
		binDir := filepath.Join(cbuildDir, outRel)
		binPath := filepath.Join(binDir, binName)

		// Derived Artifact Paths
		signedBinPath = filepath.Join(binDir, "alif-img.bin")
		tocPath = filepath.Join(binDir, "AppTocPackage.bin")
		workingDir = binDir

		// Parse Target Core from filename (Use as fallback or just use coreHint which is better)
		// We use coreHint (from YAML) for resolving config, which is more accurate.
		targetCore = coreHint

		// Auto-detect config for signing/flashing using Hints
		_, resolvedConfigPath, err := targets.ResolveTargetConfig(flashConfig, solDir, coreHint, projectHint)
		if err != nil {
			color.Error("Configuration error: %v", err)
			os.Exit(1)
		}
		flashConfig = resolvedConfigPath

		// Check and Create Image if needed
		needImage := false
		if _, err := os.Stat(tocPath); os.IsNotExist(err) {
			needImage = true
			color.Info("Bootable image (AppTocPackage.bin) missing. Creating...")
		} else {
			// Check timestamps
			binInfo, err1 := os.Stat(binPath)
			tocInfo, err2 := os.Stat(tocPath)
			if err1 == nil && err2 == nil {
				if binInfo.ModTime().After(tocInfo.ModTime()) {
					needImage = true
					color.Info("Binary is newer than image. Re-creating image...")
				}
			}
		}

		if needImage {
			s := signer.New(cfg)
			// Create Image (Pack/Sign) with Hints
			_, err = s.SignArtifact(solDir, binDir, binPath, coreHint, projectHint, flashConfig)
			if err != nil {
				color.Error("Failed to create bootable image: %v", err)
				os.Exit(1)
			}
			color.Success("Bootable image created successfully.")
		}
	}

	// 3. Flasher Setup
	f := flasher.New(cfg)

	// 4. Port Selection
	port, err := f.SelectPort()
	if err != nil {
		color.Error("Error identifying port: %v", err)
		os.Exit(1)
	}

	// 5. Flash
	color.Info("Flashing artifacts from %s...", workingDir)
	if err := f.Flash(signedBinPath, tocPath, port, targetCore, flashConfig, flashSlow, flashMethod, flashVerbose, flashNoErase); err != nil {
		color.Error("Flash failed: %v", err)
		os.Exit(1)
	}
	color.Success("Flashing complete!")
}
