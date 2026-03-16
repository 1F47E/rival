package cmd

import (
	"fmt"
	"os/exec"

	"github.com/1F47E/rival/internal/update"
	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update rival to the latest version via Homebrew",
	RunE:  runUpdate,
}

func init() {
	rootCmd.AddCommand(updateCmd)
}

func runUpdate(cmd *cobra.Command, args []string) error {
	current := Version

	// Check latest release from GitHub
	fmt.Print("Checking for updates... ")
	latest, err := update.FetchLatest()
	if err != nil {
		return fmt.Errorf("check latest version: %w", err)
	}

	if latest == current {
		fmt.Printf("already on latest (v%s)\n", current)
		return nil
	}

	fmt.Printf("v%s → v%s\n\n", current, latest)

	// Upgrade via brew
	fmt.Println("Upgrading via Homebrew...")
	brew := exec.CommandContext(cmd.Context(), "brew", "upgrade", "1f47e/tap/rival")
	brew.Stdout = cmd.OutOrStdout()
	brew.Stderr = cmd.ErrOrStderr()
	if err := brew.Run(); err != nil {
		// If brew upgrade fails (e.g. already latest), try reinstall
		fmt.Println("brew upgrade failed, trying reinstall...")
		reinstall := exec.CommandContext(cmd.Context(), "brew", "reinstall", "1f47e/tap/rival")
		reinstall.Stdout = cmd.OutOrStdout()
		reinstall.Stderr = cmd.ErrOrStderr()
		if err := reinstall.Run(); err != nil {
			return fmt.Errorf("brew reinstall: %w", err)
		}
	}

	// Reinstall skills
	fmt.Println("\nUpdating skills...")
	forceInstall = true
	if err := runInstall(cmd, nil); err != nil {
		return fmt.Errorf("install skills: %w", err)
	}

	fmt.Printf("\n✓ Updated to v%s\n", latest)
	return nil
}
