package targets

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"alif-cli/internal/color"
)

type TargetConfig map[string]interface{}

// GetMRAMAddress extracts mramAddress from the config
func (tc TargetConfig) GetMRAMAddress() string {
	if userApp, ok := tc["USER_APP"].(map[string]interface{}); ok {
		if addr, ok := userApp["mramAddress"].(string); ok {
			return addr
		}
	}
	// Fallback/Recursive search for flat structures (like Ethos config)
	for _, v := range tc {
		if sub, ok := v.(map[string]interface{}); ok {
			if addr, ok := sub["mramAddress"].(string); ok {
				return addr
			}
		}
	}
	return ""
}

// GetCPU extracts cpu_id from the config
func (tc TargetConfig) GetCPU() string {
	if userApp, ok := tc["USER_APP"].(map[string]interface{}); ok {
		if id, ok := userApp["cpu_id"].(string); ok {
			return id
		}
	}
	for _, v := range tc {
		if sub, ok := v.(map[string]interface{}); ok {
			if id, ok := sub["cpu_id"].(string); ok {
				return id
			}
		}
	}
	return ""
}

// ResolveTargetConfig determines the configuration to use based on priority:
// 1. Explicit file path
// 2. Auto-detected file in .alif/, build/config/, or current directory.
// If multiple files are found during auto-detection, it filters based on hints (core/project name).
// If still ambiguous, it prompts the user to select one.
// Returns the parsed config, the absolute path to the config file, and error.
func ResolveTargetConfig(explicitPath string, searchRoot string, coreHint, projectHint string) (TargetConfig, string, error) {
	var finalConfig TargetConfig
	var resolvedPath string
	var sourceDescription string

	// 1. Explicit Config
	if explicitPath != "" {
		resolvedPath = explicitPath
		content, err := os.ReadFile(explicitPath)
		if err != nil {
			return nil, "", fmt.Errorf("failed to read config file '%s': %w", explicitPath, err)
		}
		if err = json.Unmarshal(content, &finalConfig); err != nil {
			return nil, "", fmt.Errorf("failed to parse config file '%s': %w", explicitPath, err)
		}
		sourceDescription = fmt.Sprintf("Explicit file: %s", explicitPath)
	} else {
		// 2. Auto-detect logic
		root := "."
		if searchRoot != "" {
			root = searchRoot
		}

		// Collect candidates
		candidates := []string{}
		searchDirs := []string{
			filepath.Join(root, ".alif"),
			filepath.Join(root, "build", "config"),
			root,
		}

		for _, d := range searchDirs {
			files, _ := filepath.Glob(filepath.Join(d, "*.json"))
			for _, f := range files {
				// Filter out non-config files
				base := filepath.Base(f)
				if strings.Contains(base, "device-config") || base == "vcpkg-configuration.json" {
					continue
				}
				// Verify it's parseable as valid config
				content, err := os.ReadFile(f)
				if err == nil {
					var temp TargetConfig
					if json.Unmarshal(content, &temp) == nil {
						// Only consider it a candidate if it has USER_APP or reasonable keys
						if temp.GetMRAMAddress() != "" || temp.GetCPU() != "" {
							candidates = append(candidates, f)
						}
					}
				}
			}
		}

		if len(candidates) == 0 {
			return nil, "", fmt.Errorf("no configuration files found in auto-detect paths (%s). Please specify one with -c", root)
		}

		// Filter logic
		if len(candidates) > 1 {
			// Try filtering by coreHint
			if coreHint != "" {
				var filtered []string
				for _, c := range candidates {
					// e.g. M55_HE_cfg.json contains "M55_HE"
					// Hints might have special chars, so plain string contains is best for now
					if strings.Contains(strings.ToLower(filepath.Base(c)), strings.ToLower(coreHint)) {
						filtered = append(filtered, c)
					}
				}
				if len(filtered) > 0 {
					candidates = filtered // Narrow down
					if len(candidates) == 1 {
						color.Info("Auto-selected config based on core hint '%s': %s", coreHint, filepath.Base(candidates[0]))
					}
				}
			}

			// If still ambiguous, try projectHint as tie-breaker/filter
			if len(candidates) > 1 && projectHint != "" {
				var filtered []string
				for _, c := range candidates {
					if strings.Contains(strings.ToLower(filepath.Base(c)), strings.ToLower(projectHint)) {
						filtered = append(filtered, c)
					}
				}
				if len(filtered) > 0 {
					candidates = filtered // Narrow down
					if len(candidates) == 1 {
						color.Info("Auto-selected config based on project hint '%s': %s", projectHint, filepath.Base(candidates[0]))
					}
				}
			}
		}

		if len(candidates) == 1 {
			resolvedPath = candidates[0]
			if sourceDescription == "" { // Don't overwrite if auto-selected log already printed?
				// Just log generally
				// color.Info("Auto-detected single configuration file: %s", resolvedPath)
			}
		} else {
			color.Info("Multiple configuration files found:")
			for i, c := range candidates {
				fmt.Printf("[%d] %s\n", i+1, c)
			}
			fmt.Print("Select configuration file (enter number): ")
			var selection int
			_, err := fmt.Scanln(&selection)
			if err != nil || selection < 1 || selection > len(candidates) {
				return nil, "", fmt.Errorf("invalid selection")
			}
			resolvedPath = candidates[selection-1]
			color.Info("Selected configuration file: %s", resolvedPath)
		}

		content, err := os.ReadFile(resolvedPath)
		if err != nil {
			return nil, "", fmt.Errorf("failed to read selected config: %w", err)
		}
		if err = json.Unmarshal(content, &finalConfig); err != nil {
			return nil, "", fmt.Errorf("failed to parse selected config: %w", err)
		}
		sourceDescription = fmt.Sprintf("Auto-detected file: %s", resolvedPath)
	}

	// Logging
	color.Info("Configuration Source: %s", sourceDescription)
	// color.Info("Effective Values:")
	// color.Info("  - MRAM Address: %s", finalConfig.GetMRAMAddress())
	// color.Info("  - CPU ID:       %s", finalConfig.GetCPU())

	return finalConfig, resolvedPath, nil
}
