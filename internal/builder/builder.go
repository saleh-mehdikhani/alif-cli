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

func (b *Builder) Build(solutionPath, target, projectName string) error {
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
	env = append(env, "GCC_TOOLCHAIN_13_2_1="+b.Cfg.GccToolchain)
	env = append(env, "CMSIS_PACK_ROOT="+b.Cfg.CmsisPackRoot)

	// Find solution file
	solutionFile, _ := filepath.Glob(filepath.Join(solutionPath, "*.csolution.yml"))
	if len(solutionFile) == 0 {
		return fmt.Errorf("no .csolution.yml file found")
	}
	sol := solutionFile[0]

	selectedContext, err := b.resolveContext(sol, target, projectName, env)
	if err != nil {
		return err
	}

	fmt.Printf("Building context: %s\n", selectedContext)

	cmd := exec.Command("cbuild", sol, "--context", selectedContext, "--packs")
	cmd.Env = env
	cmd.Dir = solutionPath
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func (b *Builder) resolveContext(sol, target, projectName string, env []string) (string, error) {
	// If target looks like a full context, return it
	if strings.Contains(target, ".") && strings.Contains(target, "+") {
		return target, nil
	}

	// List contexts and match
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
			// If projectName is specified, filtering
			if projectName != "" {
				// Context format is typically: project.build-type+target
				// Check if it starts with the project name
				if !strings.HasPrefix(line, projectName+".") {
					continue
				}
			}

			// prefer debug builds for development
			if strings.Contains(line, ".debug") {
				candidates = append([]string{line}, candidates...) // push to front
			} else {
				candidates = append(candidates, line)
			}
		}
	}

	if len(candidates) == 0 {
		if projectName != "" {
			return "", fmt.Errorf("no build context found matching project '%s' and target '%s'", projectName, target)
		}
		return "", fmt.Errorf("no build context found matching target '%s'", target)
	}

	// Default to first candidate (debug preferred)
	return candidates[0], nil
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
