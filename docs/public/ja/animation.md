---
title: アニメーション
description: プラットフォームのフレームクロック上で動作するトランジション、アニメーション値、レイアウトアニメーション、モーションプリセット。
group: コアコンセプト
order: 58
---

## アニメーション値

アニメーションはプラットフォームのメインスレッド上でプラットフォーム自身のフレームクロックによって駆動されるため、フレームごとのエンジン負荷はゼロです。`AnimatedValue` はプラットフォームが動かす数値であり、`Style.Animated` はビューの不透明度、平行移動、拡大縮小、回転、背景、ボーダー色、影、テキスト色をその値にバインドします。

```go
fade := core.NewAnimatedValue(cx, 0)
cx.StartAnimation(core.Timing(fade, 1, core.TimingConfig{
	Duration: 300 * time.Millisecond,
	Easing:   core.EaseOut,
}), nil)

core.View{Style: core.Style{Animated: &core.AnimatedStyle{Opacity: fade}}}
```

`Timing`、`Spring`、`Decay` は 1 つの値を動かし、`Sequence`、`Parallel`、`Stagger`、`Delay`、`Loop` はそれらを組み合わせます。`cx.StartAnimation` は `AnimationRun` を返し、`Stop` で現在の位置で停止、`Reset` で値を元に戻します。アニメーションは現在値がどこにあってもそこから開始され、同じ値に対して新しいアニメーションを開始すると古いアニメーションは終了します。

```go
scale := core.NewAnimatedValue(cx, 1)
ui.Spring(cx, scale, 1.1, core.SpringConfig{Stiffness: 220, Damping: 14})
```

派生ノードは元の値に追従します。`core.Interpolate` は独自のイージングと外挿（extrapolation）を使って数値や色をマッピングし、`core.Add`、`Sub`、`Mul`、`Div`、`Mod`、`DiffClamp`、`SnappingDiffClamp` はその値から別の値を計算します。`ui.Lerp` は簡易的な補間ヘルパーです。

```go
angle := core.Interpolate(scroll, core.Interpolation{
	Input:  []float64{0, 240},
	Output: []float64{0, 90},
	Easing: core.EaseOut,
})
```

`Scope.RequestAnimationFrame` は次のディスプレイフレームで関数を 1 回実行し、`Scope.Animate` はそのクロック上で指定期間ステップを実行します。どちらもグラフのゴルーチン上で実行されるため、シグナルを設定できます。関数はフレーム時刻を受け取ります。この時刻は macOS、iOS、Android、Web、Linux、Windows、HarmonyOS、watchOS ではディスプレイ自身の時刻になります（HarmonyOS ではビルドマシンに OpenHarmony SDK がある場合。SDK なしでパッケージ化されたアプリはエンジンのクロックを受け取ります）。そのため、2 フレーム間の時間差で要素を動かすループはディスプレイのペースに追従します。`nsgotest` テストではテストクロックの時刻が渡されます。

ウィンドウのコールバックレートは `cx.SetPreferredFrameRate(core.FrameRateRange{Preferred: 30})` で設定します。`Minimum` と `Maximum` を渡すと、ProMotion ディスプレイ向けの `{Minimum: 80, Maximum: 120, Preferred: 120}` のようにディスプレイリンクに範囲を指定できます。そのウィンドウ自身のスコープから呼び出してください。この設定は変更されるまで `RequestAnimationFrame` と `Animate` に適用されます。ゼロ値の範囲を指定するとディスプレイの通常ペースに戻ります。コールバック時刻は引き続き実際のディスプレイ時刻のままです。利用可能なリフレッシュレートはディスプレイと OS が決定し、NSGo はコールバックのペースを調整するためにディスプレイフレームをスキップすることがあります。


`Scope.Tween` は、テキスト内のカウントアップ数値のようにアニメーション値が直接到達できない対象のために、そのクロック上で数値をステップ実行します。`Timing` が値を動かすのと同じように開始値から終了値までの各数値で関数を呼び出し、最後にもう一度最終値で呼び出します。`nsgotest` のステップで開始した場合は、アニメーションと同様に最終値まで待機し、モーション軽減設定が有効な場合は即座に最終値を渡します。TSX の `Animated.tween` も同様です。

```go
freed := core.NewSignal(cx, 0.0)
cx.Tween(0, 18.7, core.TimingConfig{Duration: 600 * time.Millisecond, Easing: core.BezierEasing(core.EaseOut)}, freed.Set, nil)

core.Text{ContentFunc: func() string { return fmt.Sprintf("%.1f GB can be freed", freed.Get()) }}
```

## トランジション

`Transition` は、ビューのスタイルやクラスが変化した際にプラットフォーム側でアニメーションを実行し、フレームごとのエンジン処理を不要にします。`Properties` でアニメーション対象を指定します。`0` を指定するとフレーム以外のすべてのプロパティが対象になります。

```go
core.View{Style: core.Style{
	Background:  core.RGB(0xffffff),
	CornerRadius: 12,
	Transition:  core.Transition{Duration: 200 * time.Millisecond, Properties: core.TransitionBackground | core.TransitionTransform},
	Transform:   core.Transform{Scale: 1},
	Hover:       &core.Style{Transform: core.Transform{Scale: 1.02}, Background: core.RGB(0xf5f5f5)},
}}
```

`TransitionLayout` は、ビュー自身のスタイルによって新しいフレームが与えられたとき、現在の表示位置から移動とリサイズを行います。一方、レイアウトアニメーションは React Native の `LayoutAnimation` のようにコミット全体をアニメーションさせます。`cx.AnimateLayout(core.LayoutEaseInEaseOutPreset)` は次のバッチにおける移動・挿入・削除をアニメーションし、プリセットにはイージング、リニア、スプリングが用意されています。`LayoutSpring` トランジションは `Response`（スプリングの非減衰固有周期）と `DampingRatio`（`< 1` で減衰振動、`1` で臨界減衰、`> 1` で過減衰）を受け取り、固定時間ではなく収束するまで動作し、途中のターゲット変更時にはその時点の位置と速度を新しいターゲットへ引き継ぎます。

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

`View.OnTransitionEnd` は、実際に値が動いたかどうかにかかわらず、変更に対するトランジションが完了した後に実行されます。モーション軽減設定が有効な場合、トランジションとレイアウトアニメーションは無効になるため、最終状態へ到達できるようにしてプラットフォーム設定を尊重してください。

## モーション

`core.Motion` は、1 つのビューの不透明度、平行移動、拡大縮小、回転をキーフレームでまとめて動かします。アプリ独自の `MotionSpec` または既製の `MotionPreset`（登場、退場、注目、ループ）を使用します。スタイルをバインドするとビューは静止状態になり、`Play` でスペックを実行し、`Exit` を設定すると `core.Presence` 内で退場モーションを再生してからビューを取り除けます。

```go
enter := core.SlideInUp.Spec()
motion := core.NewMotion(cx, &enter)
motion.Exit(core.FadeOut.Spec())

core.View{Style: core.Style{Animated: motion.Style()}, Children: content}
```

`core.Presence{When, Then, Else}` は、登録されたすべての退場アニメーションが完了するまでブランチを保持するため、何がビューを削除してもシートやトーストがモーション付きで退場します。`core.PresenceOf` は比較可能なキーとコンテンツ関数に対して同じ振る舞いを適用し、値が素早く変化する間の古いブランチ数を `MaxLeaving` で制限できます。`MotionPresets()` はすべてのプリセットを一覧表示し、`Kind()` は登場・退場・注目・ループのいずれかを返します。

## キーフレーム

`KeyframeAnimation` はプラットフォーム上でキーフレームを繰り返し再生します。各オフセットでの不透明度、拡大縮小、回転、垂直移動と、期間、遅延、イージング、反復回数を指定します。`KeyframesSpin`、`KeyframesPing`、`KeyframesPulse`、`KeyframesBounce` は Tailwind と同じものです。

```go
core.View{Style: core.Style{Animation: core.KeyframeAnimation{
	Keyframes: core.KeyframesPulse,
	Duration:  2 * time.Second,
	Iterations: 0, // forever
}}}
```

`ValueKeyframes` は Web Animations と同様に、オフセット、セグメントごとのイージング、遅延、反復回数、方向を指定してアニメーション値を複数のストップ間で動かします。ジェスチャーでも値を駆動できます。`View.PanValues`、`PinchValue`、`RotationValue` は、ジェスチャー開始時の位置からメインスレッド上で値を動かします。

`core.PanEvent` には、すべてのパンフェーズにおいてビュー内の `Position` とページ内の `PagePosition` が含まれるため、ドラッグツールで指の位置を復元するために別途プレスイベントを拾う必要はありません。

## 次のステップ

- [スタイルとテーマ](styles.md): トランジションが動かすプロパティ。
- [リストとスクロール](lists.md): スクロールビューに追従する値。
- [レイアウト](layout.md): アニメーションされるフレームの仕組み。
