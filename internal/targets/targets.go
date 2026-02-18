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

// SyncToolkitConfig updates the toolkit's global configuration to match the project's target device
func SyncToolkitConfig(alifToolsPath string, targetID string) error {
	if alifToolsPath == "" || targetID == "" {
		return nil
	}

	// 1. Resolve the full Part# string from devicesDB.db
	devicesDBPath := filepath.Join(alifToolsPath, "utils", "devicesDB.db")
	dbBytes, err := os.ReadFile(devicesDBPath)
	if err != nil {
		return fmt.Errorf("failed to read devices database: %w", err)
	}

	var devices map[string]interface{}
	if err := json.Unmarshal(dbBytes, &devices); err != nil {
		return fmt.Errorf("failed to parse devices database: %w", err)
	}

	// Strip core suffix if present (e.g., AE722F80F55D5LS:M55_HE -> AE722F80F55D5LS)
	id := strings.Split(targetID, ":")[0]
	var fullPartName string
	for key := range devices {
		if strings.Contains(key, id) {
			fullPartName = key
			break
		}
	}

	if fullPartName == "" {
		// Log a debug message but don't error.
		// A core name (like M55_HE) won't match a Part#, which is fine.
		return nil
	}

	// 2. Load global-cfg.db
	globalCfgPath := filepath.Join(alifToolsPath, "utils", "global-cfg.db")
	cfgBytes, err := os.ReadFile(globalCfgPath)
	if err != nil {
		return fmt.Errorf("failed to read toolkit global config: %w", err)
	}

	var globalCfg map[string]map[string]interface{}
	if err := json.Unmarshal(cfgBytes, &globalCfg); err != nil {
		return fmt.Errorf("failed to parse toolkit global config: %w", err)
	}

	if globalCfg["DEVICE"] == nil {
		globalCfg["DEVICE"] = make(map[string]interface{})
	}

	// 3. Resolve valid revisions for this device
	deviceInfo, _ := devices[fullPartName].(map[string]interface{})
	featureSet, _ := deviceInfo["featureSet"].(string)

	validRev := ""
	featuresDBPath := filepath.Join(alifToolsPath, "utils", "featuresDB.db")
	if fdbBytes, err := os.ReadFile(featuresDBPath); err == nil {
		var features map[string]interface{}
		if err := json.Unmarshal(fdbBytes, &features); err == nil {
			if feat, ok := features[featureSet].(map[string]interface{}); ok {
				if revs, ok := feat["revisions"].([]interface{}); ok && len(revs) > 0 {
					currentRev, _ := globalCfg["DEVICE"]["Revision"].(string)
					// Check if current is valid
					for _, r := range revs {
						if r.(string) == currentRev {
							validRev = currentRev
							break
						}
					}
					// If not valid, pick the first one
					if validRev == "" {
						validRev = revs[0].(string)
					}
				}
			}
		}
	}

	// Default to A0 if we couldn't find anything in DBs (fallback)
	if validRev == "" {
		validRev = "A0"
	}

	// 4. Update and check if actual changes are needed
	needsUpdate := false
	if globalCfg["DEVICE"]["Part#"] != fullPartName {
		ui.Item("Toolkit Sync", fmt.Sprintf("Part# → %s", id))
		globalCfg["DEVICE"]["Part#"] = fullPartName
		needsUpdate = true
	} else {
		ui.Item("Toolkit Target", id)
	}

	if globalCfg["DEVICE"]["Revision"] != validRev {
		ui.Item("Toolkit Sync", fmt.Sprintf("Rev → %s", validRev))
		globalCfg["DEVICE"]["Revision"] = validRev
		needsUpdate = true
	} else {
		ui.Item("Toolkit Rev", validRev)
	}

	if !needsUpdate {
		return nil
	}

	newCfgBytes, err := json.MarshalIndent(globalCfg, "", "    ")
	if err != nil {
		return fmt.Errorf("failed to encode toolkit global config: %w", err)
	}

	return os.WriteFile(globalCfgPath, newCfgBytes, 0644)
}
