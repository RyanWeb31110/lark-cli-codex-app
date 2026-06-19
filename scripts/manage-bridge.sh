#!/usr/bin/env bash

set -euo pipefail

usage() {
  cat <<'EOF'
Usage: ./scripts/manage-bridge.sh <command>

Manage the local Lark -> Codex app-server bridge on macOS.

Commands:
  install     Write the launchd plist
  start       Start the launchd service
  stop        Stop the launchd service
  restart     Reinstall the plist and restart the service
  status      Print launchd status fields
  logs        Tail the gateway log
  uninstall   Stop the service and remove the launchd plist

Environment overrides:
  LARK_BRIDGE_LABEL              launchd label
  LARK_BIN                       lark wrapper/binary path
  LARK_LAUNCHER                  launcher used by launchd, defaults to /bin/bash
  LARK_CONFIG_DIR                lark config directory
  LARK_AGENT_WORKSPACE           workspace passed to Codex
  LARK_AGENT_BACKEND             app_server or codex_exec
  LARK_AGENT_REASONING_EFFORT    minimal, low, medium, high, or xhigh
  LARK_AGENT_THREAD_BINDINGS     binding JSON path
  LARK_GATEWAY_LOG               gateway log path
EOF
}

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
uid="$(id -u)"

label="${LARK_BRIDGE_LABEL:-com.local.lark-cli-codex-app.gateway}"
lark_bin="${LARK_BIN:-$HOME/.local/bin/lark}"
launcher="${LARK_LAUNCHER:-/bin/bash}"
config_dir="${LARK_CONFIG_DIR:-$HOME/.lark}"
workspace="${LARK_AGENT_WORKSPACE:-$HOME/WorkSpace}"
backend="${LARK_AGENT_BACKEND:-app_server}"
reasoning_effort="${LARK_AGENT_REASONING_EFFORT:-medium}"
thread_bindings="${LARK_AGENT_THREAD_BINDINGS:-$config_dir/codex-thread-bindings.json}"
log_path="${LARK_GATEWAY_LOG:-$config_dir/gateway.log}"
plist_path="$HOME/Library/LaunchAgents/$label.plist"

domain="gui/$uid"
service="$domain/$label"

require_macos() {
  if [[ "$(uname -s)" != "Darwin" ]]; then
    echo "launchd bridge management is only supported on macOS" >&2
    exit 1
  fi
}

write_plist() {
  mkdir -p "$(dirname "$plist_path")" "$config_dir" "$(dirname "$log_path")"

  cat >"$plist_path" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>$label</string>

  <key>ProgramArguments</key>
  <array>
    <string>$launcher</string>
    <string>$lark_bin</string>
    <string>gateway</string>
    <string>serve</string>
    <string>--agent</string>
    <string>--agent-workspace</string>
    <string>$workspace</string>
    <string>--agent-backend</string>
    <string>$backend</string>
    <string>--agent-reasoning-effort</string>
    <string>$reasoning_effort</string>
    <string>--agent-thread-bindings</string>
    <string>$thread_bindings</string>
  </array>

  <key>EnvironmentVariables</key>
  <dict>
    <key>LARK_CONFIG_DIR</key>
    <string>$config_dir</string>
  </dict>

  <key>RunAtLoad</key>
  <true/>

  <key>KeepAlive</key>
  <true/>

  <key>WorkingDirectory</key>
  <string>$repo_root</string>

  <key>StandardOutPath</key>
  <string>$log_path</string>

  <key>StandardErrorPath</key>
  <string>$log_path</string>
</dict>
</plist>
EOF

  echo "Wrote $plist_path"
}

stop_service() {
  launchctl bootout "$domain" "$plist_path" >/dev/null 2>&1 || true
}

start_service() {
  launchctl bootstrap "$domain" "$plist_path" >/dev/null 2>&1 || true
  launchctl enable "$service" >/dev/null 2>&1 || true
  launchctl kickstart -k "$service"
}

status_service() {
  if ! launchctl print "$service" 2>/dev/null | egrep "state =|pid =|last exit code|program =|path =" ; then
    echo "$label is not loaded"
    return 1
  fi
}

command="${1:-}"
case "$command" in
  install)
    require_macos
    write_plist
    ;;
  start)
    require_macos
    if [[ ! -f "$plist_path" ]]; then
      write_plist
    fi
    start_service
    status_service
    ;;
  stop)
    require_macos
    stop_service
    echo "Stopped $label"
    ;;
  restart)
    require_macos
    write_plist
    stop_service
    start_service
    status_service
    ;;
  status)
    require_macos
    status_service
    ;;
  logs)
    lines="${2:-80}"
    if [[ ! -f "$log_path" ]]; then
      echo "Log file does not exist: $log_path" >&2
      exit 1
    fi
    tail -n "$lines" "$log_path"
    ;;
  uninstall)
    require_macos
    stop_service
    rm -f "$plist_path"
    echo "Removed $plist_path"
    ;;
  -h|--help|help|"")
    usage
    ;;
  *)
    echo "Unknown command: $command" >&2
    usage >&2
    exit 1
    ;;
esac
