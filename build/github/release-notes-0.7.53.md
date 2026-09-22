QuotaDock 0.7.53 — 최대 5개의 Claude 계정, 제공자 순서 변경, 세로 나노, Codex 한도 정정.

**QuotaDock** is a tiny always-on-top Windows desktop widget that monitors your
**Claude, OpenAI Codex, Google Antigravity (Gemini), and Grok** usage limits — session and
weekly quotas with their reset timers — in one glance. Built with Go + Fyne. No telemetry:
it reads the credentials your official CLIs and IDE already hold, and only ever calls their
usage endpoints.

0.7.38 이후의 누적 릴리스입니다.
This release accumulates the work since 0.7.38.

## Added

**Claude 계정을 최대 5개까지 나란히 봅니다.** 설정 › 연결의 마지막 Claude 카드에 `+`로
계정을 더하고 `−`로 마지막 계정을 뺍니다. 계정마다 표시 이름과 색을 따로 두고, 세 번째
계정부터는 각자의 브라우저 프로필을 씁니다.
Up to five Claude accounts side by side, each with its own display name and colour; a third
account onward gets its own browser profile.

**제공자 순서를 드래그로 바꿉니다.** 일반·컴팩트 창에서 제공자 묶음을 위아래로 끌면
순서가 바뀌고, 그 순서를 모든 모드와 트레이 툴팁·연결 카드가 함께 씁니다.
Drag a provider group up or down to reorder it; every mode, the tray tooltip and the
connection cards share that order.

**나노 모드를 세로로 세울 수 있습니다.** 타이틀바 토글로 카드를 아래로 쌓고 조작 버튼을
오른쪽 띠에 모읍니다. 화면 옆에 붙여 두기 좋습니다.
Nano can stand upright, stacking its cards down the window with the actions in a strip on
the right.

**일반 창의 제공자 이름 앞에 브랜드 마크가 붙습니다.**
The provider name in the normal window now leads with its brand mark.

## Changed

**일반 창이 촘촘해졌습니다.** 행 간격과 높이를 줄이고 재설정 열을 고정 폭 대신 실제로
필요한 폭으로 재면서, 그만큼을 사용량 막대에 돌려줬습니다.
The normal window is tighter, and the room it saves goes to the meters.

**일시적인 조회 실패로 레인이 비지 않습니다.** 최대 세 번까지 직전의 정상 값을 유지하다가
그래도 실패하면 오류를 표시합니다. 로그인 만료처럼 사용자가 조치해야 하는 실패는 즉시
표시합니다.
A lane rides out a failed refresh on its last good reading for up to three refreshes;
failures that need you, such as an expired sign-in, still show at once.

**README의 처음 실행 절에 Claude 연결 세 가지 방식을 설명했습니다** — CLI, 앱 내 로그인,
그리고 `CLAUDE_CODE_OAUTH_TOKEN` 환경변수.

## Fixed

**Codex 한도가 잘못 나오던 문제를 고쳤습니다.** 계정에 모델 전용 한도(예 GPT-5.3-Codex-Spark)가
있으면 일반 주간 한도와 창 길이가 같다는 이유로 한 줄로 합쳐졌고, 사용률이 높은 모델 쪽이
남으면서 **정작 중요한 일반 한도가 화면에서 사라졌습니다.** 이제 한도마다 제 줄을 가집니다 —
일반 한도가 위, 모델별 한도가 그 아래에 모델 이름과 함께 놓입니다.
Codex showed a model's figures in place of the account's own. Each limit now keeps its own
row, the account-wide one first.

**앱 내 로그인 방식의 Claude 계정이 세 번에 한 번꼴로 시간 초과되던 문제를 고쳤습니다.**
두 요청이 각각 브라우저를 열어 두 번째가 종료 중인 첫 번째에 붙던 경합이었습니다.
The browser-backed Claude account no longer times out on roughly one refresh in three.

**나노 모양이 바뀔 때 창이 곧바로 줄어듭니다.** 나노로 저장한 창이 일반 크기로 먼저 떴다가
줄어들던 것도 함께 고쳤습니다.

## Files

| File | Purpose |
|---|---|
| `QuotaDock-0.7.53-win-x64-portable.exe` | Portable single executable — no installation |
| `QuotaDock-0.7.53-win-x64-Setup.exe` | Installer (Start Menu, optional launch at startup) |
| `SHA256SUMS.txt` | Checksums |

Binaries are unsigned, so SmartScreen may warn. Verify with `SHA256SUMS.txt`, or build from
source. Windows 10 22H2+ / 11, x64.
