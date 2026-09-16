package cmd

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"devexp/internal/assets"
	"devexp/internal/hooks"
	"devexp/internal/manifest"
	"devexp/internal/ui"
)

// ── Uninstall (hidden; used by uninstall.sh) ──────────────────────────────────
//
// uninstall.sh removes agents, skills and MCPs itself, but the rules for what
// devexp may delete from a CLI's plugin directory live here, in the same code
// the installer uses, so the two can never disagree. The command is hidden: it
// is a helper for the script, not a user-facing way to uninstall.
//
// It never calls repo.Resolve: on a version mismatch that wipes the embedded
// asset cache, and uninstall.sh may be running from that very cache.

// uninstallTargets maps each --target value to its handler. Adding a CLI is
// adding an entry; the flag's usage text lists the keys, and uninstall.sh
// probes that text to know a binary supports a target.
var uninstallTargets = map[string]func(home string, dryRun bool) error{
	"opencode": doUninstallOpencode, // hook plugin + legacy flat install only
}

var (
	flagUninstallTarget string
	flagUninstallDryRun bool
	flagUninstallYes    bool
)

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove devexp from a CLI target (used by uninstall.sh)",
	Long: `Remove devexp from a CLI target. This is a helper for uninstall.sh, which
removes agents, skills and MCP servers itself.

  opencode  removes the hook plugin (plugins/devexp.js and plugins/devexp/)
            and the legacy flat plugin install, by the installer's own rules.`,
	Hidden:        true,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE:          runUninstall,
}

func init() {
	f := uninstallCmd.Flags()
	f.StringVar(&flagUninstallTarget, "target", "", "CLI to remove devexp from ("+supportedUninstallTargets()+")")
	f.BoolVarP(&flagUninstallDryRun, "dry-run", "n", false, "Preview without making changes")
	f.BoolVarP(&flagUninstallYes, "yes", "y", false, "Don't prompt (accepted for scripts; the command never prompts)")
	rootCmd.AddCommand(uninstallCmd)
}

// supportedUninstallTargets renders the target list, e.g. "supported: opencode".
func supportedUninstallTargets() string {
	names := make([]string, 0, len(uninstallTargets))
	for n := range uninstallTargets {
		names = append(names, n)
	}
	sort.Strings(names)
	return "supported: " + strings.Join(names, ", ")
}

func runUninstall(cmd *cobra.Command, args []string) error {
	handler, ok := uninstallTargets[flagUninstallTarget]
	if !ok {
		return fmt.Errorf("unsupported target %q (%s)", flagUninstallTarget, supportedUninstallTargets())
	}
	home, err := targetHome(os.Getenv("HOME"))
	if err != nil {
		return fmt.Errorf("%w — refusing to remove anything; set HOME and re-run", err)
	}
	return handler(home, flagUninstallDryRun)
}

// doUninstallOpencode removes devexp's opencode hook plugin and whatever the
// legacy flat install left behind. Agents, commands and MCP servers are
// uninstall.sh's job.
//
// Warnings (a kept entry, a symlinked plugins/) don't fail the run: what is
// kept stays recorded in the manifest. An error means the plugin roots were
// refused and nothing was removed, legacy files and config.json included.
func doUninstallOpencode(home string, dryRun bool) error {
	p, err := opencodeTargetPaths(home)
	if err != nil {
		return err
	}

	// Only a regular file counts as an existing manifest. A symlink (dangling
	// or not) is never written through: saving would create or change a file
	// somewhere else, such as a dotfiles checkout.
	manifestInfo, statErr := os.Lstat(p.manifest)
	old, loadErr := manifest.Load(p.manifest)
	if loadErr != nil {
		ui.Warn(fmt.Sprintf("manifest %s is unreadable, so plugin files are identified from disk only: %v", p.manifest, loadErr))
	}

	registry, err := uninstallHookRegistry()
	if err != nil {
		ui.Warn(fmt.Sprintf("hooks registry: %v — hook modules are identified from the manifest only", err))
		registry = nil
	}

	ui.Info(fmt.Sprintf("Removing hooks (opencode plugin → %s)...", p.plugins))
	kept, err := hooks.UninstallOpencode(registry, p.plugins, old.Plugins, dryRun)
	if err != nil {
		return fmt.Errorf("opencode plugin left untouched: %w", err)
	}
	if err := hooks.CleanLegacyOpencode(p.plugins, p.config, dryRun); err != nil {
		ui.Warn(fmt.Sprintf("legacy opencode plugin cleanup: %v", err))
	}

	// Record only what had to stay, so a devexp.js a user later puts there is
	// never taken for devexp's. Never on a dry run, never into a manifest that
	// didn't exist, never over one that failed to load, and never through a
	// symlink.
	if dryRun || statErr != nil || loadErr != nil || slices.Equal(old.Plugins, kept) {
		return nil
	}
	if !manifestInfo.Mode().IsRegular() {
		ui.Warn(fmt.Sprintf("manifest %s is a symlink, so it was left untouched — its plugins list no longer matches what is on disk", p.manifest))
		return nil
	}
	old.Plugins = kept
	if err := manifest.Save(p.manifest, old); err != nil {
		ui.Warn(fmt.Sprintf("save manifest: %v", err))
	}
	return nil
}

// uninstallHookRegistry reads the hooks registry from DEVEXP_DIR when
// uninstall.sh passes it, else from the assets embedded in this binary.
func uninstallHookRegistry() (hooks.Registry, error) {
	if dir := os.Getenv("DEVEXP_DIR"); dir != "" {
		return hooks.LoadRegistry(filepath.Join(dir, "hooks", "registry.json"))
	}
	data, err := fs.ReadFile(assets.FS, "hooks/registry.json")
	if err != nil {
		return nil, err
	}
	return hooks.ParseRegistry(data)
}
