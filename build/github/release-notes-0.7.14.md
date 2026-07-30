QuotaDock 0.7.14 — 9개 언어 지원과 화면 가장자리 잘림 수정.

**QuotaDock** is a tiny always-on-top Windows desktop widget that monitors your
**Claude, OpenAI Codex, and Google Antigravity (Gemini)** usage limits — session/weekly
quotas and reset timers — in one glance. Built with Go + Fyne. No telemetry, no
credential handling: it reuses your already-signed-in official CLIs/IDE locally.

## Fixed

**화면 가장자리에서 설정 창이 잘리던 문제** — 위젯을 화면 오른쪽 끝이나 오른쪽
아래 구석에 두고 설정을 열면 창이 커지면서 화면 밖으로 밀려 내용이 잘렸습니다.
이제 작업 영역 안으로 자동 보정되며, 설정을 닫으면 위젯이 원래 자리로 돌아갑니다.
나노 → 일반처럼 크기가 커지는 모든 화면 전환에 적용됩니다.

## Added

### 9개 언어 지원

영어·한국어에 더해 **독일어·프랑스어·이탈리아어·인도네시아어·포르투갈어(브라질)·
스페인어(스페인/라틴아메리카)**를 추가했습니다. 설정 → 표시·언어에서 고를 수 있고,
`시스템`을 선택하면 Windows 표시 언어를 따릅니다.

설치 프로그램도 독일어·프랑스어·이탈리아어·포르투갈어(브라질)·스페인어를
지원합니다.

### 트레이 아이콘을 작업 표시줄에 고정 (Windows 11)

설정 → 일반 동작의 `작업 표시줄에 고정`을 켜면 아이콘이 오버플로(∧)에 숨지 않고
작업 표시줄에 항상 표시됩니다. 권한 상승이 필요 없습니다.

## Files

| File | Purpose |
|---|---|
| `QuotaDock-0.7.14-win-x64-Setup.exe` | Installer (Start Menu, optional launch at startup) |
| `QuotaDock-0.7.14-win-x64-portable.exe` | Portable single executable |
| `SHA256SUMS.txt` | Checksums |

Binaries are unsigned — verify with `SHA256SUMS.txt`. Windows 10 22H2+ / 11, x64.
