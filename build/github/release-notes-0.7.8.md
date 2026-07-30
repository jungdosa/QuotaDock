QuotaDock 0.7.8 — first public release.

**QuotaDock** is a tiny always-on-top Windows desktop widget that monitors your
**Claude, OpenAI Codex, and Google Antigravity (Gemini)** usage limits — session/weekly
quotas and reset timers — in one glance. Built with Go + Fyne. No telemetry, no
credential handling: it reuses your already-signed-in official CLIs/IDE locally.

Claude Code · Codex CLI · Antigravity IDE의 세션/주간 사용량과 리셋 타이머를 한 화면에서
보여주는 상시 표시 위젯입니다.

## Highlights

- 일반 / 컴팩트 / 나노 3가지 위젯 화면 + 설정 화면 (라이트·다크·시스템 테마, 한국어·영어)
- 사용량 분절 막대 + 리셋 잔여 시간 막대(항상 남은 시간 기준)를 나란히 표시
- 컴팩트: 리셋 잔여 시간 컬럼 + hover 시 리셋 시각 툴팁 / 나노: 행 hover 3줄 툴팁
- 경고·위험 임계값 색 전환, 제공자별 색상 팔레트, 위험 수치 굵게(색약 대비)
- 공급자별 표시 토글, 12/24시간 날짜 형식, 트레이 상주 (`✕` = 트레이로)

## Files

| File | Purpose |
|---|---|
| `QuotaDock-0.7.8-win-x64-Setup.exe` | Installer (Start Menu, optional launch at startup) |
| `QuotaDock-0.7.8-win-x64-portable.exe` | Portable single executable |
| `SHA256SUMS.txt` | Checksums |

Binaries are unsigned — verify with `SHA256SUMS.txt`. Windows 10 22H2+ / 11, x64.
