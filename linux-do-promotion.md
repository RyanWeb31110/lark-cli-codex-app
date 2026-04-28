# lark-cli-codex-app 开源推广帖

#### 本帖使用社区开源推广，符合推广要求。我申明并遵循社区要求的以下内容：

* **我的帖子已经打上 #开源推广 标签：** 是
* **我的开源项目完整开源，无未开源部分：** 是
* **我的开源项目已链接认可 LINUX DO 社区：** 是
* **我帖子内的项目介绍，AI生成、润色内容部分已截图发出：** 是
* **以上选择我承诺是永久有效的，接受社区和佬友监督：** 是

*以下为项目介绍正文内容，AI生成、润色内容已使用截图方式发出*

---

## lark-cli-codex-app：用飞书/Lark 远程控制 Codex App 的本地闭环工作流

项目地址：

https://github.com/RyanWeb31110/lark-cli-codex-app

这是一个围绕 Codex App 和飞书/Lark 做的本地控制项目。简单说，它想解决的是：

人在飞书里发消息、派任务；本地 Codex App 收到后在电脑上执行；Codex 可以读取本地仓库、运行命令、操作浏览器/桌面，也可以通过飞书/Lark API 回写消息、文档、日历、表格、多维表格等协作对象。

也就是说，飞书/Lark 既是远程入口，也是协作结果的回写界面；Codex App 则负责真正的本地执行、代码理解、工具调用和 computer use。

### 这个项目想做什么

我自己理解的核心链路是：

```text
人在飞书/Lark 发起任务
  -> 本地 gateway 接收飞书消息事件
  -> 任务分发给 Codex App / codex exec
  -> Codex 在本机执行代码、终端、浏览器或桌面任务
  -> Codex 通过 lark-cli / skills 操作飞书
  -> 结果回写到飞书对话、文档、日历、表格等地方
```

它不是重新造一个 agent，而是依托 Codex App 原生的本地执行能力、代码理解能力、工具调用能力、computer use 能力，以及未来插件生态的潜力，把飞书/Lark 接成一个更适合团队协作的远程控制面。

### 为什么做这个

我之前试过一些聊天工具控制本地 agent 的方案，很多都停留在“发一条消息，agent 回复一段结果”的单向链路。

但我更想要的是闭环：

- 我可以在飞书群里让 Codex 排查一个 bug
- Codex 在本地 workspace 里读代码、跑测试、改文件
- 需要 GUI 时，可以交给 Codex App / computer use 操作电脑
- 需要协作时，可以把结果回写到飞书群、文档、任务、表格或多维表格
- 出门在外时，也能用手机飞书给家里/办公室电脑上的 Codex 派任务

这个场景对我来说挺自然：飞书负责入口和协作，Codex 负责本地执行和智能操作，中间通过一个本地 gateway 串起来。

### 为什么选择 Codex App，而不是重新做一个 OpenClaw / Hermes 这类通用 agent？

OpenClaw、Hermes 这类项目更像是从 agent runtime 开始构建一套完整的通用助理系统：长期运行、记忆、任务调度、工具编排和多入口交互都由项目自身承载。这条路线很强，也很适合做通用个人助理。

但这个项目没有选择重新造一个 agent runtime，而是选择站在 Codex App 上做飞书/Lark 控制层，原因很简单：我更看重 Codex App 已经具备、并且还会持续增强的原生能力。

Codex App 本身更贴近真实工程任务：它理解本地代码上下文，可以使用终端、文件、浏览器、computer use、skills、多任务和未来插件生态。对我来说，飞书/Lark 不应该替代 Codex App，而应该成为 Codex App 的远程入口和团队协作界面。

所以这个项目的定位不是“再做一个 OpenClaw / Hermes”，而是：

```text
飞书/Lark 负责发起任务、团队协作和结果承接
Codex App 负责本地执行、代码理解、工具调用和 computer use
lark-cli-codex-app 负责把两边连成闭环
```

这样做的好处是边界更清楚：agent 能力交给 Codex App 演进，飞书/Lark 集成交给这个项目打磨。未来 Codex App 的 skills、computer use、多 agent、自动化和本地执行能力越强，这个项目能承接的飞书协作场景也会越多。

### 当前已经有的能力

目前项目里主要包括：

- 飞书/Lark CLI：日历、消息、文档、通讯录、表格、多维表格、邮件、妙记等操作
- Codex 插件元数据：可以作为 Codex plugin 项目使用
- skills 目录：把常见飞书操作封装成 Codex 可调用的技能
- gateway 模式：通过飞书/Lark WebSocket 长连接接收消息事件，不需要公网 callback
- agent bridge：把飞书消息分发给本地 `codex exec`
- desktop task queue：把 GUI / computer use 类任务放进本地桌面任务队列
- README 里配了两张流程图，方便理解整体链路

### 和普通 bot 的区别

普通 bot 更像是：

```text
聊天软件 -> bot -> 回复
```

这个项目想做的是：

```text
飞书/Lark -> 本地 Codex App -> 电脑 / 代码 / 浏览器 / 飞书 -> 飞书/Lark
```

重点不只是“回复消息”，而是让 Codex 成为一个能在本地真实干活、并能把结果带回协作场景里的执行层。

### 开源情况

项目是基于 MIT 协议的 `yjwong/lark-cli` fork 做的扩展，保留了原许可证，并在此基础上增加 Codex App / gateway / desktop queue / skills 相关能力。

当前项目完整开源，没有保留未开源部分。README 中也已链接并认可 LINUX DO 社区：

https://github.com/RyanWeb31110/lark-cli-codex-app

欢迎佬友们拍砖，尤其是这几个方向：

- 飞书控制 Codex App 这个闭环是否还有更自然的交互方式
- gateway / desktop queue 的设计是否合理
- 作为 Codex 插件项目还缺哪些关键能力
- README 和使用门槛哪里还可以继续降低
