package builder

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"alif-cli/internal/config"
)

type Builder struct {
	Cfg *config.Config
}

func New(cfg *config.Config) *Builder {
	return &Builder{Cfg: cfg}
}

func (b *Builder) Build(projectPath, target string) error {
	// Construct the context string, e.g., "blinky.debug+E7-HE"
	// For simplicity, we assume project name is "blinky" or "hello" etc.
	// In a real scenario we'd parse the YAML to find valid contexts.
	// Here we try to accept exact context or try to be smart.

	// Validating inputs
	if target == "" {
		return fmt.Errorf("target is required (e.g., E7-HE)")
	}

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
	env = append(env, "GCC_TOOLCHAIN_13_2_1="+b.Cfg.GccToolchain) // Used by some cbuild templates
	env = append(env, "CMSIS_PACK_ROOT="+b.Cfg.CmsisPackRoot)

	// Command: cbuild <file> --context <context> --packs
	// If the user gives just "E7-HE", we might not know the project name part (e.g. "blinky").
	// We will try to find a matching context via 'cbuild list contexts' optionally,
	// OR we assume the user might pass "blinky.debug+E7-HE".
	//
	// Let's rely on cbuild's behavior or ask user for full context if needed.
	// BUT per requirements "alif build -b <target>" -> likely implies strict "Target Type".
	// cbuild allows filtering.

	// Heuristic: If target has no ".", assume it is just the Board/Processor part, e.g. "E7-HE".
	// We will list contexts and pick the one that ends with "+<target>".

	solutionFile, _ := filepath.Glob(filepath.Join(projectPath, "*.csolution.yml"))
	if len(solutionFile) == 0 {
		return fmt.Errorf("no solution file found")
	}
	sol := solutionFile[0]

	selectedContext, err := b.resolveContext(sol, target, env)
	if err != nil {
		return err
	}

	fmt.Printf("Building context: %s\n", selectedContext)

	cmd := exec.Command("cbuild", sol, "--context", selectedContext, "--packs")
	cmd.Env = env
	cmd.Dir = projectPath
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func (b *Builder) resolveContext(sol, target string, env []string) (string, error) {
	// If target looks like a full context, return it
	if strings.Contains(target, ".") && strings.Contains(target, "+") {
		return target, nil
	}

	// Otherwise list contexts and match
	cmd := exec.Command("cbuild", "list", "contexts", sol)
	cmd.Env = env
	out, err := cmd.Output()
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
		// check if it ends with +TARGET or contains +TARGET
		if strings.HasSuffix(line, "+"+target) {
			// prefer debug builds for development
			if strings.Contains(line, ".debug") {
				candidates = append([]string{line}, candidates...) // push to front
			} else {
				candidates = append(candidates, line)
			}
		}
	}

	if len(candidates) == 0 {
		return "", fmt.Errorf("no build context found matching target '%s'", target)
	}

	// Default to first candidate (debug preferred)
	return candidates[0], nil
}

// GetArtifactPath returns the expected location of the elf/bin after build
// This logic duplicates some of cbuild's internal logic, but is necessary to find the file to sign.
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
