package dotfiles

import "testing"

func TestSync_EmptyRepo(t *testing.T) {
	// Sync with empty repo URL should be a no-op.
	err := Sync(SyncOpts{
		RuntimePath:   "/usr/bin/podman",
		ContainerName: "test-container",
		RepoURL:       "",
		Shell:         "bash",
	})
	if err != nil {
		t.Errorf("Sync() with empty repo should succeed: %v", err)
	}
}

func TestSync_InvalidRuntime(t *testing.T) {
	// Sync with non-existent runtime should fail gracefully when repo is set.
	err := Sync(SyncOpts{
		RuntimePath:   "/nonexistent/podman",
		ContainerName: "test-container",
		RepoURL:       "https://github.com/user/dotfiles.git",
		Shell:         "bash",
	})
	if err == nil {
		t.Error("Sync() with non-existent runtime should return error")
	}
}
