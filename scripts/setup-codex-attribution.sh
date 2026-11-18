#!/usr/bin/env bash
set -euo pipefail

repo_root=$(git rev-parse --show-toplevel)
cd "$repo_root"

git config --local core.hooksPath .githooks
git config --local commit.template "$repo_root/.gitmessage"

git config --local --unset-all ai.coauthor >/dev/null 2>&1 || true
git config --local --add ai.coauthor "Codex <noreply@openai.com>"

chmod +x .githooks/prepare-commit-msg

echo "Configured repository-local Codex co-author attribution."
echo "Current co-authors:"
git config --get-all ai.coauthor
