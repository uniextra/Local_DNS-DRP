# Codex commit attribution

This repository includes a Git `prepare-commit-msg` hook based on
[ sample-codex-coauthor ](https://github.com/realsigridjin/sample-codex-coauthor).

## Install in a local clone

Run once from the repository root:

```bash
./scripts/setup-codex-attribution.sh
```

The script configures this clone to use `.githooks`. Future normal commits
automatically receive this trailer:

```text
Co-authored-by: codex <codex@openai.com>
```

The hook skips merge commits and avoids duplicate trailers. To bypass it for
one commit, set `SKIP_AI_COAUTHORS=1`.

Git hooks are local executable files. They are not activated merely by cloning
a repository, so each local clone must run the setup script once.

## GitHub attribution

The trailer must use the exact name and email associated with the Codex
GitHub identity. GitHub only links a co-author when the trailer email maps
to a GitHub account. The trailer being present in a commit message is
necessary, but does not guarantee GitHub will display a co-author unless
that email is recognized by GitHub.
