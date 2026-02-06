package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"alif-cli/internal/config"

	"github.com/spf13/cobra"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Configure Alif CLI tool paths",
	Long:  `Auto-detects or manually sets the paths for Alif Security Toolkit, CMSIS Toolbox, and GCC Toolchain.`,
	Run: func(cmd *cobra.Command, args []string) {
		runSetup()
	},
}

func init() {
	rootCmd.AddCommand(setupCmd)
}

func runSetup() {
	fmt.Println("Running Alif CLI Setup...")

	cfg, _ := config.LoadConfig() // Ignore error, start fresh if needed
	if cfg == nil {
		cfg = &config.Config{}
	}

	// 1. Detect Alif Security Toolkit
	if cfg.AlifToolsPath == "" {
		cfg.AlifToolsPath = detectAlifTools()
	}
	if cfg.AlifToolsPath == "" {
		fmt.Print("Enter path to Alif Security Toolkit (folder containing app-write-mram): ")
		fmt.Scanln(&cfg.AlifToolsPath)
	}

	// 2. Detect CMSIS Toolbox
	if cfg.CmsisToolbox == "" {
		cfg.CmsisToolbox = detectCmsisToolbox()
	}
	if cfg.CmsisToolbox == "" {
		fmt.Print("Enter path to CMSIS Toolbox bin folder (folder containing cbuild): ")
		fmt.Scanln(&cfg.CmsisToolbox)
	}

	// 3. Detect GCC Toolchain
	if cfg.GccToolchain == "" {
		cfg.GccToolchain = detectGccToolchain()
	}
	if cfg.GccToolchain == "" {
		fmt.Print("Enter path to GCC Toolchain bin folder (folder containing arm-none-eabi-gcc): ")
		fmt.Scanln(&cfg.GccToolchain)
	}

	// 4. Default CMSIS Pack Root
	if cfg.CmsisPackRoot == "" {
		cfg.CmsisPackRoot = detectCmsisPacks()
	}

	// 5. Default Signing Key (Cert) Path
	if cfg.SigningKeyPath == "" && cfg.AlifToolsPath != "" {
		// Default to the toolkit's own cert folder
		cfg.SigningKeyPath = filepath.Join(cfg.AlifToolsPath, "cert")
	}

	// Save
	if err := config.SaveConfig(cfg); err != nil {
		fmt.Printf("Error saving config: %v\n", err)
	} else {
		fmt.Println("Configuration saved successfully!")
		fmt.Printf("Toolkit: %s\nCMSIS: %s\nGCC: %s\nPacks: %s\nKeys: %s\n",
			cfg.AlifToolsPath, cfg.CmsisToolbox, cfg.GccToolchain, cfg.CmsisPackRoot, cfg.SigningKeyPath)
	}
}

// getCommonSearchDirs returns platform-appropriate common installation directories
func getCommonSearchDirs() []string {
	home, _ := os.UserHomeDir()

	dirs := []string{
		home,
		filepath.Join(home, "Downloads"),
		filepath.Join(home, "Documents"),
	}

	// Add platform-specific common directories
	switch runtime.GOOS {
	case "darwin": // macOS
		dirs = append(dirs,
			filepath.Join(home, "Projects"),
			filepath.Join(home, "Developer"),
			filepath.Join(home, "Applications"),
			"/Applications",
			"/opt",
			"/opt/homebrew",
			"/usr/local",
		)
	case "linux":
		dirs = append(dirs,
			filepath.Join(home, "projects"),
			filepath.Join(home, "workspace"),
			filepath.Join(home, "dev"),
			filepath.Join(home, "tools"),
			"/opt",
			"/usr/local",
			"/usr/share",
			"/home/tools",
		)
	case "windows":
		dirs = append(dirs,
			filepath.Join(home, "Projects"),
			filepath.Join(home, "tools"),
			"C:\\Program Files",
			"C:\\Program Files (x86)",
			"C:\\tools",
			"C:\\dev",
		)
	}

	return dirs
}

// Helpers to auto-detect common locations
func detectAlifTools() string {
	searchDirs := getCommonSearchDirs()

	// Platform-specific toolkit name patterns
	patterns := []string{
		"app-release-exec-macos",
		"app-release-exec-darwin",
		"app-release-exec-linux",
		"app-release-exec-windows",
		"app-release-exec",
		"alif-security-toolkit",
		"AlifSecurityToolkit",
		"Alif",
		"alif",
	}

	for _, baseDir := range searchDirs {
		// Check direct matches in base directory
		for _, pattern := range patterns {
			candidate := filepath.Join(baseDir, pattern)
			if _, err := os.Stat(filepath.Join(candidate, "app-write-mram")); err == nil {
				fmt.Printf("Detected Alif Tools at: %s\n", candidate)
				return candidate
			}
		}

		// Check subdirectories (1 level deep)
		entries, err := os.ReadDir(baseDir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			subDir := filepath.Join(baseDir, entry.Name())

			// Check if subdirectory itself contains the tool
			for _, pattern := range patterns {
				candidate := filepath.Join(subDir, pattern)
				if _, err := os.Stat(filepath.Join(candidate, "app-write-mram")); err == nil {
					fmt.Printf("Detected Alif Tools at: %s\n", candidate)
					return candidate
				}
			}
		}
	}
	return ""
}

func detectCmsisToolbox() string {
	searchDirs := getCommonSearchDirs()

	// Platform-specific patterns
	osArch := runtime.GOOS + "-" + runtime.GOARCH
	patterns := []string{
		filepath.Join("cmsis-toolbox", "cmsis-toolbox-"+osArch, "bin"),
		filepath.Join("cmsis-toolbox", "bin"),
		filepath.Join("CMSIS-Toolbox", "bin"),
		filepath.Join("CMSISToolbox", "bin"),
		"cmsis-toolbox/bin",
		"bin", // Direct bin folder
	}

	for _, baseDir := range searchDirs {
		// Check patterns in base directory
		for _, pattern := range patterns {
			candidate := filepath.Join(baseDir, pattern)
			if _, err := os.Stat(filepath.Join(candidate, "cbuild")); err == nil {
				fmt.Printf("Detected CMSIS Toolbox at: %s\n", candidate)
				return candidate
			}
		}

		// Search in subdirectories
		entries, err := os.ReadDir(baseDir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			subDir := filepath.Join(baseDir, entry.Name())
			for _, pattern := range patterns {
				candidate := filepath.Join(subDir, pattern)
				if _, err := os.Stat(filepath.Join(candidate, "cbuild")); err == nil {
					fmt.Printf("Detected CMSIS Toolbox at: %s\n", candidate)
					return candidate
				}
			}
		}
	}
	return ""
}

func detectGccToolchain() string {
	searchDirs := getCommonSearchDirs()

	// GCC toolchain executable name (platform-specific)
	gccBinary := "arm-none-eabi-gcc"
	if runtime.GOOS == "windows" {
		gccBinary = "arm-none-eabi-gcc.exe"
	}

	for _, baseDir := range searchDirs {
		// Check direct bin folder
		binPath := filepath.Join(baseDir, "bin")
		if _, err := os.Stat(filepath.Join(binPath, gccBinary)); err == nil {
			fmt.Printf("Detected GCC Toolchain at: %s\n", binPath)
			return binPath
		}

		// Search for arm toolchain folders
		entries, err := os.ReadDir(baseDir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			name := entry.Name()

			// Look for folders starting with common ARM toolchain patterns
			if isArmToolchainFolder(name) {
				binPath := filepath.Join(baseDir, name, "bin")
				if _, err := os.Stat(filepath.Join(binPath, gccBinary)); err == nil {
					fmt.Printf("Detected GCC Toolchain at: %s\n", binPath)
					return binPath
				}
			}
		}

		// Deep search (2 levels deep) in subdirectories
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			subDir := filepath.Join(baseDir, entry.Name())
			subEntries, err := os.ReadDir(subDir)
			if err != nil {
				continue
			}
			for _, subEntry := range subEntries {
				if !subEntry.IsDir() {
					continue
				}
				name := subEntry.Name()
				if isArmToolchainFolder(name) {
					binPath := filepath.Join(subDir, name, "bin")
					if _, err := os.Stat(filepath.Join(binPath, gccBinary)); err == nil {
						fmt.Printf("Detected GCC Toolchain at: %s\n", binPath)
						return binPath
					}
				}
			}
		}
	}
	return ""
}

// isArmToolchainFolder checks if a folder name matches common ARM toolchain patterns
func isArmToolchainFolder(name string) bool {
	if len(name) < 3 {
		return false
	}

	patterns := []string{
		"arm-gnu-toolchain",
		"arm-none-eabi",
		"gcc-arm",
		"arm-gcc",
		"GNU_Tools_ARM_Embedded",
	}

	for _, pattern := range patterns {
		if len(name) >= len(pattern) && name[:len(pattern)] == pattern {
			return true
		}
	}
	return false
}

func detectCmsisPacks() string {
	home, _ := os.UserHomeDir()

	// Platform-specific CMSIS pack locations
	var candidates []string

	switch runtime.GOOS {
	case "darwin", "linux":
		candidates = []string{
			filepath.Join(home, ".cache", "arm", "packs"),
			filepath.Join(home, ".arm", "packs"),
			filepath.Join(home, ".cmsis", "packs"),
			filepath.Join(home, "cmsis-packs"),
			filepath.Join(home, "packs"),
		}
	case "windows":
		appData := os.Getenv("APPDATA")
		localAppData := os.Getenv("LOCALAPPDATA")
		candidates = []string{
			filepath.Join(localAppData, "Arm", "Packs"),
			filepath.Join(appData, "Arm", "Packs"),
			filepath.Join(home, "cmsis-packs"),
			filepath.Join(home, "packs"),
			"C:\\Keil_v5\\ARM\\Pack",
		}
	}

	for _, p := range candidates {
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			fmt.Printf("Detected CMSIS Packs at: %s\n", p)
			return p
		}
	}

	// Default fallback based on platform
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("LOCALAPPDATA"), "Arm", "Packs")
	}
	return filepath.Join(home, ".cache", "arm", "packs")
}
