# QuotaDock

[English](./README.md) | [한국어](./README.ko-KR.md) | [简体中文](./README.zh-CN.md) | [繁體中文（臺灣）](./README.zh-TW.md) | [日本語](./README.ja-JP.md)

QuotaDock 是一款常驻置顶的轻量 Windows 桌面小组件，让你一眼看清 **Claude、OpenAI Codex 与
Google Antigravity** 的用量额度 —— 会话/每周配额以及重置倒计时。

[![Release](https://img.shields.io/github/v/release/jungdosa/QuotaDock?include_prereleases&label=release)](https://github.com/jungdosa/QuotaDock/releases)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Fyne](https://img.shields.io/badge/GUI-Fyne-orange)](https://fyne.io/)
[![Platform](https://img.shields.io/badge/platform-Windows%2010%2F11-0078D4?logo=windows&logoColor=white)](#支持平台)

<p align="center">
  <img src="docs/marketing/quotadock-normal-en.png" alt="QuotaDock 普通视图 —— Claude / Codex / Antigravity 用量与重置倒计时" width="539">
</p>

使用 Claude Code、Codex CLI 和 Antigravity IDE 时，你总在问同样的问题：*5 小时会话还剩多少？
每周额度什么时候重置？* QuotaDock 替你盯着，不必再去逐个查看。

| 服务商 | 显示内容 | 连接方式 |
|---|---|---|
| **Claude**（Claude Code） | 5 小时会话 · 7 天每周 · Fable 每周 | 复用官方 Claude CLI 的登录凭据 |
| **OpenAI Codex** | 会话 · 每周额度 | 官方 Codex CLI app-server（stdio JSONL） |
| **Google Antigravity** | Gemini 与 Claude/GPT 组的会话 · 每周 | 本地语言服务器（loopback） |

某一个服务商连接失败时，其余仍照常工作。每一行都会在**分段条（已用量）**下方绘制一条
**细长连续条（距离重置的剩余时间）**，让你一眼判断额度是否消耗得比时间更快。超过警告或
危险阈值时，进度条与数值会切换为警示色。

## 界面

三种小组件视图外加设置界面。工具栏按钮按 `普通 → 紧凑 → 迷你` 循环切换，迷你视图单击即可
返回紧凑视图。

### 普通 —— 信息最完整

按服务商分组，显示套餐徽章、用量条与重置条、倒计时和重置时刻。

<p align="center"><img src="docs/marketing/quotadock-normal-en.png" alt="普通视图（深色）" width="480"></p>

### 紧凑 —— 每行一条

更窄的视图，百分比旁边直接显示距离重置的剩余时间。将鼠标悬停在时间上会弹出精确重置时刻的
提示。

<p align="center"><img src="docs/marketing/quotadock-compact-en.png" alt="紧凑视图（深色）" width="312"></p>

### 迷你 —— 极小长条

每个服务商占一格，高度约 78px，适合固定在显示器角落。悬停某一行会显示三行提示：
`服务商 · 窗口 / 剩余时间 / 重置时刻`。

<p align="center"><img src="docs/marketing/quotadock-nano-en.png" alt="迷你视图（深色）" width="360"></p>

## 功能

- **浅色 · 深色 · 跟随系统主题**，12/24 小时日期格式，按服务商配色，警告阈值可调
- **十二种语言** —— 英语、韩语、德语、法语、意大利语、印尼语、葡萄牙语（巴西）、
  西班牙语（西班牙/拉丁美洲）、日语、简体中文、繁体中文（台湾）。选择 `系统` 则跟随 Windows 显示语言
- **连接方式按钮** —— 每个服务商行都会显示它通过哪种方式（`CLI`、`Auth`、`IDE`）连接
  以及当前状态。若缺少对应 CLI，点击按钮会在卡片内直接展开安装指引
- **内置更新** —— 启动时检查一次 GitHub Releases，也可随时手动检查。下载文件会与发布资源的
  SHA-256 校验**两次**：下载完成后一次，运行安装程序前再一次。不做周期轮询，不发送任何凭据
- **固定到任务栏（Windows 11）** —— 让托盘图标不被折叠进溢出区域

## 安装

从 [**Releases**](https://github.com/jungdosa/QuotaDock/releases) 下载。

| 文件 | 用途 |
|---|---|
| `QuotaDock-<版本>-win-x64-Setup.exe` | 安装版 —— 创建开始菜单项，可选开机自启 |
| `QuotaDock-<版本>-win-x64-portable.exe` | 免安装单文件版 |

二进制文件未做代码签名，SmartScreen 可能会提示。请用 `SHA256SUMS.txt` 校验完整性。
无需任何配置 —— QuotaDock 会自动发现你已登录的官方 CLI 与 IDE。

> 当前版本为 `0.7.15`。完成 Windows 功能验证后将升为 `1.0.0`。

## 首次运行

没有设置向导，没有登录界面，也不需要粘贴任何东西。

1. 启动 **QuotaDock**，它会以常驻置顶的小窗口出现。
2. 它会自动寻找你已经在用的官方工具并连接 —— Claude Code CLI、Codex CLI 和 Antigravity IDE。
3. 已登录的服务商会在几秒内开始显示用量。

如果某一行提示缺少 CLI，请打开 **设置 → 连接**，点击该服务商的 `CLI` 按钮。安装指引会直接在
卡片内展开 —— 需要执行的命令、如何登录，以及 QuotaDock 的查找路径。装好后点 `重新扫描` 即可，
无需重启。

日常使用：

- 工具栏按钮按 `普通 → 紧凑 → 迷你` 循环，迷你视图单击即可回到紧凑视图。
- `✕` 是收进托盘而不是退出。退出请从托盘菜单操作。
- 拖动标题栏可移动窗口，位置会在下次启动时保留。

## 隐私

QuotaDock 的设计前提是**它根本看不到你的凭据**。

- **只读取，不询问。** Claude 用量来自 Claude Code 早已保存在本机的令牌；Codex 用量通过 stdio
  与官方 Codex CLI 的 app-server 通信；Antigravity 用量来自 `127.0.0.1` 上的语言服务器。
  没有粘贴 session key 的输入框，不提取浏览器 Cookie，也没有浏览器自动化。
- **凭据不会出现在界面上。** 令牌、Cookie 和凭据文件原文不会渲染到 UI，传给界面的只有归一化的
  用量比例、经白名单校验的套餐标签和重置时刻。
- **诊断记录仅保留在本机。** 正常使用时会把小型、限长的 JSON 诊断日志写入
  `%LOCALAPPDATA%\QuotaDock\quotadock.log`。异常退出时，同一目录还可能留下 `crash.log`。
  密钥和邮箱地址会在写入前脱敏，两份日志都不会发送到任何地方。
- **对外通信仅限一份短白名单。** 服务商请求只能到达 `api.anthropic.com` 与
  `platform.claude.com`；更新检查只能到达 `api.github.com` 与 GitHub 的发布主机，且不携带任何
  凭据。其余全部是回环连接。没有遥测、没有分析、没有崩溃上报。
- **配置归你所有。** 配置保存在 `%APPDATA%\QuotaDock\settings.json`，卸载后仍会保留，且不会
  同步到任何地方。
- **不产生计费请求。** 刷新用量绝不会消耗你的额度或积分。

## 设计原则

- **不打扰。** 只要官方工具已安装并登录，它就自动连接。没有首次启动的登录向导，也没有强制弹窗。
- **不接触机密。** 令牌、Cookie 和 `auth.json` 原文不会进入界面或日志。传给界面的只有归一化的
  用量比例、经白名单校验的套餐标签和重置时刻。
- **只连本地。** 没有遥测，没有外部分析服务。loopback 连接仅允许连往经过校验的进程与固定端点。
  唯一的对外请求是更新检查，它不携带任何凭据，且只在启动时或你点击时发生。
- **不消耗额度。** 刷新用量绝不会发送计费的 AI 请求。
- **足够轻。** Go + Fyne 原生渲染，空闲 CPU 占用为 0%，内存主动管理。

## 支持平台

| 优先级 | 平台 | 状态 |
|---|---|---|
| 第一 | Windows 10 22H2+ / 11, x64 | **可用**（0.7.x） |
| 第二 | macOS 14+, Apple Silicon | 计划中 |
| 第三 | Linux x64 → arm64 | 计划中 |

## 从源码构建

Fyne 使用 CGO，仅有 Go 无法完成构建。

- Go 1.26 或更高版本
- C 编译器（Windows：MinGW-w64 / macOS：Xcode Command Line Tools / Linux：gcc + X11 开发头文件）

```sh
git clone https://github.com/jungdosa/QuotaDock.git
cd QuotaDock
go build ./cmd/quotadock
```

发布产物（Setup.exe、portable、SHA256SUMS）由 `build/windows/build-release.ps1` 生成。

## 许可证

MIT —— 参见 [LICENSE](LICENSE)。

QuotaDock 的源码、文案、界面设计与应用图标均为原创。后续工作计划记录在
[docs/REMAINING-WORK.md](docs/REMAINING-WORK.md)。

### 第三方资源

| 资源 | 来源 | 许可证 |
|---|---|---|
| `assets/fonts/Pretendard-*.ttf` | [Pretendard](https://github.com/orioncactus/pretendard) | SIL OFL 1.1 —— [`Pretendard-OFL.txt`](assets/fonts/Pretendard-OFL.txt) |
| `assets/providers/*.svg` | [lobe-icons](https://github.com/lobehub/lobe-icons) | MIT —— [`LICENSE-lobe-icons.txt`](assets/providers/LICENSE-lobe-icons.txt) |

服务商标志为各自所有者（Anthropic、OpenAI、Google）的商标，此处仅用于标识 QuotaDock 所展示
用量的对应服务。

---

<sub>keywords: Claude 用量监控 · Claude Code 速率限制 · OpenAI Codex 配额 · Gemini
Antigravity 用量 · AI 配额追踪 · 桌面小组件 · 系统托盘 · Windows · Go · Fyne</sub>
