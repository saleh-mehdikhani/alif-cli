package builder

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"alif-cli/internal/config"
	"alif-cli/internal/ui"
)

type Builder struct {
	Cfg *config.Config
}

func New(cfg *config.Config) *Builder {
	return &Builder{Cfg: cfg}
}

func (b *Builder) setupEnv() []string {
	env := os.Environ()

	// Build PATH with platform-appropriate separator
	pathSep := ":"
	if runtime.GOOS == "windows" {
		pathSep = ";"
	}

	// Prepend toolchain paths to PATH
	pathComponents := []string{
		b.Cfg.CmsisToolbox,
		b.Cfg.GccToolchain,
		os.Getenv("PATH"),
	}
	newPath := strings.Join(pathComponents, pathSep)

	env = append(env, "PATH="+newPath)
	env = append(env, "GCC_TOOLCHAIN_13_2_1="+b.Cfg.GccToolchain)
	env = append(env, "CMSIS_PACK_ROOT="+b.Cfg.CmsisPackRoot)

	return env
}

// ResolveContext lists available contexts and prompts user to select one if ambiguous.
func (b *Builder) ResolveContext(solutionPath, targetFilter, projectFilter string) (string, error) {
	ui.Header("Resolve Build Context")
	ui.Item("Filter", projectFilter)
	if targetFilter != "" {
		ui.Item("Target", targetFilter)
	}

	// Find solution file
	solutionFile, _ := filepath.Glob(filepath.Join(solutionPath, "*.csolution.yml"))
	if len(solutionFile) == 0 {
		return "", fmt.Errorf("no .csolution.yml file found in %s", solutionPath)
	}
	sol := solutionFile[0]

	env := b.setupEnv()

	// List available contexts
	cmdList := exec.Command("cbuild", "list", "contexts", sol)
	cmdList.Env = env
	out, err := cmdList.Output()
	if err != nil {
		if strings.Contains(err.Error(), "executable file not found") {
			return "", fmt.Errorf("cbuild not found. Ensure CMSIS Toolbox is installed and in PATH. Error: %v", err)
		}
		return "", fmt.Errorf("failed to list contexts: %w", err)
	}

	lines := strings.Split(string(out), "\n")
	var candidates []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if targetFilter != "" && !strings.HasSuffix(line, "+"+targetFilter) {
			continue
		}
		if projectFilter != "" && !strings.HasPrefix(line, projectFilter) {
			continue
		}
		candidates = append(candidates, line)
	}

	if len(candidates) == 0 {
		return "", fmt.Errorf("no matching build contexts found for filter='%s'", projectFilter)
	}

	var selectedContext string
	if len(candidates) == 1 {
		selectedContext = candidates[0]
		ui.Item("Selected", selectedContext)
		// ui.Success("Context resolved automatically") // Not implemented in UI yet, assume implicit
	} else {
		fmt.Println("Multiple build contexts found:")
		for i, c := range candidates {
			fmt.Printf("[%d] %s\n", i+1, c)
		}
		fmt.Print("Select context (enter number): ")
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		selection, err := strconv.Atoi(strings.TrimSpace(input))
		if err != nil || selection < 1 || selection > len(candidates) {
			return "", fmt.Errorf("invalid selection")
		}
		selectedContext = candidates[selection-1]
		ui.Item("Selected", selectedContext)
	}

	return selectedContext, nil
}

func (b *Builder) Build(solutionPath, target, projectName string, clean bool) (string, error) {
	// Find solution file first/always
	solutionFiles, _ := filepath.Glob(filepath.Join(solutionPath, "*.csolution.yml"))
	if len(solutionFiles) == 0 {
		return "", fmt.Errorf("no .csolution.yml file found in %s", solutionPath)
	}
	sol := solutionFiles[0]

	var selectedContext string
	var err error

	// If cleaning without specific filters, we Clean/Build ALL (skip selection)
	buildAll := clean && target == "" && projectName == ""

	if !buildAll {
		// 1. Resolve Context (Handles its own UI)
		selectedContext, err = b.ResolveContext(solutionPath, target, projectName)
		if err != nil {
			return "", err
		}
		ui.Header("Compile Source Code")
		ui.Item("Context", selectedContext)
	} else {
		ui.Header("Clean & Rebuild Solution")
		ui.Item("Scope", "All Contexts")
	}

	env := b.setupEnv()

	args := []string{sol, "--packs"}
	if selectedContext != "" {
		args = append(args, "--context", selectedContext)
	}
	if clean {
		args = append(args, "--rebuild")
		ui.Item("Action", "Clean & Build")
	}

	cmd := exec.Command("cbuild", args...)
	cmd.Env = env
	cmd.Dir = solutionPath

	// Capture output
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	msg := "Building..."
	if selectedContext != "" {
		msg = fmt.Sprintf("Building %s...", selectedContext)
	} else if buildAll {
		msg = "Building all contexts..."
	}

	s := ui.StartSpinner(msg)
	if err := cmd.Run(); err != nil {
		s.Fail("Build failed")
		fmt.Println("\n" + output.String()) // Print full output on error
		return "", err
	}
	s.Succeed("Build completed successfully")

	// Optional: Print size or artifacts if possible?
	// But Build returns context, caller prints artifact path.

	return selectedContext, nil
}

// GetArtifactPath returns the expected location of the elf/bin after build
func (b *Builder) GetArtifactPath(projectPath, fullContext string) string {
	parts := strings.Split(fullContext, "+")
	if len(parts) != 2 {
		return ""
	}
	target := parts[1]

	left := parts[0]
	lastDot := strings.LastIndex(left, ".")
	if lastDot == -1 {
		return ""
	}

	proj := left[:lastDot]
	buildType := left[lastDot+1:]

	return filepath.Join(projectPath, "out", proj, target, buildType, proj+".bin")
}
