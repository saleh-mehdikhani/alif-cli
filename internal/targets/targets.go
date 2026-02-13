package targets

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"alif-cli/internal/ui"
)

type TargetConfig map[string]interface{}

// GetMRAMAddress extracts mramAddress from the config
func (tc TargetConfig) GetMRAMAddress() string {
	if userApp, ok := tc["USER_APP"].(map[string]interface{}); ok {
		if addr, ok := userApp["mramAddress"].(string); ok {
			return addr
		}
	}
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

// ResolveTargetConfig determines the configuration to use
func ResolveTargetConfig(explicitPath string, searchRoot string, coreHint, projectHint string) (TargetConfig, string, error) {
	var finalConfig TargetConfig
	var resolvedPath string
	// var sourceDescription string // Handled by UI Item

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
		ui.Item("Config Source", "Explicit File")
		ui.Item("File", filepath.Base(explicitPath))
	} else {
		// 2. Auto-detect logic
		root := "."
		if searchRoot != "" {
			root = searchRoot
		}

		candidates := []string{}
		searchDirs := []string{
			filepath.Join(root, ".alif"),
			filepath.Join(root, "build", "config"),
			root,
		}

		for _, d := range searchDirs {
			files, _ := filepath.Glob(filepath.Join(d, "*.json"))
			for _, f := range files {
				base := filepath.Base(f)
				if strings.Contains(base, "device-config") || base == "vcpkg-configuration.json" {
					continue
				}
				content, err := os.ReadFile(f)
				if err == nil {
					var temp TargetConfig
					if json.Unmarshal(content, &temp) == nil {
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

		if len(candidates) > 1 {
			if coreHint != "" {
				var filtered []string
				for _, c := range candidates {
					if strings.Contains(strings.ToLower(filepath.Base(c)), strings.ToLower(coreHint)) {
						filtered = append(filtered, c)
					}
				}
				if len(filtered) > 0 {
					candidates = filtered
					if len(candidates) == 1 {
						ui.Item("Auto-Select", fmt.Sprintf("Based on core hint '%s'", coreHint))
					}
				}
			}

			if len(candidates) > 1 && projectHint != "" {
				var filtered []string
				for _, c := range candidates {
					if strings.Contains(strings.ToLower(filepath.Base(c)), strings.ToLower(projectHint)) {
						filtered = append(filtered, c)
					}
				}
				if len(filtered) > 0 {
					candidates = filtered
					if len(candidates) == 1 {
						ui.Item("Auto-Select", fmt.Sprintf("Based on project hint '%s'", projectHint))
					}
				}
			}
		}

		if len(candidates) == 1 {
			resolvedPath = candidates[0]
			ui.Item("Config", filepath.Base(resolvedPath))
		} else {
			fmt.Println("Multiple configuration files found:")
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
			ui.Item("Selected", filepath.Base(resolvedPath))
		}

		content, err := os.ReadFile(resolvedPath)
		if err != nil {
			return nil, "", fmt.Errorf("failed to read selected config: %w", err)
		}
		if err = json.Unmarshal(content, &finalConfig); err != nil {
			return nil, "", fmt.Errorf("failed to parse selected config: %w", err)
		}
	}

	return finalConfig, resolvedPath, nil
}
