#!/usr/bin/env bash

set -euo pipefail

force=0
login=0
no_login_prompt=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --force)
      force=1
      shift
      ;;
    --login)
      login=1
      shift
      ;;
    --no-login-prompt)
      no_login_prompt=1
      shift
      ;;
    -h|--help)
      cat <<'EOF'
Usage: ./scripts/install-codex-plugin.sh [--force] [--login] [--no-login-prompt]

Build the lark CLI and install bundled skills for Codex.

Options:
  --force             Overwrite existing skills with the same names
  --login             Run `lark auth login` after install if OAuth is missing or expired
  --no-login-prompt   Do not prompt for OAuth login in interactive terminals
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
env_file="$config_dir/env.sh"

mkdir -p "$skills_dir" "$bin_dir" "$config_dir"

echo "Building lark CLI..."
(cd "$repo_root" && make build)

install -m 0755 "$repo_root/lark" "$real_binary_path"

cat >"$binary_path" <<EOF
#!/usr/bin/env bash
set -euo pipefail

export LARK_CONFIG_DIR="\${LARK_CONFIG_DIR:-$config_dir}"
if [[ -f "$env_file" ]]; then
  # shellcheck disable=SC1090
  source "$env_file"
fi
bundled_codex="\${CODEX_HOME:-\$HOME/.codex}/plugins/.plugin-appserver/codex"
case "\${LARK_AGENT_CODEX_BINARY:-}" in
  ""|"codex"|"/opt/homebrew/bin/codex"|"\$HOME/bin/codex"|"\$HOME/.local/bin/codex")
    if [[ -x "\$bundled_codex" ]]; then
      export LARK_AGENT_CODEX_BINARY="\$bundled_codex"
    fi
    ;;
esac
exec "$real_binary_path" "\$@"
EOF
chmod 0755 "$binary_path"
echo "Installed binary wrapper to $binary_path"
echo "Installed CLI binary to $real_binary_path"

if [[ ! -f "$config_file" ]]; then
  cp "$repo_root/config.example.yaml" "$config_file"
  echo "Installed config template to $config_file"
fi

if [[ ! -f "$env_file" ]]; then
  cat >"$env_file" <<'EOF'
#!/usr/bin/env bash

# Set your Lark or Feishu app secret here.
# export LARK_APP_SECRET="your_app_secret"
EOF
  chmod 0600 "$env_file"
  echo "Installed env template to $env_file"
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

auth_hint=""
auth_needed=0
auth_status="$("$binary_path" auth status 2>/dev/null || true)"
if [[ "$auth_status" == *'"authenticated": true'* ]]; then
  auth_hint="OAuth status: authenticated."
else
  auth_needed=1
  auth_hint="OAuth status: not authenticated or expired. Run: $binary_path auth login"
fi

if [[ "$auth_needed" -eq 1 && "$login" -eq 0 && "$no_login_prompt" -eq 0 && -t 0 && -t 1 ]]; then
  read -r -p "OAuth is missing or expired. Run '$binary_path auth login' now? [y/N] " answer
  case "$answer" in
    y|Y|yes|YES)
      login=1
      ;;
  esac
fi

if [[ "$auth_needed" -eq 1 && "$login" -eq 1 ]]; then
  echo "Starting OAuth login..."
  "$binary_path" auth login
  auth_status="$("$binary_path" auth status 2>/dev/null || true)"
  if [[ "$auth_status" == *'"authenticated": true'* ]]; then
    auth_hint="OAuth status: authenticated."
  else
    auth_hint="OAuth status: still not authenticated. Run again when ready: $binary_path auth login"
  fi
fi

cat <<EOF

Done.

$auth_hint

Next steps:
1. Ensure $bin_dir is in your PATH
2. Update $config_file with your App ID and region
3. Set LARK_APP_SECRET in $env_file
4. Run $binary_path auth login for user-scoped APIs such as docs, calendar, message history, sheets, and mail
5. Restart Codex so it picks up the new skills
6. Optional: run ./scripts/manage-bridge.sh restart to install and start the local Lark -> Codex bridge
EOF
