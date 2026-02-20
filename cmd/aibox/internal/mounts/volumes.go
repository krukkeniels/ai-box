package mounts

import (
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
)

// CacheVolume describes a build cache named volume.
type CacheVolume struct {
	VolumeName    string
	ContainerPath string
	Description   string
}

// CacheVolumes returns the set of build cache volumes for a given prefix.
func CacheVolumes(prefix string) []CacheVolume {
	return []CacheVolume{
		{
			VolumeName:    prefix + "-maven-cache",
			ContainerPath: "/home/dev/.m2/repository",
			Description:   "Maven cache",
		},
		{
			VolumeName:    prefix + "-gradle-cache",
			ContainerPath: "/home/dev/.gradle/caches",
			Description:   "Gradle cache",
		},
		{
			VolumeName:    prefix + "-npm-cache",
			ContainerPath: "/home/dev/.npm",
			Description:   "npm cache",
		},
		{
			VolumeName:    prefix + "-yarn-cache",
			ContainerPath: "/home/dev/.yarn/cache",
			Description:   "Yarn cache",
		},
		{
			VolumeName:    prefix + "-bazel-cache",
			ContainerPath: "/home/dev/.cache/bazel",
			Description:   "Bazel cache",
		},
		{
			VolumeName:    prefix + "-nuget-cache",
			ContainerPath: "/home/dev/.nuget/packages",
			Description:   "NuGet package cache",
		},
	}
}

// EnsureVolumes creates any missing named volumes using the container runtime.
func EnsureVolumes(rtPath string, mounts []Mount) error {
	for _, m := range mounts {
		if m.Type != "volume" {
			continue
		}
		if volumeExists(rtPath, m.Source) {
			slog.Debug("volume already exists", "name", m.Source)
			continue
		}
		slog.Info("creating volume", "name", m.Source)
		out, err := exec.Command(rtPath, "volume", "create", m.Source).CombinedOutput()
		if err != nil {
			return fmt.Errorf("creating volume %s: %w\n%s", m.Source, err, string(out))
		}
	}
	return nil
}

// ListVolumes returns the names of all aibox-related volumes.
func ListVolumes(rtPath, prefix string) ([]string, error) {
	out, err := exec.Command(rtPath, "volume", "ls", "--format", "{{.Name}}").Output()
	if err != nil {
		return nil, fmt.Errorf("listing volumes: %w", err)
	}

	var result []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		name := strings.TrimSpace(line)
		if name != "" && strings.HasPrefix(name, prefix) {
			result = append(result, name)
		}
	}
	return result, nil
}

// RemoveCacheVolumes removes all build cache volumes for the given prefix.
// Home and toolpacks volumes are not removed.
func RemoveCacheVolumes(rtPath, prefix string) error {
	for _, cv := range CacheVolumes(prefix) {
		if !volumeExists(rtPath, cv.VolumeName) {
			continue
		}
		slog.Info("removing cache volume", "name", cv.VolumeName)
		out, err := exec.Command(rtPath, "volume", "rm", cv.VolumeName).CombinedOutput()
		if err != nil {
			return fmt.Errorf("removing volume %s: %w\n%s", cv.VolumeName, err, string(out))
		}
	}
	return nil
}

func volumeExists(rtPath, name string) bool {
	err := exec.Command(rtPath, "volume", "inspect", name).Run()
	return err == nil
}
