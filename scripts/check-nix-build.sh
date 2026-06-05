#!/usr/bin/env bash
# Ensure the nix build passes before pushing. When a dependency change leaves a
# fixed-output hash stale (bunDeps.outputHash or vendorHash), rewrite flake.nix
# with the correct value, commit it, and abort the push so it gets included.
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

if ! command -v nix >/dev/null 2>&1; then
  echo "nix not found, skipping nix build check" >&2
  exit 0
fi

nix_flags=(--extra-experimental-features "nix-command flakes" --no-link --print-build-logs)
max_attempts=5
updated=0

for ((attempt = 1; attempt <= max_attempts; attempt++)); do
  echo "==> nix build .#default (attempt ${attempt}/${max_attempts})"

  if log="$(nix build .#default "${nix_flags[@]}" 2>&1)"; then
    if [ "$updated" -eq 1 ]; then
      echo "==> flake.nix hashes were stale; corrected them"
      git commit --only flake.nix --no-verify -m "chore: update nix hashes for dependency changes"
      cat >&2 <<'EOF'

flake.nix was out of date with the current dependencies.
A commit with the corrected hashes has been created.
Re-run your push to include it.
EOF
      exit 1
    fi
    echo "==> nix build OK"
    exit 0
  fi

  specified="$(printf '%s\n' "$log" | grep -oP '(?:specified|wanted):\s+\Ksha\d+-\S+' | head -n1 || true)"
  got="$(printf '%s\n' "$log" | grep -oP '\bgot:\s+\Ksha\d+-\S+' | head -n1 || true)"

  if [ -n "$specified" ] && [ -n "$got" ] && [ "$specified" != "$got" ]; then
    echo "    hash mismatch: ${specified} -> ${got}"
    if ! grep -qF "$specified" flake.nix; then
      printf '%s\n' "$log" >&2
      echo "ERROR: stale hash ${specified} not found in flake.nix; fix manually." >&2
      exit 1
    fi
    sed -i "s|${specified}|${got}|" flake.nix
    updated=1
    continue
  fi

  printf '%s\n' "$log" >&2
  echo "ERROR: nix build failed and it is not a hash mismatch; fix it before pushing." >&2
  exit 1
done

echo "ERROR: nix build still failing after ${max_attempts} attempts." >&2
exit 1
