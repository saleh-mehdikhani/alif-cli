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

var buildCmd = &cobra.Command{
	Use:   "build [solution_path]",
	Short: "Build the project within a solution",
	Long: `Compiles the source code using cbuild.
	
If solution_path is not specified, uses the current directory.
The --project flag (-p) filters the build context. You can specify a partial or full context string:
  Format: <project>.<build-type>+<target>
  Example: alif build -p blinky.debug+E7-HE
  
By default, this command compiles the code. To create a bootable image (package/sign) immediately, use the --sign (-s) flag.`,
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
	// builder.Build handles its own UI (Resolve Context, Compile)
	b := builder.New(cfg)
	selectedContext, err := b.Build(solDir, "", buildProject)
	if err != nil {
		// Builder prints error details via ui/fmt
		ui.Error("Build process failed.")
		os.Exit(1)
	}

	// Determine Artifact Path
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
		// ui.Warn(fmt.Sprintf("Binary not found at expected path: %s", binPath))
		ui.Info("Scanning out/ directory for most recent binary...")
		binPath = findRecentBin(solDir)
	}

	if binPath == "" {
		ui.Error("Could not locate built binary.")
		os.Exit(1)
	}

	// 4. Sign (Create Image)
	// targetCore parsing
	targetCore := ""
	if parts := strings.Split(selectedContext, "+"); len(parts) > 1 {
		targetCore = parts[1]
	}

	signBuildDir := filepath.Dir(binPath)
	s := signer.New(cfg)
	// signer.SignArtifact handles its own UI
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

// Add Succeed wrapper to ui if missing or use Info
// Checking ui package: Succeed exists as Spinner method but checking package level...
// I added Succeed method to Spinner, but not package level func.
// I'll check ui.go again.
// ui.go has Info, Warn, Error. No Succeed at package level.
// I'll add ui.Success wrap or just use Info with green checkmark manually if needed.
// But wait, user requested "result of current step".
// I'll check ui.go again to see if I can add Success function.
