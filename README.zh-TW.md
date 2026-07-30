# QuotaDock

[English](./README.md) | [한국어](./README.ko-KR.md) | [简体中文](./README.zh-CN.md) | [繁體中文（臺灣）](./README.zh-TW.md) | [日本語](./README.ja-JP.md)

QuotaDock 是一款常駐最上層的輕巧 Windows 桌面小工具，讓你一眼掌握 **Claude、OpenAI Codex 與
Google Antigravity** 的用量額度 —— 工作階段/每週配額，以及重設倒數。

[![Release](https://img.shields.io/github/v/release/jungdosa/QuotaDock?include_prereleases&label=release)](https://github.com/jungdosa/QuotaDock/releases)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Fyne](https://img.shields.io/badge/GUI-Fyne-orange)](https://fyne.io/)
[![Platform](https://img.shields.io/badge/platform-Windows%2010%2F11-0078D4?logo=windows&logoColor=white)](#支援平台)

<p align="center">
  <img src="docs/marketing/quotadock-normal-en.png" alt="QuotaDock 一般檢視 —— Claude / Codex / Antigravity 用量與重設倒數" width="539">
</p>

使用 Claude Code、Codex CLI 與 Antigravity IDE 時，你總是在問同樣的問題：*5 小時的工作階段還剩多少？
每週上限什麼時候重設？* QuotaDock 幫你盯著，不必再一個個開儀表板查看。

| 服務供應商 | 顯示內容 | 連線方式 |
|---|---|---|
| **Claude**（Claude Code） | 5 小時工作階段 · 7 天每週 · Fable 每週 | 沿用官方 Claude CLI 的登入認證 |
| **OpenAI Codex** | 工作階段 · 每週上限 | 官方 Codex CLI app-server（stdio JSONL） |
| **Google Antigravity** | Gemini 與 Claude/GPT 群組的工作階段 · 每週 | 本機語言伺服器（loopback） |

即使其中一個供應商連線失敗，其餘仍照常運作。每一列都會在**分段長條（已用量）**下方繪製一條
**細長連續長條（距離重設的剩餘時間）**，讓你一眼判斷額度是否消耗得比時間更快。超過警告或危險
門檻值時，長條與數值會切換成警示色。

## 檢視模式

三種小工具檢視外加設定畫面。工具列按鈕會依 `一般 → 精簡 → 迷你` 循環切換，迷你檢視只要點一下
即可回到精簡檢視。

### 一般 —— 資訊最完整

依供應商分組，顯示方案標章、用量長條與重設長條、倒數計時與重設時刻。

<p align="center"><img src="docs/marketing/quotadock-normal-en.png" alt="一般檢視（深色）" width="480"></p>

### 精簡 —— 每列一行

較窄的檢視，百分比旁邊直接顯示距離重設的剩餘時間。將滑鼠停留在時間上會出現精確重設時刻的
提示訊息。

<p align="center"><img src="docs/marketing/quotadock-compact-en.png" alt="精簡檢視（深色）" width="312"></p>

### 迷你 —— 極小長條

每個供應商佔一格，高度約 78px，適合固定在螢幕角落。滑鼠停留在某一列時會顯示三行提示：
`供應商 · 區間 / 剩餘時間 / 重設時刻`。

<p align="center"><img src="docs/marketing/quotadock-nano-en.png" alt="迷你檢視（深色）" width="360"></p>

## 功能

- **淺色 · 深色 · 跟隨系統佈景主題**，12/24 小時日期格式，各供應商可個別配色，警告門檻值可調整
- **九種語言** —— 英文、韓文、德文、法文、義大利文、印尼文、葡萄牙文（巴西）、
  西班牙文（西班牙/拉丁美洲）。選擇 `系統` 則跟隨 Windows 顯示語言
- **連線方式按鈕** —— 每個供應商列都會顯示它透過哪種方式（`CLI`、`Auth`、`IDE`）連線及目前狀態。
  若缺少對應的 CLI，點一下按鈕就會在卡片內展開安裝說明
- **內建更新** —— 啟動時檢查一次 GitHub Releases，也可隨時手動檢查。下載的檔案會與發行資產的
  SHA-256 比對**兩次**：下載完成後一次，執行安裝程式前再一次。不做定期輪詢，也不送出任何認證資訊
- **釘選到工作列（Windows 11）** —— 讓系統匣圖示不會被收進溢位區域

## 安裝

請至 [**Releases**](https://github.com/jungdosa/QuotaDock/releases) 下載。

| 檔案 | 用途 |
|---|---|
| `QuotaDock-<版本>-win-x64-Setup.exe` | 安裝版 —— 建立開始功能表項目，可選擇開機時啟動 |
| `QuotaDock-<版本>-win-x64-portable.exe` | 免安裝單一執行檔 |

執行檔未經程式碼簽署，SmartScreen 可能會出現警告。請用 `SHA256SUMS.txt` 驗證完整性。
不需要任何設定 —— QuotaDock 會自動尋找你已登入的官方 CLI 與 IDE 並連線。

> 目前版本為 `0.7.15`。完成 Windows 功能驗證後將提升為 `1.0.0`。

## 設計原則

- **不打擾。** 只要官方工具已安裝並登入，它就會自動連線。沒有首次啟動的登入精靈，也沒有強制彈出視窗。
- **不碰機密資訊。** 權杖、Cookie 與 `auth.json` 原文不會進入介面或記錄檔。傳給介面的只有正規化後的
  用量比例、經允許清單驗證的方案標籤，以及重設時刻。
- **只連本機。** 沒有遙測，也沒有外部分析伺服器。loopback 連線僅允許連往已驗證的處理程序與固定端點。
  唯一的對外請求是更新檢查，不夾帶任何認證資訊，且只在啟動時或你點選時發生。
- **不消耗額度。** 更新用量絕不會送出需要計費的 AI 請求。
- **夠輕巧。** Go + Fyne 原生繪製，閒置時 CPU 佔用為 0%，記憶體也主動管理。

## 支援平台

| 優先順序 | 平台 | 狀態 |
|---|---|---|
| 第一 | Windows 10 22H2+ / 11, x64 | **可用**（0.7.x） |
| 第二 | macOS 14+, Apple Silicon | 規劃中 |
| 第三 | Linux x64 → arm64 | 規劃中 |

## 從原始碼建置

Fyne 使用 CGO，因此光有 Go 無法完成建置。

- Go 1.26 以上
- C 編譯器（Windows：MinGW-w64 / macOS：Xcode Command Line Tools / Linux：gcc + X11 開發標頭檔）

```sh
git clone https://github.com/jungdosa/QuotaDock.git
cd QuotaDock
go build ./cmd/quotadock
```

發行產物（Setup.exe、portable、SHA256SUMS）由 `build/windows/build-release.ps1` 產生。

## 授權條款

MIT —— 請參閱 [LICENSE](LICENSE)。

QuotaDock 的原始碼、文案、畫面設計與應用程式圖示皆為原創。後續工作規劃記錄於
[docs/REMAINING-WORK.md](docs/REMAINING-WORK.md)。

### 第三方資源

| 資源 | 來源 | 授權 |
|---|---|---|
| `assets/fonts/Pretendard-*.ttf` | [Pretendard](https://github.com/orioncactus/pretendard) | SIL OFL 1.1 —— [`Pretendard-OFL.txt`](assets/fonts/Pretendard-OFL.txt) |
| `assets/providers/*.svg` | [lobe-icons](https://github.com/lobehub/lobe-icons) | MIT —— [`LICENSE-lobe-icons.txt`](assets/providers/LICENSE-lobe-icons.txt) |

供應商標誌為各自所有者（Anthropic、OpenAI、Google）的商標，此處僅用於識別 QuotaDock 所顯示
用量的對應服務。

---

<sub>keywords: Claude 用量監控 · Claude Code 速率限制 · OpenAI Codex 配額 · Gemini
Antigravity 用量 · AI 配額追蹤 · 桌面小工具 · 系統匣 · Windows · Go · Fyne</sub>
