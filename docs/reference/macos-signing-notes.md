# macOS 서명·공증 참고 (Phase 4 대비)

작성일: 2026-07-30 · 대상: QuotaDock Go/Fyne

Phase 4(macOS) 착수 전에 필요한 **계정·자격 요건과 공증 흐름**을 정리한 문서다.
실제 Apple Team ID 등 계정 식별값은 이 저장소에 두지 않는다. 개발자의 로컬 기록을 참조한다.

## 왜 필요한가

| 상태 | 사용자 경험 |
|---|---|
| 서명 없음 | Gatekeeper가 "손상되어 열 수 없음"으로 차단 |
| 서명만 함 | 보안 설정 우회를 사용자에게 요구 |
| 서명 + 공증 | 경고 없이 설치 |

## 사전 준비물

1. **Apple Developer Program 계정** — 연 $99, https://developer.apple.com/programs/
2. **Developer ID Application 인증서** — Xcode(설정 → 계정 → 인증서 관리) 또는
   developer.apple.com에서 발급한 뒤 Keychain Access에서 `.p12`로 내보낸다(내보낼 때 암호 설정).
3. **App-Specific Password** — https://appleid.apple.com → 로그인 및 보안 → 앱 암호.
   생성 직후 한 번만 표시되므로 즉시 보관한다.
4. **Team ID** — developer.apple.com/account → Membership에 표시되는 10자 문자열

## 공증 흐름

1. 바이너리에 Developer ID로 서명
2. 배포 형식(`.dmg` 또는 `.zip`)으로 패키징
3. Apple에 공증 제출 — 통상 3~25분 소요
4. 승인 후 공증 티켓을 배포물에 staple
5. 최종 산출물 검증

## Go/Fyne 기준 실행 방법

- 패키징은 `fyne package -os darwin`으로 `.app` 번들을 만든다. 서명·공증을 자동으로 처리하는
  파이프라인이 없으므로 두 단계를 **직접 호출**해야 한다.
- 서명: `codesign --deep --force --options runtime --sign "Developer ID Application: …" QuotaDock.app`
  (`--options runtime`으로 Hardened Runtime 활성화 — 공증 필수 조건)
- 공증: `xcrun notarytool submit --wait` → `xcrun stapler staple`
  (구 `altool`은 폐지되었다)
- CGO 기반이므로 **대상 아키텍처별로 빌드**해야 한다(arm64 / amd64). 유니버설 바이너리가
  필요하면 각각 빌드해 `lipo`로 합친다.
- 자격값은 환경변수 또는 keychain profile(`notarytool store-credentials`)로 주입하고
  저장소에 커밋하지 않는다.

## 착수 전 확인할 것

- [ ] Fyne 검증 게이트의 macOS 항목(frameless 창, 트레이, 다중 모니터)
- [ ] macOS는 별도 원격 clone으로 작업 — 클라우드 동기화 폴더 안에서 작업하지 않는다
- [ ] 서명 없이 먼저 기능 동등성을 확인한 뒤 서명·공증을 붙인다
