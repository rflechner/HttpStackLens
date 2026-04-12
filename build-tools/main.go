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

	targets := flag.Args()
	if len(targets) == 0 {
		targets = allTargets
	}

	for _, target := range targets {
		if err := runTarget(target); err != nil {
			fmt.Fprintf(os.Stderr, "build error [%s]: %v\n", target, err)
			os.Exit(1)
		}
	}
}

func runTarget(target string) error {
	switch target {
	case "webui":
		return buildWebUI()
	case "app":
		return buildApp()
	default:
		return fmt.Errorf("unknown target %q (available: %s)", target, strings.Join(allTargets, ", "))
	}
}

// --- webui ---

func buildWebUI() error {
	fmt.Println("→ Building WebUI...")
	if err := copyWasmExec(); err != nil {
		return fmt.Errorf("copy wasm_exec.js: %w", err)
	}
	if err := buildWasm(); err != nil {
		return fmt.Errorf("build wasm: %w", err)
	}
	if err := buildCSS(); err != nil {
		return fmt.Errorf("build css: %w", err)
	}
	fmt.Println("✓ WebUI built")
	return nil
}

func copyWasmExec() error {
	out, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		return fmt.Errorf("go env GOROOT: %w", err)
	}
	goroot := strings.TrimSpace(string(out))

	src := filepath.Join(goroot, "lib", "wasm", "wasm_exec.js")
	destDir := filepath.Join("wwwroot", "js")
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

func buildWasm() error {
	if err := os.MkdirAll(filepath.Join("wwwroot", "wasm"), 0o755); err != nil {
		return err
	}
	cmd := exec.Command("go", "build", "-o", "../wwwroot/wasm/app.wasm", ".")
	cmd.Dir = "wasm"
	cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	fmt.Println("  WASM built")
	return nil
}

func buildCSS() error {
	args := []string{"-i", "./src/input.css", "-o", "./wwwroot/css/output.css", "--minify"}

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", append([]string{"/c", `node_modules\.bin\tailwindcss.cmd`}, args...)...)
	} else {
		cmd = exec.Command("node_modules/.bin/tailwindcss", args...)
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

func buildApp() error {
	fmt.Printf("→ Building app for %s/%s...\n", runtime.GOOS, runtime.GOARCH)

	output := "httpStackLens"
	if runtime.GOOS == "windows" {
		output += ".exe"
	}

	cmd := exec.Command("go", "build", "-ldflags=-s -w", "-o", output, ".")
	cmd.Dir = ".."
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
