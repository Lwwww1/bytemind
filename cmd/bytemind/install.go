package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"bytemind/internal/config"
)

func runInstall(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	fs.SetOutput(stderr)
	installDir := fs.String("to", "", "Install directory. Defaults to BYTEMIND_HOME/bin (or ~/.bytemind/bin).")
	binaryName := fs.String("name", "", "Binary name. Defaults to bytemind (bytemind.exe on Windows).")
	addToPath := fs.Bool("add-to-path", true, "Automatically add install directory to user PATH when possible.")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) > 0 {
		return fmt.Errorf("install does not accept positional args: %s", strings.Join(fs.Args(), " "))
	}

	sourcePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve current executable: %w", err)
	}
	targetPath, err := resolveInstallTarget(*installDir, *binaryName)
	if err != nil {
		return err
	}
	if err := installBinary(sourcePath, targetPath); err != nil {
		return err
	}

	targetDir := filepath.Dir(targetPath)
	fmt.Fprintf(stdout, "Installed Bytemind to %s\n", targetPath)
	if pathContainsDir(os.Getenv("PATH"), targetDir) {
		fmt.Fprintln(stdout, "PATH already includes this directory in this terminal. You can now run: bytemind")
		return nil
	}
	if *addToPath {
		changed, err := addInstallDirToUserPath(targetDir)
		if err == nil {
			if changed {
				fmt.Fprintln(stdout, "Added install directory to user PATH.")
				fmt.Fprintln(stdout, "Open a new terminal, then run: bytemind")
			} else {
				fmt.Fprintln(stdout, "Install directory already exists in user PATH.")
			}
			return nil
		}
		fmt.Fprintf(stdout, "Automatic PATH update failed: %v\n", err)
	}

	fmt.Fprintf(stdout, "Add this directory to PATH to run Bytemind from anywhere:\n%s\n", targetDir)
	printPathHint(stdout, targetDir)
	return nil
}

func resolveInstallTarget(dirValue, nameValue string) (string, error) {
	dir := strings.TrimSpace(dirValue)
	if dir == "" {
		home, err := config.ResolveHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, "bin")
	} else {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return "", err
		}
		dir = abs
	}

	name := strings.TrimSpace(nameValue)
	if name == "" {
		name = defaultBinaryName(runtime.GOOS)
	}
	if strings.ContainsAny(name, `/\`) {
		return "", errors.New("install -name must be a file name, not a path")
	}

	return filepath.Join(dir, name), nil
}

func defaultBinaryName(goos string) string {
	if strings.EqualFold(goos, "windows") {
		return "bytemind.exe"
	}
	return "bytemind"
}

func installBinary(sourcePath, targetPath string) error {
	sourcePath = strings.TrimSpace(sourcePath)
	targetPath = strings.TrimSpace(targetPath)
	if sourcePath == "" || targetPath == "" {
		return errors.New("source and target path are required")
	}

	sourceInfo, err := os.Stat(sourcePath)
	if err != nil {
		return err
	}
	if sourceInfo.IsDir() {
		return fmt.Errorf("source executable is a directory: %s", sourcePath)
	}

	targetDir := filepath.Dir(targetPath)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return err
	}

	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	tempPath := targetPath + ".tmp"
	_ = os.Remove(tempPath)
	targetFile, err := os.OpenFile(tempPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, sourceInfo.Mode())
	if err != nil {
		return err
	}

	copyOK := false
	defer func() {
		if !copyOK {
			_ = os.Remove(tempPath)
		}
	}()

	if _, err := io.Copy(targetFile, sourceFile); err != nil {
		_ = targetFile.Close()
		return err
	}
	if err := targetFile.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tempPath, sourceInfo.Mode()); err != nil && runtime.GOOS != "windows" {
		return err
	}

	if err := os.Remove(targetPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(tempPath, targetPath); err != nil {
		return err
	}
	copyOK = true
	return nil
}

func addInstallDirToUserPath(targetDir string) (bool, error) {
	targetDir = strings.TrimSpace(targetDir)
	if targetDir == "" {
		return false, errors.New("install directory is empty")
	}
	if runtime.GOOS != "windows" {
		return false, errors.New("automatic PATH update is currently supported on Windows only")
	}
	return addToWindowsUserPath(targetDir)
}

var windowsUserPathGetter = func() (string, error) {
	cmd := exec.Command("powershell", "-NoProfile", "-Command", "[Environment]::GetEnvironmentVariable('Path','User')")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("read user PATH via PowerShell: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}

var windowsUserPathSetter = func(newPath string) error {
	cmd := exec.Command("powershell", "-NoProfile", "-Command", "[Environment]::SetEnvironmentVariable('Path', $env:BYTEMIND_USER_PATH, 'User')")
	cmd.Env = append(os.Environ(), "BYTEMIND_USER_PATH="+newPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("write user PATH via PowerShell: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func addToWindowsUserPath(targetDir string) (bool, error) {
	currentUserPath, err := windowsUserPathGetter()
	if err != nil {
		return false, err
	}
	nextUserPath, changed := appendPathEntry(currentUserPath, targetDir, true)
	if !changed {
		return false, nil
	}
	if err := windowsUserPathSetter(nextUserPath); err != nil {
		return false, err
	}
	return true, nil
}

func pathContainsDir(pathEnv, targetDir string) bool {
	return pathContainsDirForOS(pathEnv, targetDir, runtime.GOOS == "windows")
}

func appendPathEntry(pathEnv, targetDir string, windows bool) (string, bool) {
	target := strings.TrimSpace(strings.Trim(targetDir, `"`))
	if target == "" {
		return pathEnv, false
	}
	if pathContainsDirForOS(pathEnv, target, windows) {
		return pathEnv, false
	}
	if strings.TrimSpace(pathEnv) == "" {
		return target, true
	}
	sep := string(os.PathListSeparator)
	if windows {
		sep = ";"
	}
	cleanBase := strings.TrimRight(pathEnv, sep+" ")
	if cleanBase == "" {
		return target, true
	}
	return cleanBase + sep + target, true
}

func pathContainsDirForOS(pathEnv, targetDir string, windows bool) bool {
	target := normalizePathEntry(targetDir, windows)
	if target == "" {
		return false
	}
	for _, item := range splitPathListForOS(pathEnv, windows) {
		if normalizePathEntry(item, windows) == target {
			return true
		}
	}
	return false
}

func splitPathListForOS(pathEnv string, windows bool) []string {
	if windows {
		return strings.Split(pathEnv, ";")
	}
	return filepath.SplitList(pathEnv)
}

func normalizePathEntry(value string, windows bool) string {
	value = strings.TrimSpace(strings.Trim(value, `"`))
	if value == "" {
		return ""
	}
	if windows {
		value = strings.ReplaceAll(value, `\`, `/`)
		value = filepath.Clean(value)
		return strings.ToLower(value)
	}
	return filepath.Clean(value)
}

func printPathHint(w io.Writer, targetDir string) {
	if runtime.GOOS == "windows" {
		fmt.Fprintln(w, "PowerShell (current terminal):")
		fmt.Fprintf(w, "$env:Path = \"%s;\" + $env:Path\n", targetDir)
		fmt.Fprintln(w, "PowerShell (persist for future terminals):")
		fmt.Fprintf(w, "$userPath = [Environment]::GetEnvironmentVariable('Path','User'); if ($userPath -notlike '*%s*') { [Environment]::SetEnvironmentVariable('Path', $userPath + ';%s', 'User') }\n", targetDir, targetDir)
		return
	}
	fmt.Fprintln(w, "Shell (current terminal):")
	fmt.Fprintf(w, "export PATH=\"%s:$PATH\"\n", targetDir)
	fmt.Fprintln(w, "Persist in your shell profile (~/.bashrc, ~/.zshrc, etc.).")
}
