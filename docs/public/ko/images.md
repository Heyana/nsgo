---
title: 이미지와 아이콘
description: URL, 앱 리소스, 바이트 또는 앱이 직접 그린 픽셀로 이미지를 표시하고 Lucide 아이콘과 SVG를 그립니다.
group: 핵심 개념
order: 54
---

## 이미지

`core.Image`는 URL, 파일, 앱 리소스 내 `asset:` 경로 또는 앱이 보유한 바이트로부터 비트맵이나 SVG를 표시합니다. 플랫폼이 메인 스레드 밖에서 이미지를 로드, 디코딩 및 캐시하며, 메인 스레드가 그 결과를 그립니다.

```go
core.Image{
	Source: core.Src("https://example.com/cover.jpg"),
	ResizeMode: core.Cover,
	Style: core.Style{Width: core.Pt(160), Height: core.Pt(100), CornerRadius: 12},
}
```

`core.Cover`는 상자를 채우고 넘치는 부분을 자릅니다. `core.Contain`은 이미지 전체를 상자 안에 맞추고, `core.Center`는 원래 크기를 유지하며, `core.ScaleDown`은 이미지가 상자보다 크지 않은 한 원래 크기를 유지합니다. `BlurRadius`는 포인트 단위로 이미지를 흐리게 만들고, `TintColor`는 불투명한 픽셀의 색상을 다시 칠하며, `ObjectPosition`은 상자 안에서 이미지의 위치를 정합니다.

크기가 조정된 비트맵은 기본적으로 부드러운 샘플링을 사용합니다. 픽셀 아트와 픽셀 버퍼의 날카로운 경계를 유지하려면 Go에서는 `Sampling: core.ImageSamplingNearest`를, TSX에서는 `sampling="nearest"`를 설정하세요. Apple 플랫폼, Windows, Linux, Android, 웹이 이 설정을 지원합니다. HarmonyOS와 미니 프로그램도 이를 적용하지만 실제 기기에서의 출력은 아직 검증되지 않았습니다. CSS `pixelated`를 사용하는 브라우저에서 정확한 픽셀 경계를 얻으려면 원본 픽셀당 정수 배의 장치 픽셀 크기로 스케일링하세요.

`Source`는 레이아웃이 사용하는 크기를 선언합니다. 한쪽 치수가 있으면 다른 쪽 치수가 파생되며, 크기가 없는 이미지는 스타일로 크기를 지정하기 전까지 0×0 크기입니다. `core.ImageSource{Data: data, Size: core.Size{Width: 24, Height: 24}}`는 `go:embed`로 임베드한 바이트를 표시하며, `Size`는 SVG에 고유 크기를 부여합니다.

[`desktop.WindowPointerPainted`](services.md)와 함께 사용하면 `HitTestAlpha: 0.5`가 macOS와 Windows에서 불투명도가 절반 미만인 픽셀을 포인터가 통과하도록 합니다. TSX에서는 `hitTestAlpha`로 동일한 임계값을 설정합니다.

## 앱이 직접 그리는 프레임

`Pixels`는 아무것도 인코딩, 로드, 디코딩하지 않고 앱이 그린 `*image.RGBA`, 즉 비디오 플레이어나 에뮬레이터의 프레임을 표시합니다. 같은 버퍼에 각 프레임을 기록하고 `Version`을 올리면, 뷰는 로드 이벤트나 페이드 효과 없이 알파 채널을 유지한 채 표시하며 macOS, iOS, tvOS, watchOS, Android, Windows, Linux에서는 중간에 빈 프레임 없이 제자리에서 프레임을 교체합니다. `Size`가 없으면 픽셀 크기를 포인트 단위로 측정합니다.

```go
frame := image.NewRGBA(image.Rect(0, 0, 960, 540))
version := core.NewSignal(cx, uint64(0))
var tick func(time.Time)
tick = func(at time.Time) {
	if decoder.WriteDue(at, frame) { // 다음 프레임을 frame.Pix에 씁니다
		version.Set(version.Get() + 1)
	}
	cx.RequestAnimationFrame(tick)
}
cx.RequestAnimationFrame(tick)

core.Image{ResizeMode: core.Contain, SourceFunc: func() core.ImageSource {
	return core.ImageSource{Pixels: frame, Version: version.Get()}
}}
```

NSGo는 새 `Version`을 전달하는 커밋이 실행될 때 픽셀을 복사하므로, 앱은 다음 `RequestAnimationFrame` 콜백에서 바로 다음 프레임을 쓸 수 있습니다. 별도 고루틴의 디코더는 두 번째 버퍼에 쓰고 교체를 전달합니다. 동일한 `Version`은 다시 그리지 않습니다. `cx.ImageSize`는 픽셀 소스에 대해 버퍼의 크기를 즉시 반환합니다. `core.Canvas`는 `DrawImage`로 동일한 방식으로 복사해 모든 플랫폼에서 픽셀을 그립니다(미니 프로그램은 WeChat 2D 캔버스가 필요합니다). `core.Shader`의 `Images`도 같은 방식으로 픽셀을 받으므로, 셰이더를 그리는 모든 플랫폼에서 에뮬레이터 화면처럼 앱이 쓴 프레임을 셰이더로 필터링할 수 있습니다.

Apple 플랫폼, Android, Windows, Linux, 웹은 픽셀 이미지를 직접 업데이트하며, WeChat 미니 프로그램은 2D 캔버스를 사용합니다. HarmonyOS는 각 버전마다 `PixelMap`을 생성하며 기기에서 검증되었고, 복사가 문자열 브리지를 거치므로 매 프레임 다시 쓰는 큰 버퍼는 그곳에서 60 fps보다 훨씬 느리게 동작합니다([픽셀 버퍼](https://github.com/nsgo-dev/nsgo/blob/main/docs/architecture/images.md#pixel-buffers)).

## 뷰 캡처하기

마운트된 뷰의 `core.Ref`로 `cx.Capture(&ref, done)`을 호출하거나, 현재 서피스 전체를 대상으로 `cx.Capture(nil, done)`을 호출하세요. 콜백은 엔진 고루틴에서 서피스의 픽셀 스케일에 맞는 PNG 바이트 또는 오류를 전달받습니다. 결과를 표시하려면 `core.ImageSource{Data: png}`를 사용하세요. 이를 공유하려면 앱의 캐시 디렉터리에 있는 `.png` 파일에 바이트를 쓰고 `core.ShareOptions.Files`에 해당 경로를 전달하세요. [파일 공유](services.md#opening-links-and-files)

캡처는 뷰의 경계 안에 보이는 서피스 픽셀을 가져옵니다. macOS, iOS, tvOS, Windows, Linux, Android, 웹에서 사용할 수 있습니다. Android는 액티비티의 창을 캡처합니다. 웹 캡처는 NSGo 셰이더 출력을 포함한 앱의 DOM을 캔버스에 그리며, 임베드된 프레임, 읽을 수 없는 이미지 및 일부 CSS 효과는 생략합니다.

## 리소스와 스케일

`resources/` 아래의 파일은 앱과 함께 패키징됩니다. `core.Src("asset:images/cover.png")`로 파일을 로드하고, `Scales: core.ScalesOf(1, 2, 3)`은 화면의 픽셀 비율에 따라 `cover.png`, `cover@2x.png` 또는 `cover@3x.png`를 선택합니다.

```go
core.Image{Source: core.ImageSource{
	URI:    "asset:images/cover.png",
	Scales: core.ScalesOf(1, 2, 3),
}}
```

`Placeholder`는 이미지를 로드하는 동안 보여 줄 소스를 지정하고, `FadeDuration`은 로드된 이미지가 그 위로 페이드인되는 시간을 정합니다. 0이면 플랫폼이 선택하며, 1나노초를 지정하면 즉시 표시합니다. `OnLoadStart`, `OnProgress`, `OnLoad`, `OnError`, `OnLoadEnd`는 로드 과정을 추적합니다. `cx.ImageSize`는 이미지를 표시하지 않고 소스의 픽셀 크기를 읽으며, `cx.PrefetchImage`는 이미지를 캐시에 미리 다운로드합니다. `cx.SetImageCacheLimit`은 인메모리 인코딩 이미지 바이트 예산(기본값 `core.DefaultImageCacheLimit`, 32 MiB; 음수는 0으로 고정됨)을 설정하고, `cx.KeepImage`는 반환된 `release`가 실행되거나 해당 스코프가 해제될 때까지 아틀라스나 텍스처를 캐시 축출(eviction)에서 고정 보호합니다. 이 제한은 고정된 항목, 진행 중인 로드, 현재 그리기 패스를 제외한 각 캐시에 적용되며, 디코딩된 픽셀, 프로세스 메모리 또는 브라우저 캐싱은 제한하지 않습니다. 0은 고정되지 않은 항목의 유지를 비활성화합니다. 네이티브 백엔드는 원격 이미지를 자체 디스크 캐시에 보관합니다.

## 애니메이션 및 벡터 이미지

GIF와 WebP 애니메이션은 뷰 안에서 각 프레임의 고유 지연 시간에 맞춰 재생되며 파일에 지정된 대로 반복합니다. SVG 문서는 이미지로 로드되며 어떤 크기에서도 선명하게 유지됩니다. 단색 SVG는 `TintColor`로 색상을 입힐 수 있습니다.

## 아이콘

`core.Icon`은 상속받은 텍스트 색상으로 SVG를 그리며, 스타일에서 크기를 지정하지 않으면 글꼴 크기와 같은 높이로 표시합니다. 상자가 SVG의 종횡비를 유지하므로 정사각형 상자에서도 늘어나지 않습니다.

```go
core.Icon{SVG: lucide.Search, Style: core.Style{FontSize: 20}, Label: "Search"}
```

`icons/lucide`는 각 Lucide 아이콘마다 PascalCase로 명명된 접근자(`lucide.ArrowUpRight`)를 제공합니다. `Label`로 이름을 지정하지 않으면 보조 기술에서 아이콘은 장식 요소로 취급됩니다. `ui.Icon`은 `ui`의 기본값이 적용된 동일한 컴포넌트이며, `core.SVG`와 `core.SVGView`는 앱이 직접 구성한 경로를 그립니다.

## 스타일이 적용된 이미지

`ui.Image`는 둥근 영역 안에 표시되는 이미지로, 로드 중에는 플레이스홀더를, 실패 시에는 흐린 그림 아이콘을 보여 줍니다. 스타일이 양쪽 크기를 모두 지정하지 않는 한 소스의 종횡비를 유지하며, 소스에 종횡비가 없으면 3:2 비율을 사용합니다.

```go
ui.Image{
	Source:      core.Src("asset:images/cover.png"),
	Label:       "Cover",
	ResizeMode:  core.Cover,
	AspectRatio: 16.0 / 9.0,
	Placeholder: ui.Skeleton{Style: core.Style{Height: core.Pt(180)}},
}
```

## 다음 단계

- [텍스트](text.md): 스팬 형태로 한 줄 안에 들어가는 이미지.
- [스타일과 테마](styles.md): 모서리, 그림자 및 색조.
- [목록과 스크롤](lists.md): 재활용되는 이미지 행.
