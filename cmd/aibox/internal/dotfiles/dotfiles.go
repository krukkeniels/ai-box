package dotfiles

import (
	"bytes"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
)

// SyncOpts holds the options for syncing dotfiles into a running container.
type SyncOpts struct {
	RuntimePath   string // path to podman/docker binary
	ContainerName string // name of the running container
	RepoURL       string // git URL of the dotfiles repo
	Shell         string // default shell: bash, zsh, or pwsh
}

// Sync clones or updates the dotfiles repo in the container's persistent home
// volume, then runs install.sh/Makefile or symlinks standard dotfiles.
// It also configures the default shell and sources aibox-env.sh.
func Sync(opts SyncOpts) error {
	if opts.RepoURL == "" {
		slog.Debug("no dotfiles repo configured, skipping sync")
		return nil
	}

	slog.Info("syncing dotfiles", "repo", opts.RepoURL, "container", opts.ContainerName)

	// Clone or pull the dotfiles repo into /home/dev/.dotfiles.
	cloneScript := fmt.Sprintf(`
set -e
DOTFILES_DIR=/home/dev/.dotfiles
if [ -d "$DOTFILES_DIR/.git" ]; then
    cd "$DOTFILES_DIR" && git pull --ff-only 2>/dev/null || true
    echo "dotfiles: updated"
else
    git clone '%s' "$DOTFILES_DIR" 2>/dev/null
    echo "dotfiles: cloned"
fi
`, opts.RepoURL)

	if err := execInContainer(opts.RuntimePath, opts.ContainerName, cloneScript); err != nil {
		return fmt.Errorf("cloning dotfiles: %w", err)
	}

	// Run install.sh or Makefile if present, otherwise symlink standard dotfiles.
	installScript := `
set -e
DOTFILES_DIR=/home/dev/.dotfiles
cd "$DOTFILES_DIR"

if [ -x install.sh ]; then
    echo "dotfiles: running install.sh"
    ./install.sh
elif [ -f Makefile ]; then
    echo "dotfiles: running make"
    make
else
    echo "dotfiles: symlinking standard dotfiles"
    for f in .bashrc .zshrc .vimrc .gitconfig .tmux.conf; do
        [ -f "$DOTFILES_DIR/$f" ] && ln -sf "$DOTFILES_DIR/$f" "/home/dev/$f"
    done
fi
`
	if err := execInContainer(opts.RuntimePath, opts.ContainerName, installScript); err != nil {
		slog.Warn("dotfiles install failed; dotfiles repo may not have standard structure", "error", err)
	}

	// Write aibox-env.sh and ensure it is sourced last in shell rc files.
	if err := writeAiboxEnv(opts.RuntimePath, opts.ContainerName); err != nil {
		slog.Warn("failed to write aibox-env.sh", "error", err)
	}

	// Configure default shell.
	if opts.Shell != "" && opts.Shell != "bash" {
		if err := setDefaultShell(opts.RuntimePath, opts.ContainerName, opts.Shell); err != nil {
			slog.Warn("failed to set default shell", "shell", opts.Shell, "error", err)
		}
	}

	return nil
}

// writeAiboxEnv creates /etc/profile.d/aibox-env.sh and ensures it's sourced
// last in .bashrc and .zshrc. This sets proxy vars and PATH entries that must
// override user dotfiles.
func writeAiboxEnv(rtPath, container string) error {
	script := `
cat > /tmp/aibox-env.sh << 'ENVEOF'
# AI-Box environment (sourced last to ensure correct proxy and PATH settings)
# This file is managed by aibox - do not edit manually.

# Ensure proxy environment variables are set (nftables enforces these anyway).
export http_proxy="${http_proxy:-http://127.0.0.1:3128}"
export https_proxy="${https_proxy:-http://127.0.0.1:3128}"
export HTTP_PROXY="${HTTP_PROXY:-http://127.0.0.1:3128}"
export HTTPS_PROXY="${HTTPS_PROXY:-http://127.0.0.1:3128}"
export no_proxy="${no_proxy:-localhost,127.0.0.1}"
export NO_PROXY="${NO_PROXY:-localhost,127.0.0.1}"

# AI tool base URLs for LLM sidecar proxy.
export ANTHROPIC_BASE_URL="${ANTHROPIC_BASE_URL:-http://localhost:8443}"
export OPENAI_BASE_URL="${OPENAI_BASE_URL:-http://localhost:8443}"

# Ensure toolpacks are on PATH.
if [ -d /opt/toolpacks ]; then
    for d in /opt/toolpacks/*/bin; do
        [ -d "$d" ] && export PATH="$d:$PATH"
    done
fi

# Standard locale and editor.
export LANG="${LANG:-en_US.UTF-8}"
export EDITOR="${EDITOR:-vim}"
export TERM="${TERM:-xterm-256color}"
ENVEOF

# Install as system-wide profile script.
sudo cp /tmp/aibox-env.sh /etc/profile.d/99-aibox-env.sh 2>/dev/null || \
    cp /tmp/aibox-env.sh /home/dev/.aibox-env.sh
rm -f /tmp/aibox-env.sh

# Ensure it's sourced in .bashrc and .zshrc (last line).
for rc in /home/dev/.bashrc /home/dev/.zshrc; do
    if [ -f "$rc" ]; then
        grep -q 'aibox-env' "$rc" 2>/dev/null || \
            echo '[ -f /etc/profile.d/99-aibox-env.sh ] && . /etc/profile.d/99-aibox-env.sh' >> "$rc"
    fi
done
`
	return execInContainer(rtPath, container, script)
}

// setDefaultShell changes the default shell for the dev user.
func setDefaultShell(rtPath, container, shell string) error {
	shellPath := "/bin/" + shell
	script := fmt.Sprintf(`
if [ -x %s ]; then
    sudo chsh -s %s dev 2>/dev/null || true
    echo "default shell set to %s"
fi
`, shellPath, shellPath, shell)
	return execInContainer(rtPath, container, script)
}

// execInContainer runs a bash script inside the container.
func execInContainer(rtPath, container, script string) error {
	cmd := exec.Command(rtPath, "exec", container, "/bin/bash", "-c", script)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("%w: %s", err, stderr.String())
	}
	output := strings.TrimSpace(string(out))
	if output != "" {
		slog.Debug("container exec output", "output", output)
	}
	return nil
}
