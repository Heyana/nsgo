---
title: 슬라이더
description: 키보드와 포인터로 트랙을 따라 하나의 값 또는 범위를 선택합니다.
group: 컴포넌트
order: 37
---

`ui.Slider`는 드래그, 단계(step), 키보드를 통해 트랙을 따라 값을 선택합니다. 하나의 값일 때는 단일 노브(knob)를 사용하며, `High`를 지정하면 두 개의 노브로 이루어진 범위가 됩니다.

```go preview=slider/basic
volume := core.NewSignal(cx, 40.0)
ui.Slider{Value: volume.Get, OnChange: volume.Set, Min: 0, Max: 100, Step: 5, Label: "Volume"}
```

## 범위

두 개의 값을 선택하려면 `High`와 `OnHighChange`를 설정하세요. `Value`는 하한값입니다. `LowLabel`과 `HighLabel`은 보조 기술을 위해 각 노브의 이름을 지정합니다.

```go preview=slider/range
price := core.NewSignal(cx, 20.0)
ceiling := core.NewSignal(cx, 80.0)
ui.Slider{
	Value: price.Get, OnChange: price.Set,
	High: ceiling.Get, OnHighChange: ceiling.Set,
	Min: 0, Max: 100, Step: 1,
	Label: "Price", LowLabel: "Minimum", HighLabel: "Maximum",
}
```

## 단계와 값

`Step`은 값을 해당 단위에 맞추어 스냅하며, 0이면 모든 값을 허용합니다. `ValueText`는 보조 기술이 값으로 읽어 주는 텍스트를 변경하고, `Disabled`는 조작을 막습니다.

```go preview=slider/format
ui.Slider{Value: speed.Get, OnChange: speed.Set, Min: 0.5, Max: 2, Step: 0.25, Label: "Speed",
	ValueText: func(v float64) string { return fmt.Sprintf("%g×", v) }}
```

동영상이 전체 재생 시간을 보고할 때처럼 범위나 단계가 동적으로 바뀔 때는 `MinFunc`, `MaxFunc`, `StepFunc`를 사용하세요. 이들은 정적 필드를 재정의하고 마운트된 슬라이더를 업데이트하면서 포커스와 진행 중인 드래그를 그대로 유지합니다. 0을 반환하는 명시적인 `MaxFunc`는 재생 시간 정보가 도착하기 전까지 축소된 범위를 허용하며, 앱의 값은 보존되고 현재 범위 내에서 표시됩니다.

```go
ui.Slider{Value: position.Get, OnChange: position.Set, MaxFunc: duration.Get, Label: "Position"}
```

TSX에서는 `min={minimum()}`, `max={duration()}`, `step={step()}`가 시그널을 그 자리에서 따르며, `max={0}`은 0을 반환하는 `MaxFunc`와 마찬가지로 범위를 축소합니다.

## 방향

`Axis: base.Vertical`은 최솟값이 아래쪽에 오도록 슬라이더를 세로로 세웁니다. 이때 `Style`에 높이를 지정하세요. `Inverted`는 최솟값을 반대쪽 끝(세로 슬라이더의 위쪽, 가로 슬라이더의 끝쪽)에 둡니다. 트랙을 따라 이동하는 방향키도 이 방향을 따릅니다. `ui.Progress` 역시 동일한 `Axis`와 `Inverted`를 받습니다.

```go
ui.Slider{Value: level.Get, OnChange: level.Set, Axis: base.Vertical, Label: "Level", Style: core.Style{Height: core.Pt(160)}}
```

## 동작

드래그는 포인터를 따라가며 움직이는 동안 `OnChange`를 보고합니다. 방향키는 포커스된 노브를 한 단계씩 이동시키고, Home과 End는 양 끝으로 이동하며, Page Up과 Page Down은 더 크게 이동합니다. `ui.Slider`는 `Step`이 0보다 클 때 트랙 아래에 최대 60개까지 단계 눈금을 표시합니다.

## 접근성

슬라이더는 `RoleAdjustable`, `Label`, 그리고 현재 값을 보고합니다. 범위 슬라이더의 경우 두 노브가 각각 자신의 레이블을 가집니다. 숫자 대신 `ValueText`가 읽힙니다. 키보드와 스크린 리더의 증가/감소 동작으로 값을 변경할 수 있습니다.

## 플랫폼

트랙은 앱이 직접 그리므로 모든 곳에서 크기가 동일합니다. 누르기, 포커스, 스크린 리더는 플랫폼에서 제공되며, 터치 드래그는 동일한 단계 단위로 포인터 드래그처럼 동작합니다.

## 관련 문서

- [폼](../forms.md): 폼 안의 슬라이더.
- [접근성](../accessibility.md): 값과 조절 가능(adjustable) 역할.
- 갤러리의 Slider, RangeSlider 및 Rating 페이지.
- [스타일이 없는 컴포넌트](../authoring/base.md#slider): 세로 방향과 나만의 눈금을 지원하는 `base.Slider`.
