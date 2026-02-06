package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"alif-cli/internal/builder"
	"alif-cli/internal/config"
	"alif-cli/internal/project"
	"alif-cli/internal/signer"

	"github.com/spf13/cobra"
)

var buildTarget string

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build and sign the project",
	Long:  `Compiles the source code using cbuild and signs artifacts using Alif security tools.`,
	Run: func(cmd *cobra.Command, args []string) {
		runBuild()
	},
}

func init() {
	buildCmd.Flags().StringVarP(&buildTarget, "board", "b", "", "Target board/core (e.g. E7-HE)")
	buildCmd.MarkFlagRequired("board")
	rootCmd.AddCommand(buildCmd)
}

func runBuild() {
	// 1. Validate Project
	projDir, err := project.IsProjectRoot("") // check CWD
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	cfg, _ := config.LoadConfig()
	if cfg == nil || cfg.AlifToolsPath == "" {
		fmt.Println("Error: Alif CLI not configured. Run 'alif setup' first.")
		os.Exit(1)
	}

	// 2. Build
	fmt.Printf("Building project in %s for target %s...\n", projDir, buildTarget)
	b := builder.New(cfg)
	if err := b.Build(projDir, buildTarget); err != nil {
		fmt.Printf("Build failed: %v\n", err)
		os.Exit(1)
	}

	// 3. Resolve Artifacts for Signing
	// We need to know the specific context to find the file.
	// We can try to re-resolve logic or find the fresh bin.
	// For M55_HE, usually ends in +E7-HE.
	// Let's assume standard template naming "blinky" or "hello"
	solFile, _ := project.FindCsolution(projDir)
	// If solFile is "alif.csolution.yml", checking projects inside is hard without parsing yaml.
	// Workaround: Look for "out/*/*/*/blinky.bin" ?
	// Let's scan `out/`
	fmt.Printf("Using solution: %s\n", solFile)
	binPath := findRecentBin(projDir)
	if binPath == "" {
		fmt.Println("Error: Could not locate built binary in out/ directory.")
		os.Exit(1)
	}
	fmt.Printf("Found binary: %s\n", binPath)

	// 4. Sign
	// Prepare In-Place Build Dir
	signBuildDir := filepath.Dir(binPath) // Sign where the bin is
	// Output is typically "AppTocPackage.bin" in CWD (buildDir)
	tocPath := filepath.Join(signBuildDir, "AppTocPackage.bin")

	// 5. Persist State (Optimistic - before signing because app-gen-toc crashes parent process on success)
	fmt.Println("Saving build state (optimistic)...")
	saveBuildState(projDir, binPath, tocPath)

	// We ignore return path from SignArtifact as we pre-calculated it
	s := signer.New(cfg)
	var errSign error
	_, errSign = s.SignArtifact(projDir, signBuildDir, binPath, buildTarget)
	if errSign != nil {
		fmt.Printf("Signing failed: %v\n", errSign)
		os.Exit(1)
	}
	fmt.Printf("Signed artifact created: %s\n", tocPath)
	fmt.Println("Build completed successfully.")
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

func saveBuildState(root, bin, toc string) {
	content := fmt.Sprintf("%s\n%s", bin, toc)
	_ = os.WriteFile(filepath.Join(root, ".alif_build_state"), []byte(content), 0644)
}
