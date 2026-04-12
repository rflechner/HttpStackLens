package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

var allTargets = []string{"webui", "app"}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: build-tools [target...]\n\n")
		fmt.Fprintf(os.Stderr, "Targets:\n")
		fmt.Fprintf(os.Stderr, "  webui   Build WASM and CSS (Tailwind)\n")
		fmt.Fprintf(os.Stderr, "  app     Build the native binary for the current platform\n")
		fmt.Fprintf(os.Stderr, "\nNo target builds everything.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	projectRoot, err := findProjectRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "build error:", err)
		os.Exit(1)
	}

	targets := flag.Args()
	if len(targets) == 0 {
		targets = allTargets
	}

	for _, target := range targets {
		if err := runTarget(target, projectRoot); err != nil {
			fmt.Fprintf(os.Stderr, "build error [%s]: %v\n", target, err)
			os.Exit(1)
		}
	}
}

// findProjectRoot walks up from cwd until it finds go.mod.
func findProjectRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := cwd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("cannot find go.mod from %s", cwd)
}

func runTarget(target, projectRoot string) error {
	switch target {
	case "webui":
		return buildWebUI(projectRoot)
	case "app":
		return buildApp(projectRoot)
	default:
		return fmt.Errorf("unknown target %q (available: %s)", target, strings.Join(allTargets, ", "))
	}
}

// --- webui ---

func buildWebUI(projectRoot string) error {
	webuiDir := filepath.Join(projectRoot, "webui")
	fmt.Println("→ Building WebUI...")
	if err := npmInstall(webuiDir); err != nil {
		return fmt.Errorf("npm install: %w", err)
	}
	if err := copyWasmExec(webuiDir); err != nil {
		return fmt.Errorf("copy wasm_exec.js: %w", err)
	}
	if err := buildWasm(webuiDir); err != nil {
		return fmt.Errorf("build wasm: %w", err)
	}
	if err := buildCSS(webuiDir); err != nil {
		return fmt.Errorf("build css: %w", err)
	}
	fmt.Println("✓ WebUI built")
	return nil
}

func copyWasmExec(webuiDir string) error {
	out, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		return fmt.Errorf("go env GOROOT: %w", err)
	}
	goroot := strings.TrimSpace(string(out))

	src := filepath.Join(goroot, "lib", "wasm", "wasm_exec.js")
	destDir := filepath.Join(webuiDir, "wwwroot", "js")
	dest := filepath.Join(destDir, "wasm_exec.js")

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	if err := copyFile(src, dest); err != nil {
		return err
	}
	fmt.Printf("  Copied wasm_exec.js from %s\n", src)
	return nil
}

func buildWasm(webuiDir string) error {
	outDir := filepath.Join(webuiDir, "wwwroot", "wasm")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	cmd := exec.Command("go", "build", "-o", filepath.Join(outDir, "app.wasm"), ".")
	cmd.Dir = filepath.Join(webuiDir, "wasm")
	cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	fmt.Println("  WASM built")
	return nil
}

func npmInstall(webuiDir string) error {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", "npm", "install")
	} else {
		cmd = exec.Command("npm", "install")
	}
	cmd.Dir = webuiDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	fmt.Println("  npm install done")
	return nil
}

func buildCSS(webuiDir string) error {
	input := filepath.Join(webuiDir, "src", "input.css")
	output := filepath.Join(webuiDir, "wwwroot", "css", "output.css")

	bin := filepath.Join(webuiDir, "node_modules", ".bin", "tailwindcss")

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", bin+".cmd", "-i", input, "-o", output, "--minify")
	} else {
		cmd = exec.Command(bin, "-i", input, "-o", output, "--minify")
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	fmt.Println("  CSS built")
	return nil
}

// --- app ---

func buildApp(projectRoot string) error {
	fmt.Printf("→ Building app for %s/%s...\n", runtime.GOOS, runtime.GOARCH)

	output := filepath.Join(projectRoot, "httpStackLens")
	if runtime.GOOS == "windows" {
		output += ".exe"
	}

	cmd := exec.Command("go", "build", "-ldflags=-s -w", "-o", output, ".")
	cmd.Dir = projectRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	fmt.Printf("✓ App built → %s\n", output)
	return nil
}

// --- helpers ---

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	_ = os.Chmod(dst, 0o644)

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
