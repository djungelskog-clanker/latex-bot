package render

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const typstPackageMount = "/typst-packages"

var TypstPackageDir = defaultTypstPackageDir()

func defaultTypstPackageDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "/nonexistent"
	}
	return filepath.Join(home, ".local", "share", "latex-bot", "typst-packages")
}

func sandboxed(dir, name string, args ...string) []string {
	command := []string{
		addressSpaceCap, cpuSecondsCap, fileSizeCap, "--",
		"bwrap",
		"--unshare-all", "--die-with-parent", "--new-session", "--clearenv",
		"--setenv", "PATH", "/usr/local/bin:/usr/bin",
		"--setenv", "HOME", "/work",
		"--ro-bind", "/usr", "/usr",
		"--symlink", "usr/bin", "/bin",
		"--symlink", "usr/lib", "/lib",
		"--symlink", "usr/lib64", "/lib64",
		"--ro-bind", "/etc/texlive", "/etc/texlive",
		"--ro-bind", "/var/lib/texmf", "/var/lib/texmf",
		"--ro-bind-try", TypstPackageDir, typstPackageMount,
		"--proc", "/proc",
		"--dev", "/dev",
		"--tmpfs", "/tmp",
		"--bind", dir, "/work",
		"--chdir", "/work",
		"--", name,
	}
	return append(command, args...)
}

func run(ctx context.Context, dir, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "prlimit", sandboxed(dir, name, args...)...)
	cmd.Env = []string{"PATH=/usr/bin:/bin"}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = time.Second

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		return stdout.String(), nil
	}
	if ctx.Err() != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", invalid("%s timed out", name)
		}
		return "", ctx.Err()
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return "", invalid("%s failed: %s", name, firstCompileError(stdout.String()+"\n"+stderr.String()))
	}
	return "", fmt.Errorf("running %s: %w", name, err)
}

func firstCompileError(log string) string {
	for _, line := range strings.Split(log, "\n") {
		for _, prefix := range []string{"! ", "error: "} {
			if strings.HasPrefix(line, prefix) {
				return strings.TrimPrefix(line, prefix)
			}
		}
	}
	return "unknown error"
}
