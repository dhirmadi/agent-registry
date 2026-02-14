#!/bin/bash
# Pre-commit hook: run go vet before allowing git commit
# Used by Claude Code hooks (PreToolUse on Bash)

INPUT=$(cat)
COMMAND=$(echo "$INPUT" | jq -r '.tool_input.command // empty')

# Only intercept git commit commands
if [[ ! "$COMMAND" =~ git\ commit ]]; then
  exit 0
fi

# Run go vet
cd "$CLAUDE_PROJECT_DIR" || exit 0
OUTPUT=$(go vet ./... 2>&1)
if [ $? -ne 0 ]; then
  echo "go vet failed — fix before committing:" >&2
  echo "$OUTPUT" >&2
  exit 2
fi

exit 0
