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

func (b *Builder) Build(solutionPath, target, projectName string) (string, error) {
	// Setup Environment
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

	// Find solution file
	solutionFile, _ := filepath.Glob(filepath.Join(solutionPath, "*.csolution.yml"))
	if len(solutionFile) == 0 {
		return "", fmt.Errorf("no .csolution.yml file found")
	}
	sol := solutionFile[0]

	// List available contexts
	cmdList := exec.Command("cbuild", "list", "contexts", sol)
	cmdList.Env = env
	out, err := cmdList.Output()
	if err != nil {
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
		if target != "" && !strings.HasSuffix(line, "+"+target) {
			continue
		}
		// Filter by project if provided (supports partial match e.g. "hello", "hello.debug", "hello.debug+E7-HE")
		if projectName != "" && !strings.HasPrefix(line, projectName) {
			continue
		}
		candidates = append(candidates, line)
	}

	if len(candidates) == 0 {
		return "", fmt.Errorf("no matching build contexts found for target='%s' project='%s'", target, projectName)
	}

	var selectedContext string
	if len(candidates) == 1 {
		selectedContext = candidates[0]
	} else {
		fmt.Println("Multiple build contexts found:")
		for i, c := range candidates {
			fmt.Printf("[%d] %s\n", i+1, c)
		}
		fmt.Print("Select context to build (enter number): ")
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		selection, err := strconv.Atoi(strings.TrimSpace(input))
		if err != nil || selection < 1 || selection > len(candidates) {
			return "", fmt.Errorf("invalid selection")
		}
		selectedContext = candidates[selection-1]
	}

	fmt.Printf("Building context: %s\n", selectedContext)

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
