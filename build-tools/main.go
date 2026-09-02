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
	"time"
)

const wailsCLIVersion = "v2.13.0"

var allTargets = []string{"webui", "app"}

type options struct {
	platform     string
	skipFrontend bool
	noPackage    bool
	version      string
	commit       string
	date         string
}

type buildMetadata struct {
	version string
	commit  string
	date    string
}

func main() {
	var opts options
	flag.StringVar(&opts.platform, "platform", "", "Wails target platform (for example windows/amd64 or darwin/arm64)")
	flag.BoolVar(&opts.skipFrontend, "skip-frontend", false, "Use the already-built Web UI assets")
	flag.BoolVar(&opts.noPackage, "no-package", false, "Build the native executable without an installer or app bundle")
	flag.StringVar(&opts.version, "version", "", "Version embedded in the application (defaults to git describe)")
	flag.StringVar(&opts.commit, "commit", "", "Commit embedded in the application (defaults to git rev-parse)")
	flag.StringVar(&opts.date, "date", "", "UTC build date embedded in the application (defaults to the current time)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: build-tools [flags] [target...]\n\n")
		fmt.Fprintf(os.Stderr, "Targets:\n")
		fmt.Fprintf(os.Stderr, "  webui   Build WASM and CSS (Tailwind)\n")
		fmt.Fprintf(os.Stderr, "  app     Package the standalone Wails app into build/bin\n")
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
		// The Wails app target invokes the frontend build configured in
		// wails.json, so it is the complete default publishing pipeline.
		targets = []string{"app"}
	}

	for _, target := range targets {
		if err := runTarget(target, projectRoot, opts); err != nil {
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

func runTarget(target, projectRoot string, opts options) error {
	switch target {
	case "webui":
		return buildWebUI(projectRoot)
	case "app":
		return buildApp(projectRoot, opts)
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
	goCmd, err := goCommand("env", "GOROOT")
	if err != nil {
		return err
	}
	out, err := goCmd.Output()
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
	cmd, err := goCommand("build", "-o", filepath.Join(outDir, "app.wasm"), ".")
	if err != nil {
		return err
	}
	cmd.Dir = filepath.Join(webuiDir, "wasm")
	cmd.Env = environmentWith(cmd.Env, "GOOS", "js")
	cmd.Env = environmentWith(cmd.Env, "GOARCH", "wasm")
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
	var err error
	cmd.Env, err = buildEnvironment()
	if err != nil {
		return err
	}
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
	var err error
	cmd.Env, err = buildEnvironment()
	if err != nil {
		return err
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

func buildApp(projectRoot string, opts options) error {
	platform, targetOS, err := resolvePlatform(opts.platform)
	if err != nil {
		return err
	}
	fmt.Printf("→ Packaging standalone Wails app for %s...\n", platform)
	compiler, err := currentGoCompiler()
	if err != nil {
		return err
	}
	fmt.Printf("  Go compiler: %s (%s)\n", compiler, runtime.Version())

	if err := ensureWailsIcon(projectRoot); err != nil {
		return fmt.Errorf("prepare Wails app icon: %w", err)
	}

	args := []string{
		"build",
		"-clean",
		"-skipbindings",
		"-trimpath",
		"-compiler", compiler,
		"-platform", platform,
		"-ldflags", versionLdflags(projectRoot, targetOS, buildMetadata{
			version: opts.version,
			commit:  opts.commit,
			date:    opts.date,
		}),
	}
	if opts.skipFrontend {
		args = append(args, "-s")
	}
	if opts.noPackage {
		args = append(args, "-nopackage")
	}
	if targetOS == "windows" {
		// Keep the distributable to one executable while still allowing Wails
		// to install WebView2 on machines where the runtime is missing.
		args = append(args, "-webview2", "embed")
	}

	cmd, source, err := wailsCommand(args...)
	if err != nil {
		return err
	}
	fmt.Printf("  Wails CLI: %s\n", source)
	cmd.Dir = projectRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("wails build: %w", err)
	}

	output := filepath.Join(projectRoot, "build", "bin", "HttpStackLens")
	if targetOS == "windows" {
		output += ".exe"
	} else if targetOS == "darwin" && !opts.noPackage {
		output += ".app"
	}
	if _, err := os.Stat(output); err != nil {
		return fmt.Errorf("Wails reported success but output %q is unavailable: %w", output, err)
	}
	fmt.Printf("✓ Standalone Wails app built → %s\n", output)
	return nil
}

func resolvePlatform(platform string) (resolved string, targetOS string, err error) {
	if platform == "" {
		return runtime.GOOS + "/" + runtime.GOARCH, runtime.GOOS, nil
	}
	parts := strings.Split(platform, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid platform %q (expected os/arch)", platform)
	}
	return platform, parts[0], nil
}

// wailsCommand prefers an installed CLI, including the default GOPATH/bin
// location when it is absent from PATH. As a final fallback, `go run` downloads
// and executes the version matching the runtime dependency in go.mod.
func wailsCommand(args ...string) (*exec.Cmd, string, error) {
	if path, err := exec.LookPath("wails"); err == nil {
		cmd := exec.Command(path, args...)
		cmd.Env, err = buildEnvironment()
		if err != nil {
			return nil, "", err
		}
		return cmd, path, nil
	}

	goEnvCmd, err := goCommand("env", "GOPATH")
	if err != nil {
		return nil, "", err
	}
	goPathOutput, err := goEnvCmd.Output()
	if err != nil {
		return nil, "", fmt.Errorf("locate GOPATH for Wails CLI: %w", err)
	}
	goPath := strings.TrimSpace(string(goPathOutput))
	cliName := "wails"
	if runtime.GOOS == "windows" {
		cliName += ".exe"
	}
	goPathCLI := filepath.Join(goPath, "bin", cliName)
	if info, statErr := os.Stat(goPathCLI); statErr == nil && !info.IsDir() {
		cmd := exec.Command(goPathCLI, args...)
		cmd.Env, err = buildEnvironment()
		if err != nil {
			return nil, "", err
		}
		return cmd, goPathCLI, nil
	}

	packageName := "github.com/wailsapp/wails/v2/cmd/wails@" + wailsCLIVersion
	goArgs := append([]string{"run", packageName}, args...)
	cmd, err := goCommand(goArgs...)
	if err != nil {
		return nil, "", err
	}
	return cmd, "go run " + packageName, nil
}

// currentGoCompiler locates a compiler whose version exactly matches the one
// that built this tool. In particular, it does not trust GOROOT: GoLand may
// leave that variable pointing at its project SDK even when Go's automatic
// toolchain selection compiled this module with a newer downloaded toolchain.
func currentGoCompiler() (string, error) {
	wanted := runtime.Version()
	found := make([]string, 0)
	for _, compiler := range goCompilerCandidates() {
		info, err := os.Stat(compiler)
		if err != nil || info.IsDir() {
			continue
		}

		cmd := exec.Command(compiler, "version")
		cmd.Env = goProbeEnvironment()
		out, err := cmd.Output()
		if err != nil {
			continue
		}
		fields := strings.Fields(string(out))
		if len(fields) < 3 {
			continue
		}
		actual := fields[2]
		found = append(found, fmt.Sprintf("%s (%s)", compiler, actual))
		if actual == wanted {
			return compiler, nil
		}
	}
	return "", fmt.Errorf("Go compiler %s was not found; candidates: %s", wanted, strings.Join(found, ", "))
}

func goCompilerCandidates() []string {
	name := "go"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	candidates := make([]string, 0)
	add := func(path string) {
		if path == "" {
			return
		}
		for _, existing := range candidates {
			if strings.EqualFold(existing, path) {
				return
			}
		}
		candidates = append(candidates, path)
	}

	if goRoot := os.Getenv("GOROOT"); goRoot != "" {
		add(filepath.Join(goRoot, "bin", name))
	}
	if goToolDir := os.Getenv("GOTOOLDIR"); goToolDir != "" {
		add(filepath.Join(filepath.Clean(filepath.Join(goToolDir, "..", "..", "..")), "bin", name))
	}
	for _, directory := range filepath.SplitList(os.Getenv("PATH")) {
		add(filepath.Join(directory, name))
	}
	if programFiles := os.Getenv("ProgramFiles"); programFiles != "" {
		add(filepath.Join(programFiles, "Go", "bin", name))
	}
	if runtime.GOOS != "windows" {
		add(filepath.Join("/usr/local/go/bin", name))
	}

	goPaths := filepath.SplitList(os.Getenv("GOPATH"))
	if len(goPaths) == 0 {
		if home, err := os.UserHomeDir(); err == nil {
			goPaths = []string{filepath.Join(home, "go")}
		}
	}
	toolchainVersion := strings.TrimPrefix(runtime.Version(), "go")
	toolchainFolder := fmt.Sprintf("toolchain@v0.0.1-go%s.%s-%s", toolchainVersion, runtime.GOOS, runtime.GOARCH)
	for _, goPath := range goPaths {
		add(filepath.Join(goPath, "pkg", "mod", "golang.org", toolchainFolder, "bin", name))
	}
	return candidates
}

func goCommand(args ...string) (*exec.Cmd, error) {
	compiler, err := currentGoCompiler()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(compiler, args...)
	cmd.Env, err = buildEnvironmentFor(compiler)
	if err != nil {
		return nil, err
	}
	return cmd, nil
}

func buildEnvironment() ([]string, error) {
	compiler, err := currentGoCompiler()
	if err != nil {
		return nil, err
	}
	return buildEnvironmentFor(compiler)
}

func buildEnvironmentFor(compiler string) ([]string, error) {
	goRoot := filepath.Dir(filepath.Dir(compiler))
	if _, err := os.Stat(filepath.Join(goRoot, "src", "runtime")); err != nil {
		return nil, fmt.Errorf("invalid GOROOT derived from Go compiler %q: %w", compiler, err)
	}

	env := os.Environ()
	env = environmentWith(env, "GOROOT", goRoot)
	env = environmentWith(env, "GOTOOLCHAIN", "local")

	goBin := filepath.Dir(compiler)
	pathValue := os.Getenv("PATH")
	if pathValue == "" {
		pathValue = goBin
	} else {
		pathValue = goBin + string(os.PathListSeparator) + pathValue
	}
	return environmentWith(env, "PATH", pathValue), nil
}

func goProbeEnvironment() []string {
	env := environmentWithout(os.Environ(), "GOROOT")
	return environmentWith(env, "GOTOOLCHAIN", "local")
}

func environmentWith(env []string, key, value string) []string {
	prefix := key + "="
	return append(environmentWithout(env, key), prefix+value)
}

func environmentWithout(env []string, key string) []string {
	result := make([]string, 0, len(env)+1)
	for _, entry := range env {
		name, _, found := strings.Cut(entry, "=")
		if found && strings.EqualFold(name, key) {
			continue
		}
		result = append(result, entry)
	}
	return result
}

func ensureWailsIcon(projectRoot string) error {
	source := filepath.Join(projectRoot, "images", "logo-v2.png")
	destination := filepath.Join(projectRoot, "build", "appicon.png")
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	if err := copyFile(source, destination); err != nil {
		return err
	}

	// Wails only generates icon.ico when it is absent. Remove the generated
	// cache so a branding update is always reflected in the packaged binary.
	generatedIcon := filepath.Join(projectRoot, "build", "windows", "icon.ico")
	if err := os.Remove(generatedIcon); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// versionLdflags builds the -ldflags value, injecting build metadata into the
// main package's version/commit/date variables. Version and commit come from
// git; when git is unavailable (not a repo, not installed) the corresponding -X
// flags are omitted and main's compiled-in defaults ("dev"/"none") stand.
func versionLdflags(projectRoot, targetOS string, metadata buildMetadata) string {
	flags := "-s -w"
	if targetOS == "windows" {
		flags += " -H windowsgui"
	}
	version := metadata.version
	if version == "" {
		version = gitOutput(projectRoot, "describe", "--tags", "--always", "--dirty")
	}
	if version != "" {
		flags += " -X main.version=" + version
	}
	commit := metadata.commit
	if commit == "" {
		commit = gitOutput(projectRoot, "rev-parse", "--short", "HEAD")
	}
	if commit != "" {
		flags += " -X main.commit=" + commit
	}
	date := metadata.date
	if date == "" {
		date = time.Now().UTC().Format("2006-01-02T15:04:05Z")
	}
	flags += " -X main.date=" + date
	fmt.Printf("  ldflags: %s\n", flags)
	return flags
}

// gitOutput runs a git command in projectRoot and returns its trimmed stdout,
// or "" on any error.
func gitOutput(projectRoot string, args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = projectRoot
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
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
