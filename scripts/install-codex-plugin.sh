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
config_dir="${LARK_CONFIG_DIR:-$HOME/.lark}"
binary_path="$bin_dir/lark"
real_binary_path="$bin_dir/lark-real"
config_file="$config_dir/config.yaml"

mkdir -p "$skills_dir" "$bin_dir" "$config_dir"

echo "Building lark CLI..."
(cd "$repo_root" && make build)

install -m 0755 "$repo_root/lark" "$real_binary_path"

cat >"$binary_path" <<EOF
#!/usr/bin/env bash
set -euo pipefail

export LARK_CONFIG_DIR="\${LARK_CONFIG_DIR:-$config_dir}"
exec "$real_binary_path" "\$@"
EOF
chmod 0755 "$binary_path"
echo "Installed binary wrapper to $binary_path"
echo "Installed CLI binary to $real_binary_path"

if [[ ! -f "$config_file" ]]; then
  cp "$repo_root/config.example.yaml" "$config_file"
  echo "Installed config template to $config_file"
fi

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
2. Update $config_file with your App ID and region
3. Set LARK_APP_SECRET in your shell environment
4. Restart Codex so it picks up the new skills
EOF
