package runtime

import (
	"log/slog"
	"os/exec"
	"strings"
)

// Detect finds the preferred container runtime on the host.
// It checks for podman first, then falls back to docker.
func Detect() (*RuntimeInfo, error) {
	if info, err := probe("podman"); err == nil {
		slog.Debug("detected container runtime", "name", info.Name, "path", info.Path, "version", info.Version)
		return info, nil
	}

	if info, err := probe("docker"); err == nil {
		slog.Debug("detected container runtime", "name", info.Name, "path", info.Path, "version", info.Version)
		return info, nil
	}

	return nil, ErrNoRuntime
}

func probe(name string) (*RuntimeInfo, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return nil, err
	}

	out, err := exec.Command(path, "--version").Output()
	if err != nil {
		return nil, err
	}

	return &RuntimeInfo{
		Name:    name,
		Path:    path,
		Version: strings.TrimSpace(string(out)),
	}, nil
}

// ErrNoRuntime is returned when neither podman nor docker is found.
var ErrNoRuntime = &NoRuntimeError{}

// NoRuntimeError indicates no container runtime was found.
type NoRuntimeError struct{}

func (e *NoRuntimeError) Error() string {
	return "no container runtime found: install podman (preferred) or docker"
}
