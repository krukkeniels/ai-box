package runtime

// RuntimeInfo describes the detected container runtime.
type RuntimeInfo struct {
	Name    string // "podman" or "docker"
	Path    string // absolute path to binary
	Version string // version string from runtime
}

// Runtime defines the interface for container runtime operations.
type Runtime interface {
	Info() RuntimeInfo
	IsAvailable() bool
}
