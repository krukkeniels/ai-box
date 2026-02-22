package mcppacks

// BuiltinPacks returns the set of built-in MCP pack manifests.
// These are the packs that ship with AI-Box and are always available.
func BuiltinPacks() []Manifest {
	return []Manifest{
		{
			Name:        "filesystem-mcp",
			Version:     "1.0.0",
			Description: "File operations sandboxed to /workspace",
			Command:     "aibox-mcp-filesystem",
			Args:        []string{"--root", "/workspace"},
			// No network required -- purely local file operations.
			FilesystemRequires: []string{"/workspace"},
			Permissions:        []string{"read", "write"},
		},
		{
			Name:            "git-mcp",
			Version:         "1.0.0",
			Description:     "Git operations for repository management",
			Command:         "aibox-mcp-git",
			Args:            []string{"--repo", "/workspace"},
			NetworkRequires: []string{"git.internal"},
			Permissions:     []string{"read", "write", "execute"},
		},
	}
}

// FindPack looks up a pack by name from the built-in registry.
// Returns nil if not found.
func FindPack(name string) *Manifest {
	for _, p := range BuiltinPacks() {
		if p.Name == name {
			return &p
		}
	}
	return nil
}
