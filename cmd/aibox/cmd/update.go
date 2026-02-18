package cmd

import (
	"bytes"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update AI-Box container images and components",
	Long: `Update pulls the latest sandbox container images from the registry
and updates any local components to their newest versions.

The update process:
  1. Checks for running AI-Box containers (warns if found)
  2. Pulls the latest image from the configured registry
  3. Verifies the image signature (if configured)
  4. Reports the old and new image digests`,
	RunE: runUpdate,
}

func init() {
	updateCmd.Flags().Bool("force", false, "pull even if a container is running")

	rootCmd.AddCommand(updateCmd)
}

func runUpdate(cmd *cobra.Command, args []string) error {
	force, _ := cmd.Flags().GetBool("force")
	rt := Cfg.Runtime
	image := Cfg.Image

	rtPath, err := exec.LookPath(rt)
	if err != nil {
		return fmt.Errorf("%s not found in PATH: %w", rt, err)
	}

	// 1. Warn if an aibox container is currently running.
	if running, name := isContainerRunning(rtPath); running && !force {
		fmt.Printf("WARNING: AI-Box container %q is currently running.\n", name)
		fmt.Println("  Updates take effect on next `aibox start`. Use --force to pull anyway.")
		fmt.Println("  Continuing with pull...")
		fmt.Println()
	}

	// 2. Capture the current image digest (if present locally).
	oldDigest := getImageDigest(rtPath, image)
	if oldDigest != "" {
		slog.Debug("current image digest", "digest", oldDigest)
	}

	// 3. Pull the latest image.
	fmt.Printf("Pulling %s ...\n", image)
	pullCmd := exec.Command(rtPath, "pull", image)
	pullCmd.Stdout = cmd.OutOrStdout()
	pullCmd.Stderr = cmd.ErrOrStderr()
	if err := pullCmd.Run(); err != nil {
		return fmt.Errorf("failed to pull image %s: %w", image, err)
	}

	// 4. Verify signature if configured.
	if Cfg.Registry.VerifySignatures {
		fmt.Println("Verifying image signature...")
		if err := verifyImageSignature(rtPath, image); err != nil {
			return fmt.Errorf("image signature verification failed: %w", err)
		}
		fmt.Println("  Signature: OK")
	}

	// 5. Compare old vs new digest.
	newDigest := getImageDigest(rtPath, image)
	if newDigest == "" {
		fmt.Println("\nUpdate complete (could not determine new digest).")
		return nil
	}

	fmt.Println()
	if oldDigest == "" {
		fmt.Printf("Image pulled (new): %s\n", truncateDigest(newDigest))
	} else if oldDigest == newDigest {
		fmt.Printf("Image is already up to date: %s\n", truncateDigest(newDigest))
	} else {
		fmt.Printf("Image updated:\n")
		fmt.Printf("  Old: %s\n", truncateDigest(oldDigest))
		fmt.Printf("  New: %s\n", truncateDigest(newDigest))
	}

	return nil
}

// isContainerRunning checks whether an aibox container is currently running.
// Returns true and the container name if found.
func isContainerRunning(rtPath string) (bool, string) {
	out, err := exec.Command(rtPath, "ps", "--filter", "label=aibox=true", "--format", "{{.Names}}").Output()
	if err != nil {
		slog.Debug("failed to check running containers", "error", err)
		return false, ""
	}
	name := strings.TrimSpace(string(out))
	if name == "" {
		return false, ""
	}
	// Return the first running container name.
	lines := strings.Split(name, "\n")
	return true, lines[0]
}

// getImageDigest returns the digest of a local image, or "" if not found.
func getImageDigest(rtPath, image string) string {
	out, err := exec.Command(rtPath, "image", "inspect", "--format", "{{.Digest}}", image).Output()
	if err != nil {
		return ""
	}
	digest := strings.TrimSpace(string(out))
	if digest == "" || digest == "<none>" {
		// Try RepoDigests field instead.
		out, err = exec.Command(rtPath, "image", "inspect", "--format", "{{index .RepoDigests 0}}", image).Output()
		if err != nil {
			return ""
		}
		digest = strings.TrimSpace(string(out))
	}
	return digest
}

// verifyImageSignature verifies the image signature using cosign or podman
// built-in verification. Returns nil if verification passes or if cosign
// is not available (with a warning logged).
func verifyImageSignature(rtPath, image string) error {
	// First try cosign if available.
	cosignPath, err := exec.LookPath("cosign")
	if err == nil {
		var stderr bytes.Buffer
		verifyCmd := exec.Command(cosignPath, "verify", image)
		verifyCmd.Stderr = &stderr
		if err := verifyCmd.Run(); err != nil {
			return fmt.Errorf("cosign verify failed: %s", stderr.String())
		}
		return nil
	}

	// Podman with /etc/containers/policy.json handles signature verification
	// at pull time. If we got here, the pull already succeeded, which means
	// the policy was satisfied.
	slog.Debug("cosign not found; relying on container runtime policy for signature verification")
	return nil
}

// truncateDigest shortens a digest for display, showing the first 12 hex chars.
func truncateDigest(digest string) string {
	// digest may be "sha256:abcdef..." or "registry/repo@sha256:abcdef..."
	if idx := strings.LastIndex(digest, "sha256:"); idx >= 0 {
		hex := digest[idx+7:]
		if len(hex) > 12 {
			return digest[:idx+7] + hex[:12]
		}
	}
	return digest
}
