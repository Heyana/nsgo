---
title: 변경 사항 만들기
description: 모든 백엔드에 걸친 일관된 변경부터 테스트, 문서, 변경 로그 한 줄까지 NSGo 풀 리퀘스트에 필요한 사항.
group: 기여하기
order: 82
---

## 시작하기 전에

단순한 버그 수정보다 큰 작업이라면 먼저 이슈나 토론을 열어 코드를 작성하기 전에 설계에 합의하세요. 설계상 열려 있는 질문들은 [ARCHITECTURE.md](https://github.com/nsgo-dev/nsgo/blob/main/ARCHITECTURE.md#open-questions)에 나열되어 있으며, 그곳에서 먼저 답을 결정합니다.

## 모든 백엔드

한 백엔드에서의 수정이나 기능은 보통 다른 백엔드에도 들어가야 합니다. 필요한 모든 대상에 동일한 변경을 적용하거나, 누락된 부분은 [로드맵](https://github.com/nsgo-dev/nsgo/blob/main/docs/roadmap.md)에 기록하세요. 컴포넌트는 모든 플랫폼에서 동일하게 보이고 동작해야 합니다.

## 검사

`go vet`, `go test`, `go build` 대신 `nsgo` 도구를 사용하세요.

```sh
go tool nsgo check   # vet every package
go tool nsgo test    # every package's tests
```

변경한 내용에 대한 테스트를 추가하세요. 사용자에게 보이는 부분은 `nsgotest`로, 나머지는 각 패키지의 자체 테스트로 작성합니다. 수정한 플랫폼에서 갤러리도 실행해 확인하세요([개발 환경 설정](../contributing.md#the-gallery)).

Go 파일은 `go.mod`에 지정된 Go 버전의 `gofmt`로 포맷하세요. `gofmt -l .`로 포맷이 필요한 파일 목록을 확인할 수 있습니다. CI는 추적되는 모든 Go 파일을 검사하며 포맷되지 않은 파일이 있으면 파일 이름과 함께 실패합니다.

## 풀 리퀘스트

스쿼시 병합 시 제목이 커밋 제목이 되므로, 제목은 `fix(linux): keep the caret after a paste`처럼 Conventional Commit 형식으로 작성하세요. 형식에 맞지 않으면 검사가 실패합니다. PR 템플릿에는 변경이 영향을 미치는 대상 플랫폼, 눈에 보이는 변경의 전후 스크린샷, 실행한 검사 목록을 적도록 되어 있습니다.

## 문서와 변경 로그

- 사실 관계가 바뀐 문서를 업데이트하세요. NSGo의 내부 동작 방식은 `docs/architecture/`의 해당 주제에, 앱과 컴포넌트의 사용법은 `docs/public/`의 해당 페이지에 반영합니다([문서와 웹사이트](docs.md)).
- `README.md`와 `README.zh-CN.md`의 내용을 동일하게 유지하세요. 그 외의 문서는 영어를 기본으로 하며 중국어 페이지는 `docs/public/zh/`에 있습니다.
- 사용자에게 보이는 변경 사항은 `CHANGELOG.md`에 직접 적지 말고(`CHANGELOG.md`는 릴리스 시 채워짐), `changelog.d/` 아래 별도의 파일에 `### Fixed` 같은 유형 제목 아래 `(#12 by @you)`와 같은 참조를 포함한 한 줄로 기록하세요([형식](https://github.com/nsgo-dev/nsgo/blob/main/changelog.d/README.md)).
