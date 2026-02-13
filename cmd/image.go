package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"alif-cli/internal/config"
	"alif-cli/internal/signer"
	"alif-cli/internal/ui"

	"github.com/spf13/cobra"
)

var imageConfig string

var imageCmd = &cobra.Command{
	Use:   "image <binary_file>",
	Short: "Create a bootable firmware image (package/sign)",
	Long: `Packages a raw binary into a bootable image (alif-img.bin) and generates the TOC (AppTocPackage.bin).
This step is required for the device to boot the application.
Use -c to specify a configuration file, or let the tool auto-detect one.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runImage(args[0])
	},
}

func init() {
	imageCmd.Flags().StringVarP(&imageConfig, "config", "c", "", "Configuration file (JSON)")
	rootCmd.AddCommand(imageCmd)
}

func runImage(binPath string) {
	// 1. Validate Input
	absBinPath, err := filepath.Abs(binPath)
	if err != nil {
		ui.Error(fmt.Sprintf("Error resolving binary path: %v", err))
		os.Exit(1)
	}
	if _, err := os.Stat(absBinPath); os.IsNotExist(err) {
		ui.Error(fmt.Sprintf("Binary file not found: %s", absBinPath))
		os.Exit(1)
	}

	cfg, _ := config.LoadConfig()
	if cfg == nil || cfg.AlifToolsPath == "" {
		ui.Error("Alif CLI not configured. Run 'alif setup' first.")
		os.Exit(1)
	}

	workDir := filepath.Dir(absBinPath)

	// signer.SignArtifact prints its own UI Header ("Create Bootable Image")
	s := signer.New(cfg)
	// targetCore is unused in SignArtifact/ResolveTargetConfig if explicit config passed
	tocPath, err := s.SignArtifact(workDir, workDir, absBinPath, "", "", imageConfig)
	if err != nil {
		ui.Error(fmt.Sprintf("Failed to create image: %v", err))
		os.Exit(1)
	}

	ui.Success(fmt.Sprintf("Image created successfully: %s", filepath.Base(tocPath)))
}
