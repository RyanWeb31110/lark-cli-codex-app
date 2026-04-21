# lark-cli-codex-app

A Codex-friendly fork of [`yjwong/lark-cli`](https://github.com/yjwong/lark-cli): a CLI for interacting with Lark/Feishu APIs, bundled with reusable AI assistant skills.

This fork keeps the upstream Go CLI and skill definitions, then adds the metadata and installation flow needed to publish it as a Codex-oriented open source plugin project.

## Why This Tool?

The official Lark MCP server exists, but its tools are not token-efficient. Each tool call returns verbose responses that consume significant context window space when used with AI assistants.

This CLI addresses that by:

- **Returning compact JSON** - Structured output optimized for programmatic consumption
- **Providing markdown conversion** - Documents are converted to markdown (~2-3x smaller than raw block structures)
- **Supporting selective queries** - Fetch only what you need (e.g., just event IDs, just document titles)

The result: AI assistants can interact with Lark using fewer tokens, leaving more context for actual work.

## Features

- **Calendar** - List, create, update, delete events; check availability; find common free time; RSVP
- **Contacts** - Look up users by ID, search by name, list department members
- **Documents** - Read documents as markdown, list folders, resolve wiki nodes, get comments
- **Messages** - Retrieve chat history, download attachments, send messages, add/list/remove reactions
- **Gateway** - Receive Feishu/Lark bot message events locally through WebSocket long connections, optionally auto-reply
- **Agent Bridge** - Dispatch inbound Feishu messages to local `codex exec` tasks and send results back to chat
- **Desktop Queue** - Route Feishu desktop-operation requests into a local desktop-GUI task queue for this Codex desktop thread
- **Webhook** - Optional fallback for event subscriptions when you explicitly want callback mode
- **Mail** - Read and search emails via IMAP with local caching
- **Minutes** - Get meeting recording metadata, export transcripts, download media
- **Sheets** - Read spreadsheet metadata and content from Lark Sheets
- **Bitable** - Query records and metadata from Lark Bitable

## What This Fork Adds

- A Codex plugin manifest at [`.codex-plugin/plugin.json`](.codex-plugin/plugin.json)
- A Codex install helper at [`scripts/install-codex-plugin.sh`](scripts/install-codex-plugin.sh)
- README guidance for installing the bundled skills into Codex or Claude Code

## Quick Start

1. Create a Lark app at https://open.larksuite.com (or Feishu app at https://open.feishu.cn) with appropriate permissions
2. Copy `config.example.yaml` to `.lark/config.yaml` and add your App ID
3. Set `region` in `.lark/config.yaml` to `lark` (default) or `feishu`
4. Set `LARK_APP_SECRET` environment variable
5. Run `./lark auth login` to authenticate
6. Start using: `./lark cal list --week`

See [USAGE.md](USAGE.md) for full documentation.

## Building

```bash
make build    # Build binary to ./lark
make test     # Run tests
make install  # Install to $GOPATH/bin
```

## Usage with Codex

This repository can be used as a Codex plugin project because it includes:

- A plugin manifest in `.codex-plugin/plugin.json`
- Bundled skills in `skills/`
- A local install helper for copying those skills into your Codex home

### Install for Codex

Build the CLI and install the bundled skills into your local Codex home:

```bash
./scripts/install-codex-plugin.sh
```

By default this will:

- Build `lark` into `./lark`
- Install the binary into `~/.local/bin/lark` with a wrapper that loads `~/.lark/env.sh`
- Copy the bundled skills into `${CODEX_HOME:-~/.codex}/skills`

If a skill already exists, the installer leaves it alone unless you pass `--force`.

After installing, restart Codex so it picks up the new skills.

## Gateway Mode

This fork now includes a local gateway that uses Feishu/Lark WebSocket event subscriptions:

```bash
lark gateway serve
```

Useful flags:

```bash
lark gateway serve \
  --event-log ~/.lark/gateway-events.jsonl \
  --auto-reply-text "收到：{{text}}"
```

Or enable the local task agent:

```bash
lark gateway serve \
  --agent \
  --agent-workspace ~/WorkSpace
```

Desktop GUI tasks use a separate queue and are processed by the built-in local desktop worker. The `/gui ` prefix is still supported, but no longer required. Plain desktop requests such as the following will also be detected automatically:

```text
打开 Safari，然后访问 openai.com
```

The gateway will queue that request and acknowledge it with a task id instead of sending it to `codex exec`.

What it does:

- Opens an outbound WebSocket connection to Feishu/Lark
- Receives `im.message.receive_v1` events without any public callback URL
- Appends incoming message events to a local JSONL file
- Optionally replies to incoming messages using the bot
- Optionally dispatches inbound messages to local `codex exec` tasks and replies with the result
- Routes explicit `/gui ...` messages or detected desktop-operation requests into a dedicated local desktop task queue
- Runs a local desktop worker that automatically picks up queued GUI tasks

Typical setup:

1. In the Feishu/Lark app console, enable event subscriptions with **persistent connection / WebSocket** mode.
2. Subscribe to the message receive event (`im.message.receive_v1` / receive message).
3. Make sure the bot is added to the target chat.
4. Run `lark gateway serve` locally.

If you want the bot to trigger local Codex tasks instead of behaving like a plain echo bot, enable the `agent` section in `config.yaml` or start with `--agent`.

Current limitation:

- Opening apps and links works directly.
- Keyboard-driven GUI actions may require granting macOS Accessibility permission to the process running `osascript`. Without that permission, the worker falls back to opening the app and replying with the computed result when possible.

### Desktop Queue Helpers

The desktop queue can be inspected and driven with:

```bash
lark desktop tasks pop
lark desktop tasks complete --id <task-id> --result "done" --reply
lark desktop tasks fail --id <task-id> --error "why" --reply
```

This is the recommended local development path because it does not require a public HTTPS tunnel.

## Webhook Mode

This fork still includes a local webhook server for Feishu/Lark event subscriptions when you explicitly need callback mode:

```bash
lark webhook serve
```

Useful flags:

```bash
lark webhook serve \
  --listen 0.0.0.0:8080 \
  --path /webhook/feishu \
  --token your-verification-token \
  --auto-reply-text "收到：{{text}}"
```

What it does:

- Handles URL verification (`challenge`) callbacks
- Accepts plaintext `im.message.receive_v1` events
- Appends incoming message events to a local JSONL file
- Optionally replies to incoming messages using the bot

Current limitation:

- Encrypted callbacks are not supported yet, so leave the Feishu/Lark **Encrypt Key** blank for this version

Typical setup:

1. Expose your local server through a public HTTPS tunnel or reverse proxy.
2. In the Feishu/Lark app console, set the request URL to your public URL plus the configured webhook path.
3. If you set a verification token in the app console, pass the same value through `webhook.verification_token` or `--token`.
4. Subscribe to the message receive event (`im.message.receive_v1` / receive message).
5. Add the bot to the target chat.

## Usage with Claude Code

This tool is designed to be invoked via Claude Code skills. Pre-built skill definitions are included in the `skills/` directory.

### Installing Skills

Copy the skill directories to your assistant skills location:

```bash
# Codex user-wide
cp -r skills/* ~/.codex/skills/

# Claude Code project-specific
cp -r skills/* /path/to/your/project/.claude/skills/

# Claude Code user-wide
cp -r skills/* ~/.claude/skills/
```

Available skills:
- `bitable` - Read Bitable app metadata and records
- `calendar` - Manage calendar events, check availability, RSVP
- `contacts` - Look up users and departments
- `documents` - Read documents, list folders, browse wikis
- `messages` - Retrieve chat history, download attachments, send messages to users and chats
- `email` - Read and search emails via IMAP with local caching
- `minutes` - Get meeting recordings, export transcripts, download media
- `sheets` - Inspect Lark Sheets data

### Configuration

The skills assume `lark` is in your PATH. If not, you can either:

1. Add the binary location to your PATH
2. Edit the skill files to use the full path
3. Set `LARK_CONFIG_DIR` environment variable to point to your `.lark/` config directory

The JSON output format makes it straightforward for AI assistants to parse responses and take action.

## License

MIT - see [LICENSE](LICENSE)
