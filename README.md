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
- Install the binary into `~/.local/bin/lark`
- Copy the bundled skills into `${CODEX_HOME:-~/.codex}/skills`

If a skill already exists, the installer leaves it alone unless you pass `--force`.

After installing, restart Codex so it picks up the new skills.

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
