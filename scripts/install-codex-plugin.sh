#!/usr/bin/env bash

set -euo pipefail

force=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --force)
      force=1
      shift
      ;;
    -h|--help)
      cat <<'EOF'
Usage: ./scripts/install-codex-plugin.sh [--force]

Build the lark CLI and install bundled skills for Codex.

Options:
  --force    Overwrite existing skills with the same names
EOF
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      exit 1
      ;;
  esac
done

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
codex_home="${CODEX_HOME:-$HOME/.codex}"
skills_dir="${codex_home}/skills"
bin_dir="${LARK_INSTALL_DIR:-$HOME/.local/bin}"

mkdir -p "$skills_dir" "$bin_dir"

echo "Building lark CLI..."
(cd "$repo_root" && make build)

install -m 0755 "$repo_root/lark" "$bin_dir/lark"
echo "Installed binary to $bin_dir/lark"

for skill_path in "$repo_root"/skills/*; do
  skill_name="$(basename "$skill_path")"
  dest_path="$skills_dir/$skill_name"

  if [[ -e "$dest_path" && "$force" -ne 1 ]]; then
    echo "Skipping existing skill $skill_name (use --force to overwrite)"
    continue
  fi

  rm -rf "$dest_path"
  cp -R "$skill_path" "$dest_path"
  echo "Installed skill $skill_name -> $dest_path"
done

cat <<EOF

Done.

Next steps:
1. Ensure $bin_dir is in your PATH
2. Configure .lark/config.yaml and LARK_APP_SECRET
3. Restart Codex so it picks up the new skills
EOF
