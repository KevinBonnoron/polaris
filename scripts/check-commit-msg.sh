#!/usr/bin/env bash
# Validate commit message: Conventional Commits with a fully lowercase subject.
set -euo pipefail

msg_file="${1:?missing commit message file path}"
subject="$(grep -m1 -vE '^[[:space:]]*(#|$)' "$msg_file" || true)"

# Skip the technical commits git generates itself.
case "$subject" in
  Merge\ * | Revert\ * | fixup!* | squash!* | amend!*) exit 0 ;;
esac

types='feat|fix|refactor|perf|docs|style|test|build|ci|chore|revert'
fail() {
  echo "✗ commit message rejected: $1" >&2
  echo "  subject : $subject" >&2
  echo "  format  : <type>(<scope>)!: <lowercase subject>" >&2
  echo "  types   : ${types//|/, }" >&2
  echo "  example : feat(tickets): add linear provider" >&2
  exit 1
}

[ -n "$subject" ] || fail "empty subject"
echo "$subject" | grep -qE "^(${types})(\([a-z0-9._/-]+\))?!?: .+" \
  || fail "does not follow Conventional Commits"
echo "$subject" | grep -q '[A-Z]' \
  && fail "uppercase letters are not allowed in the subject"

exit 0
