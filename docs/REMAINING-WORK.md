# 잔여작업 (GitHub 공개 이후)

## 크레딧(유료 추가 사용량) 표시 활성화

- 코드는 구현 완료 상태로 `internal/ui/view_cache.go`의 `creditsDisplayEnabled = false`로 게이트되어 있다.
  활성화하면: 일반 모드 레인 헤더의 `크레딧 <잔액>` 메타 텍스트, 설정 CONNECTIONS의 크레딧 항목,
  설정의 제공자별 `크레딧` 토글(`ShowClaudeCredits`/`ShowCodexCredits`)이 켜진다.
- Codex 데이터는 이미 수신 중(app-server `credits { balance, unlimited }` → `LaneState.Credits`).
  app-server 응답에는 리셋권 개수(`rateLimitResetCredits.availableCount`)와 `hasCredits`도
  포함되지만 QuotaDock은 아직 파싱하지 않는다. 착수 시 실제 응답을 덤프해 확인할 것.
### Claude 크레딧 — 브라우저 로그인이 필요 없다 (2026-07-31 실측 확인)

기존 계획은 "브라우저 로그인을 개발해야 Claude 크레딧을 얻는다"였으나 **틀렸다.**
`api.anthropic.com/api/oauth/usage`가 **이미 크레딧 데이터를 함께 돌려준다.** 앱이 5분마다
받는 바로 그 응답인데, 현재 파서가 `five_hour`·`seven_day`·`limits`만 읽고 나머지를 버린다.

응답에 포함된 미소비 필드:

```
extra_usage: is_enabled · monthly_limit · used_credits · utilization · currency ·
             decimal_places · disabled_reason · user_disabled · spend_limit_reached ·
             credits_ever_enabled · daily · weekly
spend:       used{amount_minor,currency,exponent} · limit{...} · percent · severity ·
             enabled · cap.credits{amount_minor,exponent} · balance · auto_reload ·
             can_purchase_credits · can_toggle
```

따라서 이 작업은 **파서 확장 + UI 게이트 해제**로 끝난다. 새 인증, 쿠키 저장소, 웹뷰 도입이
모두 불필요하며 "비밀정보를 다루지 않는다"는 설계 원칙도 그대로 유지된다.

구현 시 주의:

- **`limit_dollars`·`used_dollars`는 `null`이다.** 금액은 `spend.used.amount_minor`(정수)와
  `exponent`로 오므로 `amount_minor / 10^exponent`로 환산한다. 부동소수점으로 바로 다루지 말 것.
- `extra_usage.utilization`이 백분율을 직접 준다. 자체 계산으로 대체하지 말 것.
- 크레딧 미사용 계정에서는 `extra_usage.is_enabled=false`, `spend.balance=null`이 될 수 있다.
  null 안전하게 처리하고, 값이 없으면 크레딧 표시를 조용히 생략한다.
- 토큰 스코프에 결제 항목이 없는데도(`user:profile`·`user:inference` 등) 이 필드가 오므로,
  스코프만 보고 불가능하다고 판단하지 말 것.

## 업데이트 버튼 활성화

- 현재 설정 타이틀바의 `업데이트` 버튼은 비활성(`업데이트 기능 준비 중`) 상태다.
- GitHub 저장소 공개 후 GitHub Releases를 업데이트 채널로 사용해 활성화한다.
  - 최신 릴리스 조회: `GET https://api.github.com/repos/jungdosa/QuotaDock/releases/latest`
  - 현재 버전(`main.version`)과 비교해 새 버전이 있으면 버튼을 활성화하고,
    클릭 시 릴리스 페이지(또는 Setup.exe 자산)를 기본 브라우저로 연다.
  - 네트워크 접근은 기존 보안 원칙(허용된 호스트만, 자격 증명 미노출)을 따른다.
- 관련 i18n 키: `action.update`, `action.update_pending`, `tooltip.update_pending`.

## Grok 공급자 추가 (네 번째 공급자)

### 선행 조사 결과 (2026-07-31)

게이트 항목을 조사했다. **일부는 통과, 결정적인 하나가 미확인이다.**

| 게이트 | 결과 |
|---|---|
| 공식 CLI가 있는가 | **있다.** xAI `Grok Build`(바이너리 `grok`), 2026-05-14 출시. `~/.grok-build/`에 설정, `grok-build login`으로 인증 |
| 사용량을 기계 판독 가능하게 출력하는가 | **미확인.** `/usage`·`/context`·`/session-info`는 대화형 슬래시 명령이고, 비대화형 조회 경로가 확인되지 않았다 |
| 공식 API에 사용량 조회 엔드포인트가 있는가 | **미확인.** API 키 사용 시에는 구독 할당량이 아니라 xAI API 레이트리밋이 적용된다 |
| 구독 등급 라벨 | SuperGrok · X Premium+ · SuperGrok Heavy · SuperHeavy(프로모션). 공식 라벨 문자열은 미확인 |
| 한도 창 구조 | **불리하다.** 고정 일일 할당량이 공개 표로 제공되지 않으며 롤링 방식으로 보인다. 5시간/주간 창 구조와 다르다 |

**착수 조건**: 이 PC에 `grok` CLI가 설치돼 있지 않아 실측을 못 했다. 채택을 검토하려면 먼저
CLI를 설치하고 로그인한 뒤 다음을 실측해야 한다.

1. 비대화형으로 사용량을 얻는 방법이 있는가 (`grok --output-format streaming-json` 계열로
   `/usage`에 해당하는 값을 뽑을 수 있는가, 또는 로컬 설정·캐시 파일에 남는가)
2. 그 출력에 **리셋 시각**이 포함되는가. QuotaDock의 핵심 표시가 "리셋까지 남은 시간"이라
   이 값이 없으면 리셋 바를 그릴 수 없다
3. 등급 라벨의 정확한 문자열

**금지 사항은 그대로다**: 브라우저 쿠키 추출, 세션 토큰 붙여넣기 요구, 비공식 웹 스크래핑,
사용량 조회를 위한 유료 API 요청 발생. 조건을 만족하는 경로를 찾지 못하면 **채택하지 않는다.**

한도 창이 롤링 구조라면 기존 세 공급자와 표시 문법이 어긋난다. 그 경우 레인을 추가하기보다
표시 방식을 먼저 설계해야 한다.


xAI의 **Grok**을 네 번째 공급자로 추가하는 안. 기존 세 공급자의 실제 연결이 안정화된 뒤에 착수한다.

**착수 전 선행 조사가 필수다.** 기존 세 공급자는 각각 검증된 로컬 인터페이스가 있어서 채택했다(Claude 공식 CLI, Codex 공식 app-server, Antigravity 로컬 언어 서버). Grok에 대응하는 인터페이스가 존재하는지 먼저 확인한다.

- 공식 CLI가 있는가. 있다면 인증 상태와 사용량을 기계 판독 가능한 형태로 출력하는가.
- 공식 API에 사용량·한도 조회 엔드포인트가 있는가. 제3자 도구 사용이 이용약관상 허용되는가.
- 구독 등급(요금제) 라벨의 공식 목록은 무엇인가.
- 한도 창(window) 구조는 5시간/주간 형태인가, 다른 주기인가.

보안·인증 UX 원칙을 만족하는 경로를 찾지 못하면 **채택하지 않는다.** 다음은 금지다: 브라우저 쿠키 추출, 세션 토큰 붙여넣기 요구, 비공식 웹 스크래핑을 기본 사용량 소스로 채택, 사용량 조회를 위해 유료 API 요청을 발생시키는 방식.

채택 시 파급 범위:

| 영역 | 영향 |
|---|---|
| 일반 위젯 | 레인 1개 추가 |
| 컴팩트 | 아이콘 1개, 행 추가 |
| 나노 모드 | 열 4개 → 5개. 창 폭 재계산 필요 |
| 설정 | 표시 토글, 색상 선택 각 1개 추가 |
| 기본 색상 | 다섯 공급자의 기본색이 서로 달라야 한다. 경고 앰버·위험 레드 계열은 금지 |
| i18n | 라벨과 도움말 문구 추가 |
| 설정 스키마 | `providerColors`와 표시 토글 키 추가 |
| 테스트 | 공급자 격리 테스트에 Grok 포함 |

## macOS · Linux 확장

- **macOS**: 서명·공증 절차는 `docs/reference/macos-signing-notes.md` 참고. Fyne 검증
  게이트의 macOS 항목(frameless 창, 트레이, 다중 모니터)을 먼저 통과시킨다.
- **Linux**: 트레이 구현이 데스크톱 환경별로 갈리므로 대상 환경을 먼저 정한다.
- 두 OS 모두 **별도 원격 clone**으로 작업하고, 클라우드 동기화 폴더 안에서 빌드하지 않는다.

## 유지해야 할 설계 원칙 (행 라벨)

행 라벨에서 **상위 맥락이 이미 알려주는 정보를 반복하지 않는다.** 공급자 레인 헤더나 아이콘이 공급자를 알려주면 행 라벨은 기간만 말한다. 한 레인 안에 여러 모델 그룹이 있을 때만 그룹 이름을 붙인다. 컴팩트와 나노는 같은 아이콘 체계를 공유한다(두 모드가 다른 아이콘을 쓰면 학습 비용이 두 배가 된다).
