---
title: 애니메이션
description: 플랫폼의 프레임 클록에서 실행되는 트랜지션, 애니메이션 값, 레이아웃 애니메이션 및 모션 프리셋.
group: 핵심 개념
order: 58
---

## 애니메이션 값

애니메이션은 플랫폼의 자체 프레임 클록에 맞춰 메인 스레드에서 실행되므로, 프레임마다 엔진에 부하를 주지 않습니다. `AnimatedValue`는 플랫폼이 움직이는 숫자이며, `Style.Animated`는 뷰의 투명도, 이동(translation), 크기(scale), 회전, 배경, 테두리 색상, 그림자, 텍스트 색상을 이 값에 바인딩합니다.

```go
fade := core.NewAnimatedValue(cx, 0)
cx.StartAnimation(core.Timing(fade, 1, core.TimingConfig{
	Duration: 300 * time.Millisecond,
	Easing:   core.EaseOut,
}), nil)

core.View{Style: core.Style{Animated: &core.AnimatedStyle{Opacity: fade}}}
```

`Timing`, `Spring`, `Decay`는 하나의 값을 움직이며, `Sequence`, `Parallel`, `Stagger`, `Delay`, `Loop`는 이를 조합합니다. `cx.StartAnimation`은 `AnimationRun`을 반환하며, `Stop`은 실행을 현재 위치에서 멈추고 `Reset`은 값을 되돌립니다. 실행은 값이 현재 놓여 있는 지점에서 시작하며, 같은 값에 대한 새 애니메이션이 시작되면 기존 애니메이션은 종료됩니다.

```go
scale := core.NewAnimatedValue(cx, 1)
ui.Spring(cx, scale, 1.1, core.SpringConfig{Stiffness: 220, Damping: 14})
```

파생 노드는 하나의 값을 따라갑니다. `core.Interpolate`는 자체 이징과 외삽(extrapolation)으로 숫자나 색상을 매핑하고, `core.Add`, `Sub`, `Mul`, `Div`, `Mod`, `DiffClamp`, `SnappingDiffClamp`는 그로부터 다른 값을 계산합니다. `ui.Lerp`는 간단한 보간 함수입니다.

```go
angle := core.Interpolate(scroll, core.Interpolation{
	Input:  []float64{0, 240},
	Output: []float64{0, 90},
	Easing: core.EaseOut,
})
```

`Scope.RequestAnimationFrame`은 다음 디스플레이 프레임에 함수를 한 번 실행하고, `Scope.Animate`는 해당 클록으로 지정된 기간 동안 단계를 진행합니다. 둘 다 그래프의 고루틴에서 실행되므로 시그널을 설정할 수 있습니다. 함수에는 프레임의 시각이 전달되며, 이는 macOS, iOS, Android, 웹, Linux, Windows, HarmonyOS, watchOS에서 실제 디스플레이의 시각입니다(HarmonyOS에서는 빌드 머신에 OpenHarmony SDK가 있을 때 디스플레이 클록을 사용하며, 없이 패키징된 앱은 엔진의 클록을 사용합니다). 따라서 두 프레임 사이의 시간 차이로 요소를 움직이는 루프는 디스플레이의 주사율을 따르며, `nsgotest` 테스트에서는 테스트 클록의 시각이 전달됩니다.

창의 콜백 주기를 설정하려면 `cx.SetPreferredFrameRate(core.FrameRateRange{Preferred: 30})`를 사용하세요. ProMotion 화면에서 `{Minimum: 80, Maximum: 120, Preferred: 120}`처럼 `Minimum`과 `Maximum`을 함께 전달하여 디스플레이 링크에 범위를 지정할 수도 있습니다. 이 함수는 창 자신의 스코프에서 호출하세요. 설정은 변경될 때까지 `RequestAnimationFrame`과 `Animate`에 적용되며, 제로 값을 전달하면 디스플레이의 기본 주기로 복원됩니다. 콜백에 전달되는 시각은 여전히 실제 디스플레이 시각입니다. 사용 가능한 주사율은 디스플레이와 OS가 결정하며, NSGo는 콜백 주기를 맞추기 위해 디스플레이 프레임을 건너뛸 수 있습니다.

`Scope.Tween`은 텍스트 안에서 올라가는 숫자처럼 애니메이션 값이 닿을 수 없는 대상에 대해 해당 클록으로 숫자를 단계적으로 변경합니다. `Timing`이 값을 움직이는 방식대로 시작 값부터 끝 값까지 각 숫자로 함수를 호출하고, 마지막으로 최종 값으로 한 번 더 호출합니다. 이를 시작하는 `nsgotest` 스텝은 일반 애니메이션과 마찬가지로 마지막 호출까지 대기하며, 동작 줄이기(reduced motion) 설정이 켜져 있으면 즉시 마지막 값을 전달합니다. TSX의 `Animated.tween`도 동일하게 동작합니다.

```go
freed := core.NewSignal(cx, 0.0)
cx.Tween(0, 18.7, core.TimingConfig{Duration: 600 * time.Millisecond, Easing: core.BezierEasing(core.EaseOut)}, freed.Set, nil)

core.Text{ContentFunc: func() string { return fmt.Sprintf("%.1f GB can be freed", freed.Get()) }}
```

## 트랜지션

`Transition`은 뷰의 스타일이나 클래스가 바뀔 때 프레임마다 엔진 작업 없이 플랫폼에서 애니메이션을 적용합니다. `Properties`는 움직일 속성을 지정하며, 0은 프레임을 제외한 모든 속성을 뜻합니다.

```go
core.View{Style: core.Style{
	Background:  core.RGB(0xffffff),
	CornerRadius: 12,
	Transition:  core.Transition{Duration: 200 * time.Millisecond, Properties: core.TransitionBackground | core.TransitionTransform},
	Transform:   core.Transform{Scale: 1},
	Hover:       &core.Style{Transform: core.Transform{Scale: 1.02}, Background: core.RGB(0xf5f5f5)},
}}
```

`TransitionLayout`은 뷰 자신의 스타일로 새 프레임이 주어졌을 때 현재 표시된 위치에서 새 위치와 크기로 부드럽게 이동하고 크기를 바꿉니다. 반면 레이아웃 애니메이션은 React Native의 `LayoutAnimation`처럼 커밋 전체를 애니메이션합니다. `cx.AnimateLayout(core.LayoutEaseInEaseOutPreset)`은 다음 배치의 이동, 삽입, 제거를 애니메이션하며, 프리셋으로 이징, 선형, 스프링을 제공합니다. `LayoutSpring` 트랜지션은 `Response`(스프링의 비감쇠 고유 주기)와 `DampingRatio`(`< 1` 저감쇠, `1` 임계 감쇠, `> 1` 과감쇠)를 받으며, 고정된 기간이 아니라 안정될 때까지 실행되고 도중에 새 목표가 주어지면 현재 위치와 속도를 그대로 이어받습니다.

```go
cx.AnimateLayout(core.LayoutAnimation{
	Update: &core.LayoutTransition{
		Curve:        core.LayoutSpring,
		Response:     280 * time.Millisecond,
		DampingRatio: 0.85,
	},
})
open.Set(!open.Get())
```

`View.OnTransitionEnd`는 실제로 움직인 것이 있든 없든 변경에 대한 트랜지션이 끝나면 한 번 실행됩니다. 동작 줄이기 설정은 트랜지션과 레이아웃 애니메이션을 끄므로, 최종 상태에 항상 도달할 수 있게 구성하여 플랫폼 설정을 존중하세요.

## 모션

`core.Motion`은 앱이 직접 만든 `MotionSpec`이나 미리 준비된 `MotionPreset`(등장, 퇴장, 시선 끌기, 반복)을 사용해 한 뷰의 투명도, 이동, 크기, 회전을 키프레임으로 함께 움직입니다. 스타일을 바인딩하면 뷰가 정지 상태로 설정되고, `Play`는 스펙을 재생하며, `Exit`은 뷰가 `core.Presence` 안에서 자신의 모션을 재생하며 사라지도록 합니다.

```go
enter := core.SlideInUp.Spec()
motion := core.NewMotion(cx, &enter)
motion.Exit(core.FadeOut.Spec())

core.View{Style: core.Style{Animated: motion.Style()}, Children: content}
```

`core.Presence{When, Then, Else}`는 등록된 모든 퇴장 애니메이션이 끝날 때까지 해당 브랜치를 유지하므로, 무엇이 시트나 토스트를 제거하든 모션을 마친 뒤 사라지게 됩니다. `core.PresenceOf`는 비교 가능한 키와 콘텐츠 함수에 같은 동작을 적용하며, `MaxLeaving`은 값이 빠르게 바뀔 때 남아 있는 이전 브랜치의 수를 제한합니다. `MotionPresets()`는 모든 프리셋을 나열하고 `Kind()`는 등장, 퇴장, 시선 끌기, 반복 중 어떤 종류인지 반환합니다.

## 키프레임

`KeyframeAnimation`은 플랫폼에서 키프레임을 반복 재생하며, 지속 시간, 지연, 이징, 반복 횟수와 함께 각 오프셋의 투명도, 크기, 회전, 수직 이동을 지정합니다. `KeyframesSpin`, `KeyframesPing`, `KeyframesPulse`, `KeyframesBounce`는 Tailwind의 프리셋입니다.

```go
core.View{Style: core.Style{Animation: core.KeyframeAnimation{
	Keyframes: core.KeyframesPulse,
	Duration:  2 * time.Second,
	Iterations: 0, // forever
}}}
```

`ValueKeyframes`는 Web Animations처럼 오프셋, 세그먼트별 이징, 지연, 반복 횟수, 방향을 지정해 애니메이션 값을 여러 지점으로 움직입니다. 제스처 역시 값을 구동할 수 있습니다. `View.PanValues`, `PinchValue`, `RotationValue`는 제스처가 시작될 때의 위치에서부터 메인 스레드에서 값을 움직입니다.

`core.PanEvent`는 모든 팬(pan) 단계에서 뷰 내부의 `Position`과 페이지 내의 `PagePosition`을 함께 제공하므로, 드래그 도구가 손가락 위치를 알기 위해 별도의 press 이벤트를 받을 필요가 없습니다.

## 다음 단계

- [스타일과 테마](styles.md): 트랜지션이 움직이는 속성.
- [리스트와 스크롤](lists.md): 스크롤 뷰를 따르는 값.
- [레이아웃](layout.md): 애니메이션 프레임이 무엇인지.
