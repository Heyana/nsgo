---
title: 캔버스와 셰이더
description: 엔진에서 2D 그래픽을 그리고 GPU 프래그먼트 셰이더로 뷰를 채웁니다.
group: 핵심 개념
order: 59
---

## 캔버스

`core.Canvas`는 웹 페이지의 `<canvas>`처럼 엔진에 2D 드로잉을 기록하고 플랫폼이 이를 재생하도록 합니다. `Draw`는 내부에서 읽은 시그널, 캔버스의 크기 또는 사용 가능한 글꼴이 바뀔 때마다 다시 실행되며, 플랫폼은 기록된 내용을 메인 스레드에서 그립니다. `cx.RegisterFont`로 로드된 글꼴이 사용 가능해지면 `Draw`가 다시 실행되어 `TextMetrics`를 기반으로 한 텍스트와 도형을 함께 갱신합니다.

상속받은 글꼴의 em 단위로 캔버스 크기를 정하려면 `Em` 또는 `EmFunc`를 사용하세요. 너비에 따라 높이가 달라지는 콘텐츠의 경우 `MeasureEm(width, fontSize)`가 포인트 단위의 가용 너비(제한이 없을 때는 `core.Auto`)와 결정된 글꼴 크기를 바탕으로 `EmBox`를 반환합니다. 내부의 읽기는 추적되며 레이아웃 도중에도 실행되므로 시그널 쓰기를 피해야 합니다. `Ref.ContentBox()`는 패딩과 테두리 안쪽에 있는 마운트된 뷰의 레이아웃 박스를 자체 좌표계로 추적합니다.

```go
core.Canvas{
	Style: core.Style{Height: core.Pt(220)},
	Draw: func(d *core.Drawing, size core.Size) {
		d.SetFillColor(core.RGB(0xf1f5f9)).FillRect(0, 0, float32(size.Width), float32(size.Height))
		d.SetStrokeColor(core.RGB(0x007aff)).SetLineWidth(2).SetLineCap(core.CapRound)
		d.BeginPath().MoveTo(24, 160).LineTo(90, 80).LineTo(160, 120).LineTo(230, 40).Stroke()
	},
}
```

드로잉 기록은 웹의 `CanvasRenderingContext2D`를 따릅니다. `Save`와 `Restore`, 변환(transform), 직선·곡선·사각형·호로 이루어진 패스, 색상과 그라데이션을 사용한 채우기 및 선 그리기, 클리핑, 점선, 그림자, 텍스트, 이미지를 지원합니다. `core.ParsePath`는 SVG 패스 데이터에서 `core.Path`를 만들고, `FillFromView`와 `StrokeFromView`는 캔버스 뷰가 상속받은 `Fill`과 `Stroke` 스타일을 가져오므로 차트가 앱의 테마를 따르게 할 수 있습니다.

웹에서는 확대된 `core.Scroll` 내부나 크기가 조절된 `Style.Transform` / `AnimatedStyle` 안에 있는 캔버스가 움직임이 멈춘 뒤 화면 픽셀 비율의 최대 4배까지 누적 배율로 다시 그려집니다. 확대된 비트맵이 4096×4096픽셀을 초과하는 캔버스는 보이는 영역만 그리고 스크롤하거나 패닝할 때 갱신하므로 `Draw`를 다시 실행하지 않고도 선과 글리프를 선명하게 유지합니다. Apple, Android, Windows, Linux 역시 스크롤 확대/축소와 스타일 변환 모두에서 확대된 캔버스와 텍스트를 선명하게 유지합니다.

```go
d.SetFillGradient(core.CanvasLinear(0, 0, float32(size.Width), 0,
	core.GradientStop{Offset: 0, Color: core.RGB(0x4da3ff)},
	core.GradientStop{Offset: 1, Color: core.RGB(0x007aff)},
)).FillRect(0, 0, float32(size.Width), 40)
```

## 텍스트, 이미지 및 부분 요소

`MeasureText`, `TextMetrics`, `SetTextAlign`은 플랫폼의 텍스트 엔진으로 텍스트를 배치하므로 레이블이 앱의 나머지 부분과 같은 글꼴을 사용합니다. `DrawImage`, `DrawImageIn`, `DrawImageFrom`은 이미지를 그리고, `DrawSVG`와 `DrawSVGIn`은 파싱된 `*core.SVG` 문서(`core.ParseSVG`)를 본래 크기나 사각형에 맞춘 크기로 그립니다(`<text>`와 `<tspan>` 요소는 플랫폼 자체 텍스트 엔진으로 측정·렌더링되며 `currentColor`는 드로잉의 채우기 색상을 따릅니다). 또한 `OnPress`에서는 `IsPointInPath`와 `IsPointInStroke`로 마지막 드로잉을 히트 테스트할 수 있습니다. 네이티브 백엔드는 최근 그려진 비고정 캔버스 이미지 소스 64개를 캐싱하며, 스프라이트 아틀라스나 오래 유지되는 텍스처가 캐시에서 밀려나지 않도록 고정하려면 `release := cx.KeepImage(src)`를 호출해 `release()`가 실행되거나 `cx`가 정리될 때까지 유지하세요.

```go
badge := core.MustParseSVG([]byte(`<svg viewBox="0 0 120 40">
	<rect width="120" height="40" rx="8" fill="#007aff"/>
	<text x="60" y="25" text-anchor="middle" font-size="14" fill="#fff">Live</text>
</svg>`))

d.DrawSVGIn(badge, 12, 12, 180, 60)
```

크기가 조절된 비트맵은 기본적으로 부드러운 샘플링(보간)을 사용합니다. 픽셀 아트를 그리려면 `DrawImage`, `DrawImageFrom` 또는 `DrawAtlas` 전에 `d.SetImageSmoothing(false)`를 호출하거나 TSX에서 `ctx.imageSmoothingEnabled = false`를 설정하세요. `Save`와 `Restore`는 나머지 드로잉 상태와 함께 이 설정도 보존합니다. 최근접 이웃(nearest-neighbor) 샘플링은 Apple 플랫폼, Windows, Linux, Android, 웹에서 사용할 수 있으며 HarmonyOS와 미니 프로그램에서도 적용되지만 아직 기기에서 출력이 검증되지는 않았습니다.

`d.DrawVertices(image, vertices)`는 텍스처가 입혀진 메시를 그립니다. 연속된 3개의 `core.CanvasVertex` 값이 하나의 삼각형을 이룹니다. `DrawImageFrom`과 마찬가지로 `Position`은 캔버스 포인트로, `TexCoord`는 이미지 포인트로 설정하세요. 각 정점의 `Color`는 텍스처와 곱해져 삼각형 전체에 보간되며, 제로 값은 텍스처를 변경하지 않습니다. 정점을 업데이트하고 드로잉을 다시 기록하면 메시를 변형할 수 있습니다. 이미지는 픽셀 버퍼나 `OffscreenImage`의 `Source()`를 포함해 어떤 `ImageSource`든 될 수 있으나, Android의 캔버스와 미니 프로그램은 아직 오프스크린 이미지를 그리지 못합니다. `SetImageSmoothing(false)`는 메시에도 적용됩니다. 웹에서 임의의 메시를 그리려면 인라인 이미지나 동일 출처 이미지가 필요합니다. 미니 프로그램은 웹 플레이어로 메시를 그리며 WeChat DevTools나 기기에서는 아직 검증되지 않았습니다. HarmonyOS는 각 삼각형을 클리핑하고 아핀 변환으로 이미지를 그리며, `DrawImage`와 마찬가지로 인코딩된 이미지와 픽셀 버퍼는 그리지만 오프스크린 이미지는 그리지 못하고 각 삼각형의 평균 정점 알파는 반영하나 색상 틴트는 적용하지 않습니다. 또한 불투명 삼각형 사이의 이음매만 가려 주며 아직 기기에서 검증되지 않았습니다.

`DrawTintedImage`, `DrawTintedImageIn`, `DrawTintedImageFrom`은 합성하기 전에 이미지의 sRGB 및 알파 채널에 색상을 곱합니다. 기본 source-over 모드에서는 투명한 픽셀 뒤로 배경이 그대로 보입니다. TSX에서는 `drawImage`와 동일한 좌표 형식으로 `drawTintedImage(image, tint, ...args)`를 사용하세요. 웹에서 교차 출처(cross-origin) 소스의 픽셀에 틴트를 입히려면 익명 CORS 요청이 허용되어야 합니다. HarmonyOS는 바이트, 앱 리소스, 픽셀 버퍼에서 온 이미지에 틴트를 적용하지만 아직 기기에서 검증되지 않았으며 오프스크린 이미지는 아직 그려지지 않습니다.

한 이미지에서 여러 스프라이트를 순서대로 그리려면 `d.DrawAtlas(source, rects, xforms, alphas)`를 사용하세요. 각 `core.Bounds`는 이미지 자체 포인트 좌표계에서 타일을 선택합니다. 대응하는 `core.RSXform`은 타일의 왼쪽 상단 모서리를 `TX, TY`로 매핑하며, `SCos`와 `SSin`에는 크기 배율에 회전 각도의 코사인과 사인을 곱한 값을 설정합니다. 예를 들어 `core.RSXform{SCos: 0.5, TX: 40, TY: 20}`은 32포인트 타일을 절반 크기로 (40, 20) 위치에 그립니다. 완전한 불투명도로 그리려면 `nil`을 전달하고, 그렇지 않으면 현재 드로잉 알파에 곱할 계수들을 전달하세요.

`Drawing.Node`는 차트의 막대나 지도의 영역처럼 드로잉의 일부에 범위(bounds), 역할, 레이블, 선택 상태를 지정합니다. 보조 기술은 이러한 부분을 캔버스의 자식 요소로 읽고 테스트는 레이블, 텍스트 또는 테스트 ID로 이를 찾으며, 해당 부분에 대한 누르기는 그 중심점을 누르는 것으로 처리됩니다.

```go
d.Node(core.CanvasNode{
	Bounds: core.Bounds{Origin: core.Point{X: x, Y: y}, Size: core.Size{Width: w, Height: h}},
	Role:   core.RoleButton,
	Label:  "Open the report",
	TestID: "report",
})
```

## 오프스크린 이미지

`cx.NewOffscreenImage(size, scale, draw)`는 `core.Drawing`을 보존된 오프스크린 이미지로 한 번 렌더링해 두어 이후 프레임에서 `DrawImage`, `DrawImageIn`, `DrawImageFrom`, `DrawAtlas` 또는 `core.Image`로 그릴 수 있게 합니다. `scale <= 0`을 전달하면 현재 디스플레이의 픽셀 비율을 사용합니다. 드로잉은 `Invalidate()`나 `Resize(size, scale)`가 호출되거나 새 글꼴이 등록될 때만 다시 렌더링되며, `cx`가 정리되거나 `Release()`가 호출되면 자동으로 해제됩니다.

```go
tiles := cx.NewOffscreenImage(core.Size{Width: 512, Height: 512}, 0, func(d *core.Drawing) {
	// Drawn once into the offscreen image until tiles.Invalidate() is called.
	for y := range 16 {
		for x := range 16 {
			d.SetFillColor(tileColor(x, y)).FillRect(float32(x*32), float32(y*32), 32, 32)
		}
	}
})

core.Canvas{
	Style: core.Style{Width: core.Pt(320), Height: core.Pt(240)},
	Draw: func(d *core.Drawing, _ core.Size) {
		// Reuses the cached surface in a single DrawImage operation per frame.
		d.DrawImage(tiles.Source(), -cameraX.Get(), -cameraY.Get())
	},
}
```

## 애니메이션 인자

`Drawing.Bind`는 마지막 연산의 인자를 애니메이션 노드에 바인딩하며, 플랫폼은 커밋 없이 자체 프레임 클록에 맞춰 해당 부분만 다시 그립니다. `core.ArgX`, `ArgY`, `ArgR`, `ArgA`, `ArgColor`가 각 인자를 가리키며, macOS, iOS, Linux는 변경된 영역만 다시 그리는 반면 Windows는 다시 기록하고 Android와 웹은 캔버스 전체를 다시 그립니다.

```go
needle := core.NewAnimatedValue(cx, 0)
d.Save().Translate(cx, cy).Rotate(0).Bind(core.ArgA, needle).
	BeginPath().MoveTo(-14, -3).LineTo(80, 0).LineTo(-14, 3).ClosePath().Fill().Restore()
```

## 셰이더

`core.Shader`는 GPU가 모든 픽셀에 대해 실행하는 프래그먼트 셰이더로 자신의 영역을 채웁니다. `core.CompileShader`는 `Fragment` 함수가 있는 Go 문법의 프로그램을, `core.CompileWGSL`은 WGSL을 받습니다. `Uniforms`는 프로그램의 내보낸 변수들을 설정하고, `Images`는 최대 4개의 텍스처를 전달합니다.

```go
var program = core.MustCompileShader(`
package main

var Tint vec4

func Fragment(dst vec4, pos vec2) vec4 {
	v := 0.5 + 0.5*sin(pos.x/40)
	return Tint * vec4(v, v, v, 1)
}
`)

core.Shader{
	Program:  program,
	Uniforms: core.Uniforms{"Tint": core.RGB(0x007aff)},
	Style:    core.Style{Height: core.Pt(220), CornerRadius: 16},
}
```

Go 셰이더는 픽셀의 중심 좌표를 `dst`로, 포인트 단위의 같은 지점을 `pos`로 전달받으며 GLSL의 내장 함수들을 사용할 수 있고, `rectSize`, `pixelScale`, `image0At`~`image3At` 및 그 크기 함수들이 NSGo 확장으로 제공됩니다. WGSL 프로그램은 `fn shade(uv: vec2f) -> vec4f`를 작성하고 `nsh.time`과 `nsh.size`를 읽으며 `Paused`로 클록을 멈출 수 있습니다. 셰이더용 GPU 경로가 없는 플랫폼에서는 아무것도 그리지 않습니다.

`core.ShaderEffect`는 자식 요소를 WGSL을 거쳐 그리고 `core.ScreenEffect`는 서피스 전체를 덮습니다. `core.ShaderTransition`과 `core.ScreenTransition`은 `Value`가 바뀌기 전과 후의 화면을 샘플링합니다. `core.TransitionShader`는 내비게이션과 다이얼로그에서 이를 수행합니다. Android에서 효과를 위해 뷰를 캡처하려면 Android 10 이상과 외부 텍스처를 지원하는 OpenGL ES 3.0이 필요합니다. `core.Canvas`와 `core.Shader` 모두 `Label`을 받으며 자식 요소는 그 위에 그려집니다.

## 다음 단계

- [이미지와 아이콘](images.md): 캔버스가 그리는 그림.
- [애니메이션](animation.md): 캔버스가 바인딩하는 값.
- [접근성](accessibility.md): 캔버스가 노출하는 부분.
