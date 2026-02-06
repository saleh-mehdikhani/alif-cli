package cmd

import (
	"os"
	"path/filepath"
	"strings"

	"alif-cli/internal/color"
	"alif-cli/internal/config"
	"alif-cli/internal/flasher"
	"alif-cli/internal/project"

	"github.com/spf13/cobra"
)

var flashCmd = &cobra.Command{
	Use:   "flash",
	Short: "Flash the last built binary",
	Long:  `Flashes the signed binary from the last successful build to the connected Alif board.`,
	Run: func(cmd *cobra.Command, args []string) {
		runFlash()
	},
}

func init() {
	rootCmd.AddCommand(flashCmd)
}

func runFlash() {
	// 1. Validate Context
	projDir, err := project.IsProjectRoot("")
	if err != nil {
		color.Error("Error: %v", err)
		os.Exit(1)
	}

	// 2. Load State
	stateFile := filepath.Join(projDir, ".alif_build_state")
	content, err := os.ReadFile(stateFile)
	if err != nil {
		color.Error("Error: No build state found. Run 'alif build' first.")
		os.Exit(1)
	}
	lines := strings.Split(string(content), "\n")
	if len(lines) < 2 {
		color.Error("Error: Invalid build state.")
		os.Exit(1)
	}
	// binPath := lines[0] // original bin
	tocPath := lines[1]
	dir := filepath.Dir(tocPath)
	signedBinPath := filepath.Join(dir, "alif-img.bin")

	cfg, _ := config.LoadConfig()

	f := flasher.New(cfg)

	// 3. Port Selection
	port, err := f.SelectPort()
	if err != nil {
		color.Error("Error identifying port: %v", err)
		os.Exit(1)
	}

	// 4. Flash
	color.Info("Flashing from %s to port %s...", dir, port)
	if err := f.Flash(signedBinPath, tocPath, port); err != nil {
		color.Error("Flash failed: %v", err)
		os.Exit(1)
	}
	color.Success("Flashing complete!")
}
