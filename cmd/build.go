package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"alif-cli/internal/builder"
	"alif-cli/internal/color"
	"alif-cli/internal/config"
	"alif-cli/internal/project"
	"alif-cli/internal/signer"
	"alif-cli/internal/ui"

	"github.com/spf13/cobra"
)

// Variables for flags
var buildProject string
var buildSign bool
var buildClean bool

var buildCmd = &cobra.Command{
	Use:   "build [solution_path]",
	Short: "Build the project within a solution",
	Long: `Compiles the source code using cbuild.
	
If solution_path is not specified, uses the current directory.
The --project (-p) flag filters the build context.
The --clean flag forces a rebuild (clean then build).
  
By default, this command compiles the code. To create a bootable image immediately, use the --sign (-s) flag.`,
	Run: func(cmd *cobra.Command, args []string) {
		solutionPath := ""
		if len(args) > 0 {
			solutionPath = args[0]
		}
		runBuild(solutionPath)
	},
}

func init() {
	buildCmd.Flags().StringVarP(&buildProject, "project", "p", "", "Project name or context filter (e.g. 'blinky' or 'blinky.debug')")
	buildCmd.Flags().BoolVarP(&buildSign, "sign", "s", false, "Create bootable image (package/sign) after building")
	buildCmd.Flags().BoolVar(&buildClean, "clean", false, "Clean artifacts and rebuild (full rebuild)")
	rootCmd.AddCommand(buildCmd)
}

func runBuild(solutionPath string) {
	start := time.Now()

	// 1. Validate Solution
	solDir, err := project.IsSolutionRoot(solutionPath)
	if err != nil {
		ui.Error(fmt.Sprintf("%v", err))
		os.Exit(1)
	}

	cfg, _ := config.LoadConfig()
	if cfg == nil || cfg.AlifToolsPath == "" {
		ui.Error("Alif CLI not configured. Run 'alif setup' first.")
		os.Exit(1)
	}

	// 2. Build
	b := builder.New(cfg)
	// Pass clean flag to trigger --rebuild if requested
	selectedContext, err := b.Build(solDir, "", buildProject, buildClean)
	if err != nil {
		ui.Error("Build process failed.")
		os.Exit(1)
	}

	// Determine Artifact Path (Only if single context selected)
	if selectedContext == "" {
		ui.Header("Process Complete")
		ui.Item("Duration", time.Since(start).Round(time.Millisecond).String())
		ui.Success("Clean & Rebuild of all contexts completed successfully.")
		return
	}

	binPath := b.GetArtifactPath(solDir, selectedContext)

	if !buildSign {
		// Summary
		ui.Header("Build Summary")
		ui.Item("Context", selectedContext)
		ui.Item("Artifact", binPath)
		ui.Item("Duration", time.Since(start).Round(time.Millisecond).String())

		fmt.Println()
		ui.Info(fmt.Sprintf("To flash this project, run: %s", color.Sprintf(color.BoldCyan, "alif flash -p %s", selectedContext)))
		return
	}

	// 3. Resolve Artifacts for Signing (if path missing, try fallback)
	if _, err := os.Stat(binPath); os.IsNotExist(err) {
		ui.Info("Scanning out/ directory for most recent binary...")
		binPath = findRecentBin(solDir)
	}

	if binPath == "" {
		ui.Error("Could not locate built binary.")
		os.Exit(1)
	}

	// 4. Sign (Create Image)
	targetCore := ""
	if parts := strings.Split(selectedContext, "+"); len(parts) > 1 {
		targetCore = parts[1]
	}

	signBuildDir := filepath.Dir(binPath)
	s := signer.New(cfg)
	_, errSign := s.SignArtifact(solDir, signBuildDir, binPath, targetCore, buildProject, "")
	if errSign != nil {
		ui.Error(fmt.Sprintf("Image creation failed: %v", errSign))
		os.Exit(1)
	}

	// Final Summary
	ui.Header("Process Complete")
	ui.Item("Context", selectedContext)
	ui.Item("Image", filepath.Join(signBuildDir, "AppTocPackage.bin"))
	ui.Item("Duration", time.Since(start).Round(time.Millisecond).String())
	ui.Success("Build and packaging completed successfully.")
}

func findRecentBin(root string) string {
	var recent string
	var recentTime int64
	filepath.Walk(filepath.Join(root, "out"), func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(info.Name(), ".bin") {
			if !strings.Contains(info.Name(), "alif-img") && !strings.Contains(info.Name(), "AppTocPackage") {
				if info.ModTime().Unix() > recentTime {
					recentTime = info.ModTime().Unix()
					recent = path
				}
			}
		}
		return nil
	})
	return recent
}
