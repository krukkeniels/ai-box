package mounts

import (
	"fmt"
	"os/user"
)

// Mount describes a single container mount point.
type Mount struct {
	Type        string // "bind", "volume", "tmpfs"
	Source      string // host path (bind) or volume name (volume); empty for tmpfs
	Target      string // container path
	Options     string // mount options
	Description string // human-readable purpose
}

// Layout builds the complete set of container mount points for a given
// workspace path, using the naming convention aibox-<username>-<purpose>.
func Layout(workspacePath, tmpSize, varTmpSize string) ([]Mount, error) {
	prefix, err := volumePrefix()
	if err != nil {
		return nil, fmt.Errorf("determining volume prefix: %w", err)
	}

	if tmpSize == "" {
		tmpSize = "2g"
	}
	if varTmpSize == "" {
		varTmpSize = "1g"
	}

	mounts := []Mount{
		// Workspace: bind mount from host.
		{
			Type:        "bind",
			Source:      workspacePath,
			Target:      "/workspace",
			Options:     "rw,nosuid,nodev",
			Description: "developer workspace",
		},
		// Persistent home directory.
		{
			Type:        "volume",
			Source:      prefix + "-home",
			Target:      "/home/dev",
			Options:     "rw,nosuid,nodev",
			Description: "persistent home",
		},
		// Toolpacks volume (populated in later phases).
		{
			Type:        "volume",
			Source:      prefix + "-toolpacks",
			Target:      "/opt/toolpacks",
			Options:     "rw,nosuid,nodev",
			Description: "tool packs",
		},
		// tmpfs for /tmp.
		{
			Type:        "tmpfs",
			Target:      "/tmp",
			Options:     fmt.Sprintf("rw,noexec,nosuid,size=%s", tmpSize),
			Description: "ephemeral temp",
		},
		// tmpfs for /var/tmp.
		{
			Type:        "tmpfs",
			Target:      "/var/tmp",
			Options:     fmt.Sprintf("rw,noexec,nosuid,size=%s", varTmpSize),
			Description: "ephemeral var temp",
		},
	}

	// Build cache volumes.
	for _, cv := range CacheVolumes(prefix) {
		mounts = append(mounts, Mount{
			Type:        "volume",
			Source:      cv.VolumeName,
			Target:      cv.ContainerPath,
			Options:     "rw,nosuid,nodev",
			Description: cv.Description,
		})
	}

	return mounts, nil
}

// RuntimeArgs converts the mount layout into container runtime CLI arguments
// (for podman/docker).
func RuntimeArgs(mounts []Mount) []string {
	var args []string
	for _, m := range mounts {
		switch m.Type {
		case "bind":
			args = append(args, "--mount", fmt.Sprintf("type=bind,source=%s,target=%s,%s", m.Source, m.Target, m.Options))
		case "volume":
			args = append(args, "--mount", fmt.Sprintf("type=volume,source=%s,target=%s,%s", m.Source, m.Target, m.Options))
		case "tmpfs":
			args = append(args, "--mount", fmt.Sprintf("type=tmpfs,target=%s,tmpfs-mode=1777,%s", m.Target, m.Options))
		}
	}
	return args
}

// volumePrefix returns the naming prefix for aibox volumes:
// aibox-<username>.
func volumePrefix() (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", err
	}
	return "aibox-" + u.Username, nil
}

// VolumePrefix is exported for use by other packages that need to
// reference aibox volumes by name.
func VolumePrefix() (string, error) {
	return volumePrefix()
}
