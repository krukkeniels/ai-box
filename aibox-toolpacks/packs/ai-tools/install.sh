#!/bin/bash
set -euo pipefail

PACK_DIR="/opt/toolpacks/ai-tools"

echo "==> Installing AI tools pack (Claude Code CLI, Codex CLI)"

mkdir -p "${PACK_DIR}/bin"

# Ensure Node.js is available (needed for npm-based installs).
if ! command -v node &>/dev/null; then
    echo "  Node.js not found; installing minimal Node.js..."
    apt-get update -qq
    apt-get install -y -qq curl ca-certificates
    curl -fsSL https://deb.nodesource.com/setup_20.x | bash -
    apt-get install -y -qq nodejs
fi

# Install Claude Code CLI.
if ! command -v claude &>/dev/null; then
    echo "  Installing Claude Code CLI..."
    npm install -g @anthropic-ai/claude-code 2>/dev/null || {
        echo "  Claude Code CLI not available via npm; skipping."
    }
fi

# Install Codex CLI.
if ! command -v codex &>/dev/null; then
    echo "  Installing Codex CLI..."
    npm install -g @openai/codex 2>/dev/null || {
        echo "  Codex CLI not available via npm; skipping."
    }
fi

# Configure AI tools to use the LLM sidecar proxy.
# These environment variables are also set by the container runtime,
# but we ensure they're in the profile for shell sessions.
cat > /etc/profile.d/toolpack-ai-tools.sh << 'EOF'
# AI-Box LLM sidecar proxy configuration.
export ANTHROPIC_BASE_URL="http://localhost:8443"
export OPENAI_BASE_URL="http://localhost:8443"
EOF

# Ensure config directories exist.
mkdir -p "${HOME}/.claude"
mkdir -p "${HOME}/.codex"

echo "==> AI tools pack installed successfully."
echo "  AI tools are configured to route through the LLM sidecar proxy (localhost:8443)."
