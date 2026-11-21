---
title: 动画
description: 过渡、动画值、布局动画和动效预设，全部运行在平台的帧时钟上。
group: 核心概念
order: 58
---

## 动画值

动画运行在平台主线程上，由平台自己的帧时钟推进，因此每帧都不消耗引擎。`AnimatedValue` 是平台移动的一个数字；`Style.Animated` 把视图的透明度、位移、缩放、旋转、背景、边框颜色、阴影和文本颜色绑定到它。

```go
fade := core.NewAnimatedValue(cx, 0)
cx.StartAnimation(core.Timing(fade, 1, core.TimingConfig{
	Duration: 300 * time.Millisecond,
	Easing:   core.EaseOut,
}), nil)

core.View{Style: core.Style{Animated: &core.AnimatedStyle{Opacity: fade}}}
```

`Timing`、`Spring` 和 `Decay` 移动一个值，`Sequence`、`Parallel`、`Stagger`、`Delay` 和 `Loop` 组合它们。`cx.StartAnimation` 返回 `AnimationRun`，`Stop` 让动画停在当前位置，`Reset` 把值放回开始处。动画从值的当前位置开始，同一个值的新动画会结束旧的。

```go
scale := core.NewAnimatedValue(cx, 1)
ui.Spring(cx, scale, 1.1, core.SpringConfig{Stiffness: 220, Damping: 14})
```

派生节点跟随一个值：`core.Interpolate` 按自己的缓动和外推映射数字或颜色，`core.Add`、`Sub`、`Mul`、`Div`、`Mod`、`DiffClamp` 和 `SnappingDiffClamp` 由它计算其他值。`ui.Lerp` 是简短的插值。

```go
angle := core.Interpolate(scroll, core.Interpolation{
	Input:  []float64{0, 240},
	Output: []float64{0, 90},
	Easing: core.EaseOut,
})
```

`Scope.RequestAnimationFrame` 在下一个显示帧运行一次函数，`Scope.Animate` 在该时钟上推进一段时间。两者都运行在响应式图的 goroutine 上，因此可以设置 signal。函数收到的是该帧的时间：在 macOS、iOS、Android、Web、Linux、Windows、HarmonyOS 和 watchOS 上是显示器自己的帧时间（HarmonyOS 需要构建机装有 OpenHarmony SDK，没有 SDK 打包的应用得到的是引擎时钟的时间），所以按两帧时间差移动内容的循环会跟上显示器的节奏；`nsgotest` 测试中则是测试时钟的时间。

`Scope.Tween` 在该时钟上推进一个数字，用于动画值够不到的地方，比如文本里向上滚动的数字：它像 `Timing` 移动值那样，把从一个数到另一个数的每个值传给一个函数，最后再传一次终值。启动它的 `nsgotest` 步骤会像等待动画一样等到终值；开启减弱动态效果时，它立即给出终值。TSX 的 `Animated.tween` 与之相同。

```go
freed := core.NewSignal(cx, 0.0)
cx.Tween(0, 18.7, core.TimingConfig{Duration: 600 * time.Millisecond, Easing: core.BezierEasing(core.EaseOut)}, freed.Set, nil)

core.Text{ContentFunc: func() string { return fmt.Sprintf("%.1f GB 可释放", freed.Get()) }}
```

## 过渡

`Transition` 在视图的样式或类名变化时，由平台播放过渡，每帧不需要引擎参与。`Properties` 指定哪些属性移动；为 0 时表示除尺寸位置外的所有属性。

```go
core.View{Style: core.Style{
	Background:  core.RGB(0xffffff),
	CornerRadius: 12,
	Transition:  core.Transition{Duration: 200 * time.Millisecond, Properties: core.TransitionBackground | core.TransitionTransform},
	Transform:   core.Transform{Scale: 1},
	Hover:       &core.Style{Transform: core.Transform{Scale: 1.02}, Background: core.RGB(0xf5f5f5)},
}}
```

`TransitionLayout` 在视图自己的样式给出新框架时，从它当前显示的位置移动和改变大小。布局动画则动画整个提交，像 React Native 的 `LayoutAnimation`：`cx.AnimateLayout(core.LayoutEaseInEaseOutPreset)` 动画下一批的移动、插入和删除，预设覆盖缓动、线性和弹簧。`LayoutSpring` 过渡接受 `Response`（弹簧的自然周期）和 `DampingRatio`（`< 1` 欠阻尼、`1` 临界阻尼、`> 1` 过阻尼），按物理运动直到静止而非按固定时长截断；在中途收到新目标时，会保留上一帧的位置和速度平滑衔接。

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

`View.OnTransitionEnd` 在一次变化完成它的过渡后运行，无论是否有东西移动。减少动态效果会关闭过渡和布局动画，因此要让最终状态仍然可达。

## 动效

`core.Motion` 用关键帧同时移动一个视图的透明度、位移、缩放和旋转，关键帧来自应用自己的 `MotionSpec` 或现成的 `MotionPreset`：进入、退出、吸引注意和循环。绑定它的样式会把视图放在静止状态；`Play` 播放一个 spec，`Exit` 让视图在 `core.Presence` 中带着动效离开。

```go
enter := core.SlideInUp.Spec()
motion := core.NewMotion(cx, &enter)
motion.Exit(core.FadeOut.Spec())

core.View{Style: core.Style{Animated: motion.Style()}, Children: content}
```

`core.Presence{When, Then, Else}` 会保留一个分支，直到它注册的所有退出动画结束，因此 sheet 或 toast 无论被什么移除，都会带着动效离开。`core.PresenceOf` 将相同机制用于可比较的键和内容函数，`MaxLeaving` 会在值快速变化时限制离开的旧分支数量。`MotionPresets()` 列出所有预设，`Kind()` 说明它是进入、退出、吸引注意还是循环。

## 关键帧

`KeyframeAnimation` 在平台上重复关键帧：每个偏移处的透明度、缩放、旋转和纵向位移，带时长、延迟、缓动和次数。`KeyframesSpin`、`KeyframesPing`、`KeyframesPulse` 和 `KeyframesBounce` 是 Tailwind 的。

```go
core.View{Style: core.Style{Animation: core.KeyframeAnimation{
	Keyframes:  core.KeyframesPulse,
	Duration:   2 * time.Second,
	Iterations: 0, // 永远
}}}
```

`ValueKeyframes` 像 Web Animations 一样让动画值经过多个停靠点，支持偏移、分段缓动、延迟、次数和方向。手势也可以驱动值：`View.PanValues`、`PinchValue` 和 `RotationValue` 在手势开始时从原值出发，在主线程上移动它们。

`core.PanEvent` 在每个 pan 阶段提供视图内的 `Position` 和页面内的 `PagePosition`，因此拖拽工具无需依赖单独的按压事件来获取手指位置。

## 下一步

- [样式与主题](styles.md)：过渡移动的属性。
- [列表与滚动](lists.md)：跟随滚动视图的值。
- [布局](layout.md)：动画的框架是什么。
