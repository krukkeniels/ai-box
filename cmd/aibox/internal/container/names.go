package container

import (
	"crypto/sha256"
	"fmt"
	"os/user"
	"path/filepath"
	"strings"
)

// ContainerName generates a deterministic container name from the workspace path.
// Format: aibox-<username>-<workspace-hash-8chars>
func ContainerName(workspacePath string) string {
	username := currentUsername()
	absPath, err := filepath.Abs(workspacePath)
	if err != nil {
		absPath = workspacePath
	}
	hash := sha256.Sum256([]byte(absPath))
	return fmt.Sprintf("aibox-%s-%x", sanitize(username), hash[:4])
}

// ContainerLabel is the label applied to all aibox containers for filtering.
const ContainerLabel = "aibox=true"

func currentUsername() string {
	u, err := user.Current()
	if err != nil {
		return "unknown"
	}
	return u.Username
}

// sanitize replaces characters that are invalid in container names.
func sanitize(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			b.WriteRune(c)
		}
	}
	result := b.String()
	if result == "" {
		return "user"
	}
	return result
}
