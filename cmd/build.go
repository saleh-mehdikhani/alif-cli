package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"alif-cli/internal/builder"
	"alif-cli/internal/color"
	"alif-cli/internal/config"
	"alif-cli/internal/project"
	"alif-cli/internal/signer"

	"github.com/spf13/cobra"
)

// Variables for flags
var buildProject string

var buildCmd = &cobra.Command{
	Use:   "build [solution_path]",
	Short: "Build and sign the project within a solution",
	Long: `Compiles the source code using cbuild and signs artifacts using Alif security tools.
	
If solution_path is not specified, uses the current directory.`,
	Run: func(cmd *cobra.Command, args []string) {
		solutionPath := ""
		if len(args) > 0 {
			solutionPath = args[0]
		}
		runBuild(solutionPath)
	},
}

func init() {
	buildCmd.Flags().StringVarP(&buildProject, "project", "p", "", "Specific project name to build (optional)")
	rootCmd.AddCommand(buildCmd)
}

func runBuild(solutionPath string) {
	// 1. Validate Solution
	solDir, err := project.IsSolutionRoot(solutionPath)
	if err != nil {
		color.Error("Error: %v", err)
		os.Exit(1)
	}

	cfg, _ := config.LoadConfig()
	if cfg == nil || cfg.AlifToolsPath == "" {
		color.Error("Error: Alif CLI not configured. Run 'alif setup' first.")
		os.Exit(1)
	}

	// 2. Build
	if buildProject != "" {
		color.Info("Building project '%s' in %s...", buildProject, solDir)
	} else {
		color.Info("Building solution in %s...", solDir)
	}

	b := builder.New(cfg)
	// Passing empty string as target board/context
	if err := b.Build(solDir, "", buildProject); err != nil {
		color.Error("Build failed: %v", err)
		os.Exit(1)
	}

	// 3. Resolve Artifacts for Signing
	solFile, _ := project.FindCsolution(solDir)
	fmt.Printf("Using solution: %s\n", solFile)
	binPath := findRecentBin(solDir)
	if binPath == "" {
		color.Error("Error: Could not locate built binary in out/ directory.")
		os.Exit(1)
	}
	color.Info("Found binary: %s", binPath)

	// 4. Sign
	// Prepare In-Place Build Dir
	signBuildDir := filepath.Dir(binPath) // Sign where the bin is
	// Output is typically "AppTocPackage.bin" in CWD (buildDir)
	tocPath := filepath.Join(signBuildDir, "AppTocPackage.bin")

	// 5. Persist State (Optimistic - before signing because app-gen-toc crashes parent process on success)
	fmt.Println("Saving build state (optimistic)...")
	// Save build state without target core info (empty)
	saveBuildState(solDir, binPath, tocPath, "")

	// We ignore return path from SignArtifact as we pre-calculated it
	s := signer.New(cfg)
	var errSign error
	// Signer is passed empty target core
	_, errSign = s.SignArtifact(solDir, signBuildDir, binPath, "", "")
	if errSign != nil {
		color.Error("Signing failed: %v", errSign)
		os.Exit(1)
	}
	color.Success("Signed artifact created: %s", tocPath)
	color.Success("Build completed successfully.")
}

func findRecentBin(root string) string {
	var recent string
	var recentTime int64
	filepath.Walk(filepath.Join(root, "out"), func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(info.Name(), ".bin") {
			fmt.Printf("Debug: Checking %s\n", info.Name())
			if !strings.Contains(info.Name(), "alif-img") && !strings.Contains(info.Name(), "AppTocPackage") {
				if info.ModTime().Unix() > recentTime {
					recentTime = info.ModTime().Unix()
					recent = path
					fmt.Printf("Debug: Selected candidate %s\n", path)
				}
			}
		}
		return nil
	})
	return recent
}

func saveBuildState(root, bin, toc, target string) {
	content := fmt.Sprintf("%s\n%s\n%s", bin, toc, target)
	_ = os.WriteFile(filepath.Join(root, ".alif_build_state"), []byte(content), 0644)
}
