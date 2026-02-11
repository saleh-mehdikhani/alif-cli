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

var buildTarget string
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
	buildCmd.Flags().StringVarP(&buildTarget, "board", "b", "", "Target board/core (e.g. E7-HE)")
	buildCmd.Flags().StringVarP(&buildProject, "project", "p", "", "Specific project name to build (optional)")
	buildCmd.MarkFlagRequired("board")
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
		color.Info("Building project '%s' in %s for target %s...", buildProject, solDir, buildTarget)
	} else {
		color.Info("Building solution in %s for target %s...", solDir, buildTarget)
	}

	b := builder.New(cfg)
	if err := b.Build(solDir, buildTarget, buildProject); err != nil {
		color.Error("Build failed: %v", err)
		os.Exit(1)
	}

	// 3. Resolve Artifacts for Signing
	// We need to know the specific context to find the file.
	// We can try to re-resolve logic or find the fresh bin.
	// For M55_HE, usually ends in +E7-HE.
	// Let's assume standard template naming "blinky" or "hello"
	solFile, _ := project.FindCsolution(solDir)
	// If solFile is "alif.csolution.yml", checking projects inside is hard without parsing yaml.
	// Workaround: Look for "out/*/*/*/blinky.bin" ?
	// Let's scan `out/`
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
	saveBuildState(solDir, binPath, tocPath, buildTarget)

	// We ignore return path from SignArtifact as we pre-calculated it
	s := signer.New(cfg)
	var errSign error
	_, errSign = s.SignArtifact(solDir, signBuildDir, binPath, buildTarget, "")
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
