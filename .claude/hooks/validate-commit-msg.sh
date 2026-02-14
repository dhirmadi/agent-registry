#!/bin/bash
# Pre-commit hook: validate Conventional Commits format
# Used by Claude Code hooks (PreToolUse on Bash)

INPUT=$(cat)
COMMAND=$(echo "$INPUT" | jq -r '.tool_input.command // empty')

# Only intercept git commit commands
if [[ ! "$COMMAND" =~ git\ commit ]]; then
  exit 0
fi

# Extract commit message from -m flag (handles heredoc and quoted forms)
COMMIT_MSG=$(echo "$COMMAND" | grep -oP '(?<=-m\s")[^"]+|(?<=-m\s'\'')[^'\'']+' | head -1)

# If using heredoc (cat <<), extract the first meaningful line
if [ -z "$COMMIT_MSG" ]; then
  COMMIT_MSG=$(echo "$COMMAND" | sed -n '/cat <<.*EOF/,/EOF/p' | grep -v 'cat <<' | grep -v 'EOF' | head -1 | sed 's/^[[:space:]]*//')
fi

# Skip validation if we couldn't extract a message (e.g., --amend without -m)
if [ -z "$COMMIT_MSG" ]; then
  exit 0
fi

# Validate Conventional Commits: type(scope): description
if ! echo "$COMMIT_MSG" | grep -qP '^(feat|fix|test|refactor|docs|chore|ci|perf)(\([a-z0-9-]+\))?: .+'; then
  echo "Commit message does not follow Conventional Commits format:" >&2
  echo "  Expected: <type>(<scope>): <description>" >&2
  echo "  Types: feat, fix, test, refactor, docs, chore, ci, perf" >&2
  echo "  Got: $COMMIT_MSG" >&2
  exit 2
fi

exit 0
