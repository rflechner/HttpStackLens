package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "build error:", err)
		os.Exit(1)
	}
}

func run() error {
	if err := copyWasmExec(); err != nil {
		return fmt.Errorf("copy wasm_exec.js: %w", err)
	}
	if err := buildWasm(); err != nil {
		return fmt.Errorf("build wasm: %w", err)
	}
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

	fmt.Printf("Copied wasm_exec.js from %s\n", src)
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

	fmt.Println("WASM built successfully")
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	// Ensure the destination is writable if it already exists
	_ = os.Chmod(dst, 0o644)

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
