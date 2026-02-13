package builder

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"alif-cli/internal/config"
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

	// Set explicit PATH for child process finding tools if needed (though exec.Command uses parent PATH for lookups usually)
	// We return env to be set on cmd.Env
	return env
}

// ResolveContext lists available contexts and prompts user to select one if ambiguous.
func (b *Builder) ResolveContext(solutionPath, targetFilter, projectFilter string) (string, error) {
	// Find solution file
	solutionFile, _ := filepath.Glob(filepath.Join(solutionPath, "*.csolution.yml"))
	if len(solutionFile) == 0 {
		return "", fmt.Errorf("no .csolution.yml file found in %s", solutionPath)
	}
	sol := solutionFile[0]

	env := b.setupEnv()

	// List available contexts
	// Note: We generally assume cbuild is in system PATH or CMSIS Toolbox path.
	// If it's only in CMSIS Toolbox path and that's not in system PATH, exec.Command might fail unless we use absolute path.
	// But sticking to existing logic:
	cmdList := exec.Command("cbuild", "list", "contexts", sol)
	cmdList.Env = env
	out, err := cmdList.Output()
	if err != nil {
		// Provide helpful error if cbuild not found
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
		// Filter by target if provided
		if targetFilter != "" && !strings.HasSuffix(line, "+"+targetFilter) {
			continue
		}
		// Filter by project if provided (supports partial match e.g. "hello", "hello.debug")
		if projectFilter != "" && !strings.HasPrefix(line, projectFilter) {
			continue
		}
		candidates = append(candidates, line)
	}

	if len(candidates) == 0 {
		return "", fmt.Errorf("no matching build contexts found for target='%s' project='%s' in %s", targetFilter, projectFilter, sol)
	}

	var selectedContext string
	if len(candidates) == 1 {
		selectedContext = candidates[0]
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
	}

	return selectedContext, nil
}

func (b *Builder) Build(solutionPath, target, projectName string) (string, error) {
	// 1. Resolve Context
	selectedContext, err := b.ResolveContext(solutionPath, target, projectName)
	if err != nil {
		return "", err
	}

	fmt.Printf("Building context: %s\n", selectedContext)

	// find solution again purely for the command arg (ResolveContext checks it too)
	solutionFile, _ := filepath.Glob(filepath.Join(solutionPath, "*.csolution.yml"))
	sol := solutionFile[0]

	env := b.setupEnv()

	cmd := exec.Command("cbuild", sol, "--context", selectedContext, "--packs")
	cmd.Env = env
	cmd.Dir = solutionPath
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", err
	}

	return selectedContext, nil
}

// GetArtifactPath returns the expected location of the elf/bin after build
func (b *Builder) GetArtifactPath(projectPath, fullContext string) string {
	// Standard CMSIS layout: out/<project>/<target>/<build-type>/<project>.elf
	// context: project.build-type+target
	// Example: blinky.debug+E1C-HE

	parts := strings.Split(fullContext, "+")
	if len(parts) != 2 {
		return ""
	}
	target := parts[1]

	left := parts[0] // project.build-type
	lastDot := strings.LastIndex(left, ".")
	if lastDot == -1 {
		return ""
	}

	proj := left[:lastDot]
	buildType := left[lastDot+1:]

	return filepath.Join(projectPath, "out", proj, target, buildType, proj+".bin")
}
