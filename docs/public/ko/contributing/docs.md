---
title: 문서와 웹사이트
description: 문서 페이지 작성, 번역, 웹사이트 실행 및 에이전트용 스킬 최신 유지.
group: 기여하기
order: 83
---

## 페이지

이 문서는 `docs/public/`에 있는 마크다운 파일들로, GitHub에서도 그대로 읽을 수 있습니다. `en/`과 `zh/`는 경로 구조가 일대일로 대응하며, 중국어 번역이 없는 페이지는 안내 문구와 함께 영어 페이지를 표시합니다. 각 페이지는 프론트 매터로 시작합니다.

```yaml
---
title: Testing
description: Test an app the way a user uses it, headless or in a native window.
group: Toolchain
order: 21
---
```

- `group`은 사이드바에서 페이지가 속하는 그룹으로 해당 페이지의 언어로 작성하며, `order`는 정렬 순서를 정합니다. 그룹은 독자가 접하는 순서대로 튜토리얼, 툴체인, 컴포넌트, 핵심 개념, 컴포넌트 작성, 기여하기 순으로 배치됩니다. 소개 페이지처럼 `group`이 없는 페이지는 이들보다 앞에 놓입니다.
- 레이아웃이 `title`과 `description`을 화면에 표시하므로 본문은 `##`부터 시작합니다.

페이지 간 연결에는 상대 `.md` 링크(`[테스트](testing.md#run-on-a-platform)`)를 사용하세요. 사이트가 이를 자체 URL로 변환합니다. 각 페이지는 하나의 질문에 답하며, NSGo의 내부 동작 방식은 `docs/architecture/`에 둡니다.

## 웹사이트

`website/`의 웹사이트는 Astro 통합 패키지인 [nsgo-website](https://github.com/nsgo-dev/nsgo-website)가 `website/site.config.ts`의 설정을 읽어 빌드합니다. 실행하려면 [Bun](https://bun.com) 1.4 이상이 필요합니다.

```sh
cd website
bun install
bun run dev    # at http://localhost:4321/
bun run build  # type-check and build, as the deployment does
```

사이트 빌드 및 배포 방법은 `website/README.md`에 나와 있습니다.

## 에이전트용 문서

웹사이트는 언어 모델을 위해 영어 페이지도 함께 게시합니다. `/llms.txt`는 페이지 목록을, `/llms-full.txt`는 전체 내용을, `/docs/<page>.md`는 절대 링크를 사용하고 코드 블록 마크를 제거한 개별 페이지를 제공합니다(`src/lib/llms.ts`). 이 파일들은 별도 작업 없이 문서 페이지를 그대로 따릅니다.

`skills/nsgo/`는 에이전트가 `bunx skills add nsgo-dev/nsgo`로 설치하는 스킬입니다. `SKILL.md`에는 앱이 따라야 할 규칙이 들어 있고 `references/`는 작업에 필요할 때 읽는 참고 문서입니다. 앱 코드 작성 방식, CLI 도구 명령, 테스트 또는 패키징이 바뀔 때는 문서와 함께 이 스킬도 수정하세요. 에이전트가 반드시 지켜야 할 내용만 남기고 나머지는 문서로 링크합니다. `go tool nsgo test -run TestSkills .`가 프론트 매터, 링크, 그리고 언급된 NSGo의 모든 이름을 검사합니다.
