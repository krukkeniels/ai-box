package cmd

import (
	"fmt"
	"os/exec"

	"github.com/aibox/aibox/internal/mounts"
	"github.com/spf13/cobra"
)

var repairCmd = &cobra.Command{
	Use:   "repair",
	Short: "Repair AI-Box resources",
	Long:  `Repair provides subcommands for fixing common issues with AI-Box.`,
}

var repairCacheCmd = &cobra.Command{
	Use:   "cache",
	Short: "Clear and rebuild build cache volumes",
	Long: `Remove all AI-Box build cache volumes (Maven, Gradle, npm, Yarn, Bazel)
and recreate them. This does NOT remove the persistent home directory
or toolpack volumes.

Use this when build caches are corrupted or you want a clean slate.`,
	RunE: runRepairCache,
}

func init() {
	repairCmd.AddCommand(repairCacheCmd)
	rootCmd.AddCommand(repairCmd)
}

func runRepairCache(cmd *cobra.Command, args []string) error {
	rt := Cfg.Runtime
	rtPath, err := exec.LookPath(rt)
	if err != nil {
		return fmt.Errorf("%s not found in PATH: %w", rt, err)
	}

	prefix, err := mounts.VolumePrefix()
	if err != nil {
		return fmt.Errorf("determining volume prefix: %w", err)
	}

	// Check for running containers first.
	if running, name := isContainerRunning(rtPath); running {
		return fmt.Errorf("cannot repair caches while container %q is running; run `aibox stop` first", name)
	}

	fmt.Println("Removing build cache volumes...")
	if err := mounts.RemoveCacheVolumes(rtPath, prefix); err != nil {
		return fmt.Errorf("removing cache volumes: %w", err)
	}

	fmt.Println("Recreating cache volumes...")
	cacheMounts := make([]mounts.Mount, 0)
	for _, cv := range mounts.CacheVolumes(prefix) {
		cacheMounts = append(cacheMounts, mounts.Mount{
			Type:   "volume",
			Source: cv.VolumeName,
			Target: cv.ContainerPath,
		})
	}
	if err := mounts.EnsureVolumes(rtPath, cacheMounts); err != nil {
		return fmt.Errorf("recreating cache volumes: %w", err)
	}

	fmt.Println("Cache volumes repaired successfully.")
	return nil
}
