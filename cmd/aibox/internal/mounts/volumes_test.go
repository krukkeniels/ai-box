package mounts

import (
	"strings"
	"testing"
)

func TestCacheVolumes_Count(t *testing.T) {
	vols := CacheVolumes("aibox-testuser")
	// Spec requires: maven, gradle, npm, yarn, bazel, nuget.
	if len(vols) != 6 {
		t.Errorf("CacheVolumes() returned %d volumes, want 6", len(vols))
	}
}

func TestCacheVolumes_NamingConvention(t *testing.T) {
	prefix := "aibox-alice"
	vols := CacheVolumes(prefix)

	for _, v := range vols {
		if !strings.HasPrefix(v.VolumeName, prefix+"-") {
			t.Errorf("volume %q should start with %q", v.VolumeName, prefix+"-")
		}
	}
}

func TestCacheVolumes_ExpectedNames(t *testing.T) {
	prefix := "aibox-bob"
	vols := CacheVolumes(prefix)

	expectedSuffixes := []string{
		"-maven-cache",
		"-gradle-cache",
		"-npm-cache",
		"-yarn-cache",
		"-bazel-cache",
		"-nuget-cache",
	}

	names := make(map[string]bool)
	for _, v := range vols {
		names[v.VolumeName] = true
	}

	for _, suffix := range expectedSuffixes {
		want := prefix + suffix
		if !names[want] {
			t.Errorf("missing expected cache volume %q", want)
		}
	}
}

func TestCacheVolumes_ContainerPaths(t *testing.T) {
	vols := CacheVolumes("aibox-user")

	expectedPaths := map[string]bool{
		"/home/dev/.m2/repository": false,
		"/home/dev/.gradle/caches": false,
		"/home/dev/.npm":           false,
		"/home/dev/.yarn/cache":    false,
		"/home/dev/.cache/bazel":   false,
		"/home/dev/.nuget/packages": false,
	}

	for _, v := range vols {
		if _, ok := expectedPaths[v.ContainerPath]; ok {
			expectedPaths[v.ContainerPath] = true
		}
	}

	for path, found := range expectedPaths {
		if !found {
			t.Errorf("missing cache volume with container path %q", path)
		}
	}
}

func TestCacheVolumes_Descriptions(t *testing.T) {
	vols := CacheVolumes("aibox-user")

	for _, v := range vols {
		if v.Description == "" {
			t.Errorf("cache volume %q has empty description", v.VolumeName)
		}
	}
}

func TestCacheVolumes_DifferentPrefixes(t *testing.T) {
	alice := CacheVolumes("aibox-alice")
	bob := CacheVolumes("aibox-bob")

	if len(alice) != len(bob) {
		t.Fatal("different prefixes should produce same number of volumes")
	}

	for i := range alice {
		if alice[i].VolumeName == bob[i].VolumeName {
			t.Errorf("volumes for different users should have different names: %q", alice[i].VolumeName)
		}
		// Container paths should be identical across users.
		if alice[i].ContainerPath != bob[i].ContainerPath {
			t.Errorf("container paths should be identical across users: alice=%q bob=%q",
				alice[i].ContainerPath, bob[i].ContainerPath)
		}
	}
}
