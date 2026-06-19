# lark-cli-codex-app

一个面向 Codex 的飞书/Lark 本地控制项目。它基于 [`yjwong/lark-cli`](https://github.com/yjwong/lark-cli)，保留上游 Go CLI 和技能定义，并补充了 Codex 插件元数据、本地安装脚本、飞书事件网关、Codex 任务分发和桌面任务队列。

这个项目的核心不是简单地“让 Codex 调用飞书 API”，而是把飞书/Lark 变成 Codex App 的远程入口和协作工作台：

```text
人在飞书/Lark 中发起任务
  -> lark gateway / webhook 接收消息事件
  -> 本地 Codex app-server bridge 执行任务
  -> Codex 使用 computer use 操作电脑、浏览器、终端和文件
  -> Codex 使用 lark-cli + skills 操作飞书/Lark
  -> 结果回写到飞书/Lark 对话、文档、日历、表格或多维表格
```

换句话说，这个项目并不是重新实现一个 agent，而是依托 Codex App 原生的本地执行、代码理解、工具调用、computer use 和未来插件生态潜力，把飞书/Lark 接入为 Codex 的远程交互入口、任务调度入口和团队协作回写界面。

![飞书控制 Codex App，Codex 回写飞书](./assets/readme/closed-loop.png)

## 项目定位

多数 chat-to-agent 项目只解决单向链路：人在聊天软件里发一条消息，本地 agent 执行后回复结果。

`lark-cli-codex-app` 想做的是更完整的闭环：

- **人在飞书里发起任务**：可以在单聊、群聊或移动端飞书里给 Codex 下指令。
- **Codex 在本机执行**：Codex app-server bridge 默认在你的本地机器上运行，拥有项目文件、终端、浏览器和开发工具上下文；`codex exec` 作为兼容回退保留。
- **Codex 操作电脑**：通过桌面任务队列和 computer use，Codex 可以处理需要前台 GUI 的任务。
- **Codex 操作飞书**：通过 `lark-cli` 和内置 skills，Codex 可以读取和更新飞书消息、文档、日历、表格、多维表格、邮件和妙记。
- **结果回到飞书**：执行结果、摘要、任务状态或后续动作可以回写到原来的飞书对话或相关协作对象中。

目标是基于 Codex App 自身能力持续增强的方向，让飞书/Lark 成为 Codex 的本地控制面和协作延展层，而不是只做一个普通机器人。

## 为什么选择 Codex App？

OpenClaw、Hermes 这类项目更像是从 agent runtime 开始构建一套完整的通用助理系统：长期运行、记忆、任务调度、工具编排和多入口交互都由项目自身承载。这条路线很强，也很适合做通用个人助理。

`lark-cli-codex-app` 选择了另一种边界：不重新造一个 agent runtime，而是依托 Codex App 已有并持续演进的原生能力，把飞书/Lark 接成 Codex 的远程控制入口和团队协作界面。

Codex App 本身更贴近真实工程任务：它理解本地代码上下文，可以使用终端、文件、浏览器、computer use、skills、多任务和未来插件生态。这个项目更希望把精力放在“飞书/Lark 如何可靠地触发 Codex、Codex 如何把结果回写到飞书/Lark”这条链路上。

也就是说：

```text
飞书/Lark 负责发起任务、团队协作和结果承接
Codex App 负责本地执行、代码理解、工具调用和 computer use
lark-cli-codex-app 负责把两边连成闭环
```

未来 Codex App 的本地执行、skills、computer use、多 agent 和自动化能力越强，这个项目能承接的飞书协作场景也会越多。

## 为什么需要这个项目？

官方 Lark MCP Server 可以使用，但对 AI assistant 来说不够省上下文。很多工具调用会返回较大的原始结构，容易占用大量 context window。

这个 CLI 的设计重点是让 Codex 以更低成本操作飞书/Lark：

- **紧凑 JSON 输出**：返回结构化、易解析、适合程序消费的结果。
- **文档 Markdown 转换**：将飞书文档转换成 Markdown，比原始 block 结构更适合 assistant 阅读。
- **选择性查询**：只取真正需要的信息，例如只取事件 ID、文档标题或部分记录字段。
- **技能化调用**：把常见飞书操作封装成 Codex/Claude Code 可复用的 skills。

这样 Codex 可以把更多上下文留给真正的工作，而不是消耗在冗长 API payload 上。

## 典型场景

- 在飞书群里让 Codex 排查一个 bug。Codex 在本地读取仓库、运行测试、总结原因，并把结果回复到群里。
- 出门时用手机飞书给本机 Codex 发任务。Codex 在你的本地 workspace 里继续工作，完成后回报进度。
- 让 Codex 总结一篇飞书文档，提取 action items，并在飞书里创建后续任务或回复摘要。
- 让 Codex 结合本地代码上下文和飞书数据，例如会议纪要、聊天记录、日历、表格、多维表格，生成更完整的判断。
- 通过飞书触发桌面操作请求，例如打开浏览器、进入某个网页、准备一个需要前台处理的 GUI 任务。

## 项目方向

这个项目会继续围绕“飞书/Lark 控制 Codex App，并让 Codex App 反过来操作飞书/Lark”这个闭环演进：

- 从飞书/Lark 接收个人或群组任务。
- 将任务路由到本地 Codex app-server bridge，并保留 `codex exec` 回退路径。
- 保留本地事件日志，便于排查、回放和调试。
- 将桌面 GUI 请求交给前台 helper 处理。
- 让 Codex 将结构化结果回写到飞书/Lark 中团队正在工作的地方。

## 功能

- **日历**：列出、创建、更新、删除日程；查询忙闲；寻找共同空闲时间；回复邀请。
- **通讯录**：按 ID 查询用户，按姓名搜索用户，列出部门成员。
- **文档**：读取文档 Markdown，列出文件夹，解析知识库节点，获取评论。
- **消息**：读取聊天历史，下载附件，发送消息，管理表情回应。
- **网关**：通过飞书/Lark WebSocket 长连接在本地接收机器人消息事件。
- **Agent Bridge**：将飞书消息分发给长驻 Codex app-server 任务，并把结果回复到聊天中。
- **桌面队列**：将飞书中的桌面操作请求路由到本地 Codex Desktop 前台任务队列。
- **Webhook**：在需要 callback 模式时提供本地 webhook server 作为可选方案。
- **邮件**：通过 IMAP 本地缓存读取和搜索邮件。
- **妙记**：获取会议录制信息，导出转写文本，下载音视频。
- **表格**：读取飞书电子表格元数据和内容。
- **多维表格**：查询飞书多维表格记录和元数据。

## 这个 fork 增加了什么？

- Codex 插件清单： [`.codex-plugin/plugin.json`](.codex-plugin/plugin.json)
- Codex 本地安装脚本： [`scripts/install-codex-plugin.sh`](scripts/install-codex-plugin.sh)
- 本地 bridge 管理脚本： [`scripts/manage-bridge.sh`](scripts/manage-bridge.sh)
- 可复制到 Codex 或 Claude Code 的内置 skills： [`skills/`](skills/)
- 飞书/Lark 本地 WebSocket 网关，用于把聊天消息路由到 Codex 任务。
- 桌面任务队列，用于把 GUI 请求转交给前台 Codex Desktop 会话处理。

## 快速开始

1. 在 https://open.larksuite.com 创建 Lark 应用，或在 https://open.feishu.cn 创建飞书应用，并配置所需权限。
2. 将 `config.example.yaml` 复制到 `.lark/config.yaml`，填入 App ID。
3. 在 `.lark/config.yaml` 中设置 `region`：国际版 Lark 使用 `lark`，飞书使用 `feishu`。
4. 设置 `LARK_APP_SECRET` 环境变量。
5. 运行 `./lark auth login` 完成授权。
6. 开始使用，例如：`./lark cal list --week`

完整命令说明见 [USAGE.md](USAGE.md)。

## 构建

```bash
make build    # 构建二进制到 ./lark
make test     # 运行测试
make install  # 安装到 $GOPATH/bin
```

## 在 Codex 中使用

这个仓库可以作为 Codex 插件项目使用，因为它包含：

- `.codex-plugin/plugin.json` 插件清单
- `skills/` 内置技能目录
- 用于复制技能和安装 CLI 的本地安装脚本
- 用于安装、启动、停止和排查本地 bridge 的管理脚本

### 安装到 Codex

#### 通过 Codex Plugin Marketplace 安装

本仓库包含 repo-scoped marketplace 元数据：

```text
.agents/plugins/marketplace.json
```

把这个仓库作为本地 marketplace source 加到 Codex：

```bash
codex plugin marketplace add /path/to/lark-cli-codex-app
```

然后重启 Codex，在 Codex App 的 **Plugins** 里切换到 **Lark Codex App** marketplace，安装 **Lark Codex Bridge**。

这个 marketplace 入口负责让 Codex 识别和安装插件本体；本地 bridge 后台服务、飞书 OAuth、`~/.lark/env.sh` 里的 `LARK_APP_SECRET` 仍需要按下面的安装步骤配置。

#### 通过安装脚本安装 CLI 和 Skills

构建 CLI，并将内置 skills 安装到本地 Codex home：

```bash
./scripts/install-codex-plugin.sh
```

默认行为：

- 构建 `lark` 到 `./lark`
- 将二进制安装到 `~/.local/bin/lark`，并通过 wrapper 自动加载 `~/.lark/env.sh`
- 将内置 skills 复制到 `${CODEX_HOME:-~/.codex}/skills`
- 检查 `lark auth status`，如果 OAuth 已过期或未登录，会提示运行 `lark auth login`
- 保留 bridge 管理脚本在仓库内，便于用 launchd 启动 gateway

如果目标 skill 已存在，安装脚本会跳过它。需要覆盖时可使用：

```bash
./scripts/install-codex-plugin.sh --force
```

安装脚本会检查 OAuth 状态。在交互式终端里，如果 user OAuth 未登录或已过期，会询问是否立刻运行 `lark auth login`。也可以显式要求安装后直接进入授权流程：

```bash
./scripts/install-codex-plugin.sh --login
```

安装后重启 Codex，让新的 skills 生效。

如果需要让 Codex 读取或更新日历、文档、消息历史、表格、邮件等 user-scoped API，请先完成 OAuth：

```bash
~/.local/bin/lark auth login
~/.local/bin/lark auth status
```

### 管理本地 Bridge

安装 CLI 和 skills 后，可以用仓库内脚本管理本地 Lark -> Codex bridge：

```bash
./scripts/manage-bridge.sh install
./scripts/manage-bridge.sh start
./scripts/manage-bridge.sh status
./scripts/manage-bridge.sh logs
./scripts/manage-bridge.sh restart
./scripts/manage-bridge.sh stop
```

也可以通过 Makefile 使用同样的入口：

```bash
make bridge-restart
make bridge-status
make bridge-logs
```

常用环境变量覆盖：

```bash
LARK_AGENT_WORKSPACE="$HOME/WorkSpace" ./scripts/manage-bridge.sh restart
LARK_AGENT_BACKEND=codex_exec ./scripts/manage-bridge.sh restart
LARK_AGENT_REASONING_EFFORT=low ./scripts/manage-bridge.sh restart
```

默认后端是 `app_server`，它会复用长驻 `codex app-server`，比每次启动 `codex exec` 更适合聊天触发任务。

Bridge 会把飞书会话和 Codex thread 绑定到本地 JSON 文件，并记录最近任务摘要，默认路径是：

```text
~/.lark/codex-thread-bindings.json
```

在飞书里可以发送这些控制命令：

```text
#status
#bind 1
#new
#reset
```

- `#status`：查看当前飞书话题/聊天连接的 Codex 会话；如果尚未连接，会列出最近可连接的 Codex 会话和摘要。
- `#bind 1`：连接 `#status` 列出的第 1 个 Codex 会话；也支持高级用法 `#bind <codex_thread_id>`。
- `#new`：新建一个持久 Codex thread 并连接当前飞书话题/聊天。
- `#reset`：断开当前飞书话题/聊天和 Codex 会话的连接。

这不是接管 Codex App 当前 UI 输入框；它是让飞书消息通过 `codex app-server` 继续指定的底层 Codex thread。bridge 会把 App 可见的用户输入尽量保持为“来自飞书消息 + 原文 + 极短来源说明”，避免把内部路由 prompt 写进会话历史；但 Codex App 已打开的聊天窗口不会因此被远程控制，是否刷新仍取决于 App 自身的前端订阅机制。

## Gateway 模式

Gateway 模式是推荐的本地控制方式。它使用飞书/Lark WebSocket 事件订阅，不需要公网 callback URL：

![网关模式：本地优先的控制链路](./assets/readme/gateway-mode.png)

```bash
lark gateway serve
```

如果你希望把它作为后台服务运行，推荐使用：

```bash
./scripts/manage-bridge.sh restart
```

常用参数：

```bash
lark gateway serve \
  --event-log ~/.lark/gateway-events.jsonl \
  --auto-reply-text "收到：{{text}}"
```

启用本地 Codex agent：

```bash
lark gateway serve \
  --agent \
  --agent-workspace ~/WorkSpace \
  --agent-backend app_server \
  --agent-reasoning-effort medium
```

Desktop GUI 请求会进入单独队列。仍然支持 `/gui ` 前缀，但普通桌面请求也会被自动识别，例如：

```text
打开 Safari，然后访问 openai.com
```

这类请求会被 gateway 加入桌面任务队列，并回复 task id，而不是直接发送给 Codex agent bridge。

Gateway 做的事情：

- 通过 outbound WebSocket 连接飞书/Lark。
- 接收 `im.message.receive_v1` 消息事件，不需要公网回调地址。
- 将收到的消息事件追加到本地 JSONL 日志。
- 可选：用机器人自动回复消息。
- 可选：将飞书消息分发给长驻 Codex app-server 执行，并把结果回复到聊天中；需要时可切换回 `codex_exec`。
- 让 Codex 任务通过 `lark` 命令和 Codex skills 回写飞书/Lark。
- 将显式 `/gui ...` 消息或自动识别出的桌面操作请求放入桌面任务队列。

典型设置：

1. 在飞书/Lark 开放平台后台启用事件订阅，并选择 **长连接 / WebSocket** 模式。
2. 订阅消息接收事件：`im.message.receive_v1`。
3. 确认机器人已加入目标聊天。
4. 在本地运行 `lark gateway serve`。

如果希望机器人触发本地 Codex 任务，而不是只做普通 echo bot，可在 `config.yaml` 中启用 `agent` 配置，或启动时添加 `--agent`。

如需真正执行点击、输入等前台 GUI 操作，请让后台 gateway 专注于接收飞书消息，然后在一个有 GUI 权限的前台会话中运行：

```bash
lark desktop helper serve
```

当前限制：

- 打开应用和链接可以直接工作。
- 键盘驱动的 GUI 操作可能需要给运行 `lark desktop helper serve` 的前台应用授予 macOS Accessibility 权限，例如 Terminal 或 Codex Desktop。
- 如果没有该权限，helper 会尽量退化为打开应用或返回可计算结果。

### 桌面队列辅助命令

桌面队列可以用下面的命令查看和驱动：

```bash
lark desktop tasks pop
lark desktop tasks complete --id <task-id> --result "done" --reply
lark desktop tasks fail --id <task-id> --error "why" --reply
```

这是推荐的本地开发路径，因为它不需要公网 HTTPS tunnel。

## Webhook 模式

当你明确需要 callback 模式时，可以使用本地 webhook server：

```bash
lark webhook serve
```

常用参数：

```bash
lark webhook serve \
  --listen 0.0.0.0:8080 \
  --path /webhook/feishu \
  --token your-verification-token \
  --auto-reply-text "收到：{{text}}"
```

Webhook 做的事情：

- 处理 URL verification 的 `challenge` 回调。
- 接收明文 `im.message.receive_v1` 事件。
- 将收到的消息事件追加到本地 JSONL 日志。
- 可选：用机器人回复收到的消息。

当前限制：

- 暂不支持加密 callback。本版本中请将飞书/Lark 后台的 **Encrypt Key** 留空。

典型设置：

1. 通过公网 HTTPS tunnel 或反向代理暴露本地 server。
2. 在飞书/Lark 应用后台将 request URL 设置为公网地址加 webhook path。
3. 如果后台配置了 verification token，请在 `webhook.verification_token` 或 `--token` 中使用相同值。
4. 订阅消息接收事件：`im.message.receive_v1`。
5. 将机器人加入目标聊天。

## 在 Claude Code 中使用

这个工具也可以通过 Claude Code skills 调用。预置 skill 定义位于 `skills/` 目录。

### 安装 Skills

将 skill 目录复制到对应 assistant 的 skills 位置：

```bash
# Codex 用户级
cp -r skills/* ~/.codex/skills/

# Claude Code 项目级
cp -r skills/* /path/to/your/project/.claude/skills/

# Claude Code 用户级
cp -r skills/* ~/.claude/skills/
```

可用 skills：

- `bitable`：读取多维表格 app 元数据和记录。
- `calendar`：管理日历事件，查询忙闲，回复邀请。
- `contacts`：查询用户和部门。
- `documents`：读取文档，列出文件夹，浏览知识库。
- `lark-bridge`：安装、启动、停止、查看和排查本地 Lark -> Codex bridge。
- `messages`：读取聊天记录，下载附件，向用户或群聊发送消息。
- `email`：通过 IMAP 本地缓存读取和搜索邮件。
- `minutes`：获取会议录制，导出转写文本，下载媒体。
- `sheets`：读取飞书电子表格数据。

### 配置

skills 默认假设 `lark` 已在 `PATH` 中。如果不在，可以选择：

1. 将 `lark` 所在目录加入 `PATH`。
2. 修改 skill 文件，使用 `lark` 的完整路径。
3. 设置 `LARK_CONFIG_DIR` 环境变量，指向你的 `.lark/` 配置目录。

CLI 的 JSON 输出格式让 AI assistant 更容易解析结果并继续执行动作。

## 许可证

MIT，详见 [LICENSE](LICENSE)。

## 社区与致谢

本项目认可并感谢 [LINUX DO](https://linux.do) 社区对开源项目交流、试用反馈和公开讨论的支持。欢迎从社区讨论出发，一起把“飞书/Lark 控制 Codex App，Codex App 再回写飞书/Lark”的本地闭环工作流打磨得更稳定、更实用。
