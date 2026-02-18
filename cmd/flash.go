package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"alif-cli/internal/builder"
	"alif-cli/internal/config"
	"alif-cli/internal/flasher"
	"alif-cli/internal/project"
	"alif-cli/internal/signer"
	"alif-cli/internal/targets"
	"alif-cli/internal/ui"

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
var flashNoVerify bool

var flashCmd = &cobra.Command{
	Use:   "flash [binary_file]",
	Short: "Flash a built project or a specific binary",
	Long:  `Flashes the signed binary to the connected Alif board.`,
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
	flashCmd.Flags().BoolVar(&flashNoVerify, "no-verify", false, "Skip checking the connected hardware device")
	flashCmd.Flags().BoolVar(&flashNoVerify, "nv", false, "Skip checking the connected hardware device (alias for --no-verify)")
	rootCmd.AddCommand(flashCmd)
}

func runFlash(path string) {
	// 0. Determine Mode
	isBinary := false
	if path != "" {
		info, err := os.Stat(path)
		if err != nil {
			ui.Error(fmt.Sprintf("%v", err))
			os.Exit(1)
		}
		if info.IsDir() {
			ui.Error("Directory argument not supported. Use -p to select project, or provide path to a binary file.")
			os.Exit(1)
		}
		isBinary = true
	}

	var signedBinPath, tocPath, workingDir string
	var targetCore string

	cfg, _ := config.LoadConfig()
	if cfg == nil || cfg.AlifToolsPath == "" {
		ui.Error("Alif CLI not configured. Run 'alif setup' first.")
		os.Exit(1)
	}

	if isBinary {
		// --- BINARY MODE ---
		ui.Header("Binary Mode Setup")
		binPath, _ := filepath.Abs(path)
		workingDir = filepath.Dir(binPath)

		// 0. Retrieve configuration
		resolvedConfig, resolvedConfigPath, err := targets.ResolveTargetConfig(flashConfig, workingDir, "", "")
		if err != nil {
			ui.Error(fmt.Sprintf("Configuration error: %v", err))
			os.Exit(1)
		}

		// --- Hardware Pre-Verification ---
		f := flasher.New(cfg)

		ui.Header("Flash Target")
		port, err := f.SelectPort()
		if err != nil {
			ui.Error(fmt.Sprintf("Error identifying port: %v", err))
			os.Exit(1)
		}

		// Update ISP Config so verification tools use the correct port
		if flashMethod == "ISP" {
			if err := f.UpdateISPConfig(port); err != nil {
				ui.Warn(fmt.Sprintf("Failed to update ISP config: %v", err))
			}
		}

		// 1. Copy binary to toolkit
		configAbsPath, _ := filepath.Abs(resolvedConfigPath)
		binBaseName := filepath.Base(binPath)
		toolkitBinPath := filepath.Join(cfg.AlifToolsPath, "build", "images", binBaseName)

		os.MkdirAll(filepath.Dir(toolkitBinPath), 0755)

		srcFile, _ := os.Open(binPath)
		dstFile, _ := os.Create(toolkitBinPath)
		io.Copy(dstFile, srcFile)
		srcFile.Close()
		dstFile.Close()

		// 2. Sync Toolkit Config
		if err = targets.SyncToolkitConfig(cfg.AlifToolsPath, resolvedConfig.GetCPU()); err != nil {
			ui.Warn(fmt.Sprintf("Toolkit sync failed: %v", err))
		}

		// Perform live verification
		if flashMethod == "ISP" && !flashNoVerify {
			if err := targets.VerifyConnectedDevice(cfg.AlifToolsPath, resolvedConfig.GetCPU()); err != nil {
				// VerifyConnectedDevice prints its own failure
				os.Exit(1)
			}
		}

		// 3. Run app-gen-toc
		cmd := exec.Command(filepath.Join(cfg.AlifToolsPath, "app-gen-toc"), "-f", configAbsPath)
		cmd.Dir = cfg.AlifToolsPath
		var output bytes.Buffer
		cmd.Stdout = &output
		cmd.Stderr = &output

		sp := ui.StartSpinner("Generating TOC...")
		if err := cmd.Run(); err != nil {
			sp.Fail("TOC generation failed")
			fmt.Println("\n" + output.String())
			os.Exit(1)
		}
		sp.Succeed("TOC generated successfully")

		// 4. Flash
		cmdFlash := exec.Command(filepath.Join(cfg.AlifToolsPath, "app-write-mram"), "-p")
		cmdFlash.Dir = cfg.AlifToolsPath
		var outFlash bytes.Buffer
		cmdFlash.Stdout = &outFlash
		cmdFlash.Stderr = &outFlash

		spFlash := ui.StartSpinner("Flashing binary...")
		if err := cmdFlash.Run(); err != nil {
			spFlash.Fail("Flash failed")
			fmt.Println("\n" + outFlash.String())
			os.Exit(1)
		}
		spFlash.Succeed("Flash complete!")
		return

	} else {
		// --- PROJECT MODE (Solution/Context) ---
		// Find Solution Root (Scanning silently)
		cwd, _ := os.Getwd()
		solDir, err := project.IsSolutionRoot(cwd)
		if err != nil {
			ui.Error("Could not find solution (.csolution.yml) in current directory.")
			os.Exit(1)
		}

		// Resolve Context
		b := builder.New(cfg)
		selectedContext, err := b.ResolveContext(solDir, "", flashProject)
		if err != nil {
			ui.Error(fmt.Sprintf("%v", err))
			os.Exit(1)
		}

		// Find corresponding .cbuild.yml file recursively
		targetFile := selectedContext + ".cbuild.yml"
		var selectedFile string

		filepath.Walk(solDir, func(path string, info os.FileInfo, err error) error {
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
			ui.Error(fmt.Sprintf("Build configuration file '%s' not found.", targetFile))
			os.Exit(1)
		}

		ui.Item("Config", filepath.Base(selectedFile))

		// Parse YAML
		v := viper.New()
		v.SetConfigFile(selectedFile)
		if err := v.ReadInConfig(); err != nil {
			ui.Error(fmt.Sprintf("Error reading build config: %v", err))
			os.Exit(1)
		}

		// Parse Hints (Device Core and Project Name)
		deviceStr := v.GetString("build.device") // e.g. "Alif Semiconductor::AE722F80F55D5LS:M55_HE"
		var coreHint string
		if parts := strings.Split(deviceStr, ":"); len(parts) > 0 {
			coreHint = parts[len(parts)-1]
		}

		// Extract Full Part Number for dynamic resolution
		targetCore = coreHint
		deviceParts := strings.Split(deviceStr, "::")
		if len(deviceParts) > 1 {
			targetCore = deviceParts[1] // e.g. AE722F80F55D5LS:M55_HE
		}

		projectHint := flashProject
		if projectHint == "" {
			if idx := strings.Index(selectedContext, "."); idx != -1 {
				projectHint = selectedContext[:idx]
			} else {
				projectHint = selectedContext
			}
		}

		// Extract Output Directory
		outRel := v.GetString("build.output-dirs.outdir")
		binName := ""
		outputs := v.Get("build.output").([]interface{})
		for _, o := range outputs {
			omap := o.(map[string]interface{})
			if omap["type"] == "bin" {
				binName = omap["file"].(string)
				break
			}
		}

		// Construct paths
		cbuildDir := filepath.Dir(selectedFile)
		binDir := filepath.Join(cbuildDir, outRel)
		binPath := filepath.Join(binDir, binName)

		// Derived Artifact Paths
		signedBinPath = filepath.Join(binDir, "alif-img.bin")
		tocPath = filepath.Join(binDir, "AppTocPackage.bin")
		workingDir = binDir

		// --- Hardware Pre-Verification ---
		f := flasher.New(cfg)

		ui.Header("Flash Target")
		port, err := f.SelectPort()
		if err != nil {
			ui.Error(fmt.Sprintf("Error identifying port: %v", err))
			os.Exit(1)
		}

		// Update ISP Config so verification tools use the correct port
		if flashMethod == "ISP" {
			if err := f.UpdateISPConfig(port); err != nil {
				ui.Warn(fmt.Sprintf("Failed to update ISP config: %v", err))
			}
		}

		// 2. Sync Toolkit Config
		if err := targets.SyncToolkitConfig(cfg.AlifToolsPath, targetCore); err != nil {
			ui.Warn(fmt.Sprintf("Toolkit sync failed: %v", err))
		}

		// Perform live verification (User wants this after toolkit sync logs)
		if flashMethod == "ISP" && !flashNoVerify {
			if err := targets.VerifyConnectedDevice(cfg.AlifToolsPath, targetCore); err != nil {
				// VerifyConnectedDevice prints its own failure
				os.Exit(1)
			}
		}

		// 3. Create Image (Pack/Sign) with Hints. We always run this to ensure
		// all artifacts and side-effects (like .ds script updates) are applied.
		s := signer.New(cfg)
		_, err = s.SignArtifact(solDir, binDir, binPath, coreHint, projectHint, flashConfig)
		if err != nil {
			ui.Error(fmt.Sprintf("Failed to create bootable image: %v", err))
			os.Exit(1)
		}

		// 4. Flash
		if err := f.Flash(signedBinPath, tocPath, port, targetCore, flashConfig, flashSlow, flashMethod, flashVerbose, flashNoErase); err != nil {
			ui.Error(fmt.Sprintf("Flash failed: %v", err))
			os.Exit(1)
		}
		return
	}
}
