# 🪖 Bulchimbeon（불침번）

> **为你的 Mac 站岗的小兵。**
> 只在 AI 编程代理**真正干活的时候**阻止 Mac 休眠，活儿干完就敬个礼说"换岗！"，
> 让 Mac 恢复正常睡眠的菜单栏应用。
>
> **无需 hook、无需守护进程、不安装 root 辅助程序。一个应用，macOS 13 起即可使用。**

*불침번*（读作 bul-chim-beon，汉字「不寝番」）是韩国军队里的守夜岗：一个人整夜不睡，好让大家安心睡觉。
现在由一个像素风小兵来替你的 Mac 值夜班。

> 📝 本中文版由 AI 翻译，欢迎提交 PR 改进表达。
> [English](README.md) · [한국어](README.ko.md) · [日本語](README.ja.md)
> 💡 [Ideas / 문의 / 建议](https://github.com/s3onghyun/bulchimbeon/discussions)
> 🗳️ **你和哪个代理一起用？在哪一步卡住了？** [来投票并告诉我们](https://github.com/s3onghyun/bulchimbeon/discussions)

![Bulchimbeon 演示](docs/media/demo.gif)

## 快速开始

安装：`brew install --cask s3onghyun/tap/bulchimbeon` —— 或[下载最新的 macOS zip](https://github.com/s3onghyun/bulchimbeon/releases) 并移动到 `/Applications`。

**首次启动：** 应用目前为 ad-hoc 签名，尚未经过 Apple 公证，macOS 会拦截一次：

1. 打开 Bulchimbeon。macOS 提示无法打开时，点击 **完成**。
2. 前往 **系统设置 → 隐私与安全性**，向下滚动到 **安全性**。
3. 在 *已阻止“Bulchimbeon”* 旁点击 **仍要打开**，然后确认。
   （从 macOS 15 起，右键 → 打开 已无法绕过。）

<!-- TODO(mac-capture): add the Gatekeeper screenshot docs/media/gatekeeper-open-anyway.png here -->

通知显示为 **「脚本编辑器」** 是正常的：公证之前，macOS 不会给应用本身授予通知权限。

**支持的代理：** Claude Code · Codex · Gemini CLI · Cursor Agent · Copilot CLI · Aider · OpenCode（也可添加自定义代理）。

### 安装后 1 分钟检查

1. 给代理派个任务，例如让 Claude Code 跑一遍测试。
2. 几秒内，菜单栏里的小兵会打开手电筒。
3. 打开菜单。第一行显示 **Bulchimbeon — 值班中 · 已过 0:12 · …**，下一行写明原因，
   例如 **Claude Code 正在工作**。
4. 没有反应？运行 `/Applications/Bulchimbeon.app/Contents/MacOS/Bulchimbeon --diagnose` 查看检测结果，
   并[告诉我们卡在哪里](https://github.com/s3onghyun/bulchimbeon/discussions)。

要查看 macOS 实际持有的防休眠，运行 `pmset -g assertions`。值班中：

```text
2026-10-31 13:17:17 +0900
Assertion status system-wide:
   BackgroundTask                 0
   ApplePushServiceTask           0
   UserIsActive                   0
   PreventUserIdleDisplaySleep    0
   SoftwareUpdateTask             0
   PreventSystemSleep             0
   ExternalMedia                  0
   PreventUserIdleSystemSleep     1
   NetworkClientActive            0
Listed by owning process:
   pid 2102(Bulchimbeon): [0x002847ba00019690] 13:31:53 PreventUserIdleSystemSleep named: "Bulchimbeon on duty: keeping AI agents awake"
Kernel Assertions: 0x100=MAGICWAKE
   id=592  level=255 0x100=MAGICWAKE creat=  mod= description=en0 owner=IOSkywalkNetworkBSDClient
```

- `2026-10-31 13:17:17 +0900`：pmset 最先输出的日期和时间。
- `Assertion status system-wide`：每种防休眠当前被持有的数量。
  `PreventUserIdleSystemSleep 1` 表示有程序在阻止 Mac 空闲休眠。`PreventUserIdleDisplaySleep 0`
  表示屏幕仍可关闭，这是默认的值班模式；选择 **保持屏幕常亮** 时这一行会变成 1。
- `Listed by owning process`：由谁持有。`pid 2102(Bulchimbeon)` 是本应用（你的 pid 会不同），
  `[0x002847ba00019690]` 是这条防休眠的 ID，`13:31:53` 是开始持有的时间，`PreventUserIdleSystemSleep` 是类型，
  `named: "Bulchimbeon on duty: keeping AI agents awake"` 是可用来查找的固定名称。
  其他让 Mac 保持唤醒的应用也会列在这里。
- `Kernel Assertions`：macOS 自己的条目（这里是 `en0` 的网络唤醒），与 Bulchimbeon 无关，你的可能不同。

捕获环境：macOS 26.6.2，Apple Silicon，Bulchimbeon 0.11.0。

待命时（没有代理在工作），Bulchimbeon 会释放防休眠，`pid …(Bulchimbeon)` 这一行随之消失。

<!-- TODO(mac-capture): real idle `pmset -g assertions` output and its line-by-line explanation -->

### 常见问题

**为什么不直接用 `caffeinate` 或 Amphetamine？** 想手动让 Mac 保持唤醒一段固定时间时，它们很好用。Bulchimbeon 只在 AI 代理真正干活时开启和关闭防休眠，所以不会整夜开着。

**它会查看或发送什么？** 不使用 hook。唯一的网络请求是每天一次的[检查更新](README.md#updates)，除了应用版本什么也不发送，并且可以关闭（`defaults write io.github.s3onghyun.bulchimbeon checkForUpdates -bool false`，Linux 上为 `bulchimbeon config set checkForUpdates false`）。它只查看你自己账户的本地进程元数据来判断支持的代理是否在工作，从不保存或输出进程命令行和环境变量。

**合上盖子也能保持唤醒吗？** 可以，但开启合盖模式需要管理员授权。值班结束或退出应用时，会恢复原来的合盖睡眠设置。

| 状态 | 小兵 | 何时 |
|------|------|------|
| 💤 就寝 | 戴着钢盔打瞌睡，zzz | 没有需要守护的任务 |
| 👀 待命 | 东张西望 | 等待代理"上班" |
| 🪖 站岗中 | 拿着手电筒巡逻 | Mac 不会休眠 |
| 🫡 换岗 | `^ ^` 敬礼 | 代理安静下来 / 站岗时间结束 |
| 🥵 战斗休息 | 满头大汗，闭眼降温 | Mac 过热 |

**巡逻模式**：小兵会拿着手电筒在菜单栏正下方来回巡逻。所有点击都会直接穿透，鼠标靠近时他会变成半透明让路。
在全屏应用（演示、屏幕共享、视频）上不会出现。

## 为什么做这个

为了让代理整晚跑任务，我以前是这么干的：

```bash
caffeinate -d                 # ……然后忘了关
sudo pmset disablesleep 1     # ……然后忘了改回来，电池耗到 0%
```

Bulchimbeon **只在代理干活时**保持唤醒，干完了自动换岗。

## 功能

| 功能 | 说明 |
|------|------|
| 🤖 代理站岗 | 识别 Claude Code、Codex、Gemini CLI、Cursor Agent、Copilot CLI、Aider、OpenCode（可自定义添加）。**只统计正在工作的代理，不统计仅仅开着的会话**，安静几分钟后自动换岗 |
| 🪖 定时站岗 | 30 分钟 / 1 小时 / 2 小时 / 4 小时 / 无限期 |
| 站岗方式 | 屏幕常亮（`caffeinate -d`）· 屏幕可熄灭但 Mac 保持唤醒（`caffeinate -i`）· **合盖也保持唤醒**（需管理员权限） |
| 🔋 低电量撤岗 | 使用电池且电量低于阈值（默认 20%）时撤岗 |
| 🌡️ 高温预警 → 🔥 战斗休息 | Mac 变热时提醒；温度危险时暂停站岗让它降温，**降温后自动恢复** |
| ⚠️ 擅离岗位检测 | 防休眠断言因任何原因丢失时立即恢复 |
| 🕵️ 值班哨兵 | 合盖模式下即使应用被强制退出，也会在 5 秒内恢复原来的休眠设置 |
| 📋 交接班报告 | 离开 10 分钟以上回来时，汇报这段时间的站岗情况 |
| 🔁 持续站岗 | 应用重启、更新、系统重启后继续剩余的站岗时间 |
| 🖥️ 其他阻止休眠的应用 | 菜单里会显示，例如"Vivaldi（视频）" |
| 🌐 简体中文 / 韩语 / 英语 | 默认跟随系统语言，可在菜单 **언어 / Language / 语言** 中切换 |

## 与同类应用对比

[Adrafinil](https://github.com/kageroumado/adrafinil) 更早解决了同一个问题。两者思路相反，按需选择即可。（截至 2026 年 10 月）

| | Bulchimbeon | Adrafinil |
|---|---|---|
| 如何判断代理在工作 | **从外部观察**（`caffeinate` 子进程、CPU） | **代理主动通知**（hook）+ 观察 |
| 安装后改动了什么 | **没有**。不修改代理的配置文件 | 向代理配置添加 hook、用户守护进程、root 辅助程序 |
| macOS | **13 及以上** | 26.4 及以上 |
| 合盖保持唤醒 | 每次开启需管理员密码，哨兵保证恢复设置 | 安装时授权一次 |
| 代理主动请求保持唤醒 | — | CLI、MCP 工具 |
| 分发 | ad-hoc 签名（公证准备中） | 已公证，Homebrew 官方 cask |

**适合 Adrafinil**：macOS 26.4 及以上，想用 hook 精确联动或让代理主动请求。
**适合 Bulchimbeon**：使用 macOS 13–15，不想往代理配置或系统里安装任何东西，或者想要一个小兵站岗。

## 安装

```bash
brew install --cask s3onghyun/tap/bulchimbeon

# 更新 —— Homebrew 自动更新间隔较长，请先更新 tap
brew update && brew upgrade --cask bulchimbeon
```

需要 macOS 13 及以上，通用二进制（Apple Silicon 与 Intel）。

首次启动：macOS 会拦截一次，步骤见[快速开始](#快速开始)。

## 工作原理（简要）

- 防休眠使用 `IOPMAssertion`（与 `caffeinate` 相同）；合盖模式额外使用 `pmset -a disablesleep 1`
- 通过 libproc **只扫描当前用户的进程**，按可执行文件名、`argv[0]`、安装路径、脚本路径识别代理（不做子串匹配）
- "正在工作"的判断：Claude Code 只在工作时自己启动的 `caffeinate` 子进程，或进程树的 CPU 使用率（60 秒平均）
- 判断逻辑位于不含系统调用的状态机（`DutyEngine`）中，使用伪时钟做了场景测试

详情：[English README](README.md) · [设计文档（韩语）](docs/design.md)

## 安全

应用会以管理员权限执行 `pmset`，并读取其他进程的信息，因此单独整理了威胁模型：[SECURITY.md](SECURITY.md)

- 唯一的网络通信是每天一次的检查更新（只发送版本，可关闭），零第三方依赖
- 不保存、不输出其他进程的参数与环境变量；不读取 AI 对话记录
- 发布包只在 GitHub Actions 中构建，附带 SHA-256 校验和与构建来源证明（attestation）

## 参与贡献

欢迎提交 Bug（"🚨 小兵站岗态度不端"）、新代理识别请求、台词与像素画创意，以及**对本翻译的改进**：[CONTRIBUTING.md](CONTRIBUTING.md)

## 许可证

MIT

## 界面语言

中文系统默认显示简体中文（包括繁体中文地区设置，目前未提供繁体译文）。也可在菜单
**언어 / Language / 语言 → 简体中文** 中手动选择；应用会自动重启并恢复值班状态。
选择“跟随系统”可清除应用专用语言设置。不支持的系统语言仍回退到英语。

Linux CLI 可用 `bulchimbeon config set language zh` 保存中文偏好，或用
`bulchimbeon status --lang zh` 临时指定。语言优先级为命令行参数、已保存设置、系统环境变量。
