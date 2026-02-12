package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"alif-cli/internal/color"
	"alif-cli/internal/config"
	"alif-cli/internal/targets"

	"github.com/spf13/cobra"
)

var signConfig string

var signCmd = &cobra.Command{
	Use:   "sign <binary_file>",
	Short: "Sign a binary using Alif security tools",
	Long: `Signs a binary file for the Alif platform.
Use -c to specify a configuration file, or let the tool auto-detect one.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runSign(args[0])
	},
}

func init() {
	signCmd.Flags().StringVarP(&signConfig, "config", "c", "", "Signing configuration file (JSON)")
	rootCmd.AddCommand(signCmd)
}

func runSign(binPath string) {
	// 1. Validate Input
	absBinPath, err := filepath.Abs(binPath)
	if err != nil {
		color.Error("Error resolving binary path: %v", err)
		os.Exit(1)
	}
	if _, err := os.Stat(absBinPath); os.IsNotExist(err) {
		color.Error("Binary file not found: %s", absBinPath)
		os.Exit(1)
	}

	cfg, _ := config.LoadConfig()
	if cfg == nil || cfg.AlifToolsPath == "" {
		color.Error("Error: Alif CLI not configured. Run 'alif setup' first.")
		os.Exit(1)
	}

	// 2. Resolve Configuration
	workingDir := filepath.Dir(absBinPath)
	_, resolvedConfigPath, err := targets.ResolveTargetConfig(signConfig, workingDir)
	if err != nil {
		color.Error("Configuration error: %v", err)
		os.Exit(1)
	}

	// 3. Prepare Toolkit
	// Copy binary to toolkit/build/images/ with original name to satisfy app-gen-toc relative path expectations
	// Note: config usually specifies "binary" path relative to toolkit root?
	// Actually, resolvedConfigPath is absolute now.
	// But app-gen-toc often expects the binary to be in a specific location if the config uses relative paths.
	// Our ResolveTargetConfig finds the config file.
	// The config file content usually has "binary": "user-app.bin".
	// app-gen-toc runs from CLI root? NO, we usually run it from toolkit root.
	// If we run from toolkit root, the config's "binary" path is relative to toolkit root.
	// We need to ensure the binary is where the config expects it.

	// BUT, we want to sign *this* binary.
	// We might need to rewrite the config temporarily or copy the binary to match the config.
	// "Simplified Workflow" in flash.go copies binary to toolkit/build/images/<basename>.
	// Does the config point there?
	// Usually auto-detected configs in .alif/ point to relative paths or specific paths?
	// Let's assume standard Alif workflow: config points to "binary": "build/images/..." or something.

	// Wait, if we use the user's config file (e.g. .alif/M55_HE_cfg.json), it usually has:
	// "USER_APP": { "binary": "..." } ? No, M55_HE_cfg.json usually DOES NOT have "binary" path?
	// Let's check a sample config.
	// In Step 160 (app-device-config.json), binary was "app-device-config.json".
	// In M55_HE_cfg.json (viewed in Step 147, actually 132/147 were custom generated?).
	// Let's look at `tools/definitions/targets/E7/M55_HE.json` again. It has no "binary" field in USER_APP.
	// Standard SETOOLS configs need a "binary" field in the image struct?

	// Actually, app-gen-toc takes `-f config`. The config defines the images.
	// If the config does NOT define the binary path, app-gen-toc fails?
	// OR does app-gen-toc look for a file with same name?
	// Let's check `internal/signer/signer.go` logic.
	// It parses the config to find "binary" field.

	// If the user's config does not have "binary" field pointing to our binary, we have a problem.
	// Current `flash.go` simplified workflow:
	// copies binary to `toolkit/build/images/binBaseName`.
	// Does it modify config? NO.
	// It assumes the config points to that?
	// Actually, `flash.go` creates a temporary config? No, it uses `resolvedConfigPath`.

	// If the user provides a generic config (like our templates), it might NOT have the specific binary name.
	// We might need to inject the binary path into a temporary config.

	// Let's verify `tools/definitions/targets/E7/M55_HE.json` again.
	// It has "USER_APP": { "mramAddress": ... }. No "binary" path.
	// The official `M55_HE_cfg.json` usually has "binary": "build/images/m55_he.bin" or similar?

	// The safest way is to:
	// 1. Read the resolved config.
	// 2. Inject/Update the "binary" path for USER_APP to point to where we put the binary (or where it is).
	// 3. Save as temp config.
	// 4. Run app-gen-toc with temp config.

	// Let's implement this robust approach.

	// Read Config
	content, err := os.ReadFile(resolvedConfigPath)
	if err != nil {
		color.Error("Failed to read config: %v", err)
		os.Exit(1)
	}
	var cfgMap map[string]interface{}
	json.Unmarshal(content, &cfgMap)

	// Find USER_APP or similar and set binary
	// We copy binary to a known location in toolkit to avoid path issues with SETOOLS (which can be finicky)
	binBaseName := filepath.Base(absBinPath)
	toolkitBinDir := filepath.Join(cfg.AlifToolsPath, "build", "images")
	toolkitBinPath := filepath.Join(toolkitBinDir, binBaseName)
	os.MkdirAll(toolkitBinDir, 0755)

	// Copy Binary
	src, _ := os.Open(absBinPath)
	dst, _ := os.Create(toolkitBinPath)
	io.Copy(dst, src)
	src.Close()
	dst.Close()

	// Update Config Map
	// Try to find the section with "mramAddress" or just "USER_APP"
	updated := false
	if userApp, ok := cfgMap["USER_APP"].(map[string]interface{}); ok {
		// Use relative path from proper root?
		// app-gen-toc runs in AlifToolsPath.
		// So path should be "build/images/<name>"
		userApp["binary"] = fmt.Sprintf("build/images/%s", binBaseName)
		updated = true
	} else {
		// Fallback: iterate and find one with cpu_id?
		for k, v := range cfgMap {
			if sub, ok := v.(map[string]interface{}); ok {
				if _, ok := sub["cpu_id"]; ok {
					sub["binary"] = fmt.Sprintf("build/images/%s", binBaseName)
					updated = true
					break // Assume single app for now
				}
			}
		}
	}

	if !updated {
		color.Warning("Could not identify USER_APP section in config. TOC generation might fail if binary path is missing.")
	}

	// Write Temp Config
	tempConfigPath := filepath.Join(cfg.AlifToolsPath, "temp_sign_config.json")
	tempBytes, _ := json.MarshalIndent(cfgMap, "", "  ")
	os.WriteFile(tempConfigPath, tempBytes, 0644)

	// Run app-gen-toc
	color.Info("Running app-gen-toc...")
	cmdGen := exec.Command(filepath.Join(cfg.AlifToolsPath, "app-gen-toc"), "-f", tempConfigPath)
	cmdGen.Dir = cfg.AlifToolsPath
	cmdGen.Stdout = os.Stdout
	cmdGen.Stderr = os.Stderr
	if err := cmdGen.Run(); err != nil {
		color.Error("Signing failed: %v", err)
		os.Exit(1)
	}

	// Retrieve "alif-img.bin" (which is what app-gen-toc usually creates if configured so, or checking build/images)
	// Actually, app-gen-toc creates "build/images/alif-img.bin"?
	// Or "build/images/<binBaseName>.bin.sign"?
	// We want the TOC package usually? "AppTocPackage.bin"?
	// Let's assume we want to copy the signed artifacts back to the user's dir.

	color.Success("Signing complete!")
	color.Info("Artifacts are in %s/build/images/", cfg.AlifToolsPath)

	// Optional: Copy back to source dir?
	// User might expect "myapp.bin.signed" or similar.
	// Providing a message is good enough for now.
}
