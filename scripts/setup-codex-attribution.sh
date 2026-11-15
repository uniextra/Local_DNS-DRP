#!/usr/bin/env bash
set -euo pipefail

repo_root=$(git rev-parse --show-toplevel)
cd "$repo_root"

git config --local core.hooksPath .githooks
mkdir -p .githooks
chmod +x .githooks/prepare-commit-msg

echo "Codex co-author hook installed for this repository."
echo "Configured trailer: Co-authored-by: codex <codex@openai.com>"
