---
name: lark-bridge
description: Manage the local Lark to Codex bridge - install/start/stop/restart/status the launchd gateway, inspect logs, and explain how Lark messages are routed to Codex app-server.
---

# Lark Codex Bridge

Use this skill when the user asks to set up, start, stop, restart, inspect, or debug the local Lark/Feishu bridge that sends bot messages into Codex.

## What The Bridge Does

The bridge keeps a local Lark/Feishu WebSocket connection open and listens for bot message events. When agent mode is enabled, normal text messages are dispatched to the local Codex app-server backend and the result is replied back to the originating Lark thread.

The bridge is local-first:

- No public HTTPS callback URL is required when using gateway mode.
- The Lark/Feishu app still needs WebSocket event subscriptions enabled.
- The local process must keep running on the Mac that owns the workspace.
- Codex tasks run through `codex app-server` by default.
- `codex_exec` remains available as a fallback backend.

## Common Commands

Run commands from the repository root unless the user has installed the scripts somewhere else.

Install the CLI and bundled skills:

```bash
./scripts/install-codex-plugin.sh
```

Install or refresh the launchd plist:

```bash
./scripts/manage-bridge.sh install
```

Start the bridge:

```bash
./scripts/manage-bridge.sh start
```

Restart the bridge after code or config changes:

```bash
./scripts/manage-bridge.sh restart
```

Check bridge status:

```bash
./scripts/manage-bridge.sh status
```

Inspect recent bridge logs:

```bash
./scripts/manage-bridge.sh logs
```

Stop the bridge:

```bash
./scripts/manage-bridge.sh stop
```

Remove the launchd plist:

```bash
./scripts/manage-bridge.sh uninstall
```

## Useful Overrides

The management script accepts environment overrides:

```bash
LARK_AGENT_WORKSPACE="$HOME/WorkSpace" ./scripts/manage-bridge.sh restart
LARK_AGENT_BACKEND=codex_exec ./scripts/manage-bridge.sh restart
LARK_AGENT_REASONING_EFFORT=low ./scripts/manage-bridge.sh restart
LARK_CONFIG_DIR="$HOME/.lark-work" ./scripts/manage-bridge.sh restart
```

Prefer `app_server` for normal use. Use `codex_exec` only when the app-server bridge is unavailable or being debugged.

## Lark Control Commands

When the gateway agent is enabled, send these commands directly in Lark:

```text
#status
#bind 1
#new
#reset
```

- `#status` shows the current Lark chat/thread connection. When unbound, it lists recent connectable Codex sessions with summaries.
- `#bind 1` binds the current Lark chat/thread to the first session shown by `#status`. Advanced users can still pass `#bind <codex_thread_id>`.
- `#new` creates a new persistent Codex thread and binds the current Lark chat/thread to it.
- `#reset` clears the current Lark chat/thread binding.

This does not remote-control the Codex App UI input box. It routes Lark messages to the selected underlying Codex thread through `codex app-server`. The app-server input should stay short and human-readable, usually `来自飞书消息` plus the original Lark text and a brief note that the current Codex App window may not refresh live, so Codex App history is not polluted by internal routing prompts and the agent does not promise live UI echo.

## Configuration Checklist

Before starting the bridge, confirm:

- `~/.lark/config.yaml` contains the Lark or Feishu app ID and region.
- `~/.lark/env.sh` exports `LARK_APP_SECRET`, or the secret is already available in the service environment.
- The Lark/Feishu app has WebSocket event subscriptions enabled.
- The app subscribes to `im.message.receive_v1`.
- The bot is added to the target chat.
- `lark auth login` has been run if the task needs user-scoped APIs.

## Debugging

If Lark receives the acknowledgement but no final Codex result:

1. Run `./scripts/manage-bridge.sh status`.
2. Run `./scripts/manage-bridge.sh logs 120`.
3. Check whether the log shows `agent_backend: app_server`.
4. If app-server is failing, retry with `LARK_AGENT_BACKEND=codex_exec ./scripts/manage-bridge.sh restart`.

If the bridge does not receive messages:

1. Confirm the launchd service is running.
2. Confirm the Lark/Feishu app region matches `region` in `config.yaml`.
3. Confirm the app uses WebSocket event subscription mode.
4. Confirm the bot is in the chat.
