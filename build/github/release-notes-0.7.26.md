QuotaDock 0.7.26 — 크레딧 표시, 12개 언어, 자동 복구.

**QuotaDock** is a tiny always-on-top Windows desktop widget that monitors your
**Claude, OpenAI Codex, and Google Antigravity (Gemini)** usage limits — session/weekly
quotas and reset timers — in one glance. Built with Go + Fyne. No telemetry, no
credential handling: it reuses your already-signed-in official CLIs/IDE locally.

0.7.15 이후 열 번의 내부 빌드를 거친 누적 릴리스입니다.
This release accumulates ten internal builds since 0.7.15.

## Added

**Claude 크레딧(유료 추가 사용량) 표시.** 일반 모드 레인 헤더에 잔액이 뜨고, 설정에서
공급자별로 켜고 끌 수 있습니다. 새 인증이나 추가 로그인은 필요 없습니다 — 앱이 이미 받고
있던 응답에 들어 있던 값입니다.
Claude credit balance now appears in the lane header, toggleable per provider in Settings.
No new sign-in: the data was already in the response the app receives.

**일본어·중국어 간체·번체 추가 — 총 12개 언어.** 한국어·영어·독일어·프랑스어·이탈리아어·
인도네시아어·포르투갈어(브라질)·스페인어(스페인/중남미)에 이어집니다. 표시 언어는 Windows
설정을 따르며 설정에서 직접 고를 수도 있습니다.
Japanese, Simplified Chinese and Traditional Chinese join the eight existing languages,
for twelve in total.

**트레이 아이콘 툴팁에 사용량 요약.** 창을 열지 않고 마우스만 올려도 현재 사용률을 봅니다.
Hovering the tray icon now summarises current usage without opening the window.

## Fixed

**Codex 연결이 끊긴 뒤 스스로 돌아옵니다.** 이전에는 한 번 끊기면 설정에서 "다시 연결"을
직접 눌러야 했습니다. 갱신 실패가 이어지면 다음 갱신이 재연결부터 시작합니다. 일시적인
오류에 세션을 버리지 않도록 첫 실패는 한 번 봐주고, 재연결이 도움이 되지 않는 상태
(CLI 미설치·구버전·로그인 안 됨)에서는 헛되이 재시도하지 않습니다.
Codex now recovers a dropped connection on its own instead of waiting for a manual
"Reconnect". Transient failures get one grace attempt; states a reconnect cannot fix are
left alone.

**모니터 구성이 바뀌면 창이 화면 안으로 돌아옵니다.** 모니터를 끄거나 해상도를 바꿔 창이
보이지 않는 영역에 남는 경우를 처리합니다.
The window returns onto a visible work area when the monitor layout changes.

**창이 비어 보이면 스스로 다시 그립니다.** 드물게 시작 직후 창은 떠 있는데 내용이 그려지지
않는 경우가 있었습니다. 시작 후 세 번에 걸쳐 실제로 그려졌는지 확인하고, 비어 있으면
다시 그립니다.
If the window comes up blank — rare, but observed after an unclean shutdown — the app now
notices and repaints itself.

## Changed

**모드 전환 아이콘을 다듬었습니다.** 일반·컴팩트·나노 세 아이콘의 크기 서열이 한눈에
들어오도록 치수를 정리했습니다.
The display-mode icons were resized so the size hierarchy reads at a glance.

**문제 진단용 로그를 남깁니다.** `%LOCALAPPDATA%\QuotaDock\quotadock.log`에 앱 시작·종료,
공급자 갱신 성공 여부, 연결 상태 변화가 기록됩니다. **사용량 수치·계정·토큰은 기록하지
않습니다.** 크기는 1 MB로 제한되며 외부로 전송되지 않습니다.
A local diagnostic log is written to `%LOCALAPPDATA%\QuotaDock\quotadock.log`. It records
start/exit, refresh outcomes and connection-state changes — never usage figures, accounts
or tokens. Capped at 1 MB and never transmitted anywhere.

## Files

| File | Purpose |
|---|---|
| `QuotaDock-0.7.26-win-x64-Setup.exe` | Installer (Start Menu, optional launch at startup) |
| `QuotaDock-0.7.26-win-x64-portable.exe` | Portable single executable |
| `SHA256SUMS.txt` | Checksums |

Binaries are unsigned — verify with `SHA256SUMS.txt`. Windows 10 22H2+ / 11, x64.
