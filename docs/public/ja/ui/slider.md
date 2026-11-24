---
title: スライダー
description: キーボードやポインターを使って、トラック上の 1 つの値または範囲を選択します。
group: コンポーネント
order: 37
---

`ui.Slider` はドラッグ、ステップ、キーボード操作によってトラック上の値を選択します。1 つの値を扱う場合は単一のノブになり、`High` を指定すると 2 つのノブによる範囲指定になります。

```go preview=slider/basic
volume := core.NewSignal(cx, 40.0)
ui.Slider{Value: volume.Get, OnChange: volume.Set, Min: 0, Max: 100, Step: 5, Label: "Volume"}
```

## 範囲指定

2 つの値を選択するには `High` と `OnHighChange` を設定します。`Value` が下限側になります。`LowLabel` と `HighLabel` は支援技術向けにそれぞれのノブに名前を付けます。

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

## ステップと値

`Step` は値をスナップし、ステップが 0 の場合は任意の値を取ります。`ValueText` は支援技術が値として読み上げる内容を変更し、`Disabled` は操作を無効化します。

```go preview=slider/format
ui.Slider{Value: speed.Get, OnChange: speed.Set, Min: 0.5, Max: 2, Step: 0.25, Label: "Speed",
	ValueText: func(v float64) string { return fmt.Sprintf("%g×", v) }}
```

動画が再生時間を報告する場合など、範囲やステップが動的に変化するときは `MinFunc`、`MaxFunc`、`StepFunc` を使用します。これらは対応する静的フィールドを上書きし、フォーカスと進行中のドラッグを保持したままマウント済みのスライダーを更新します。`MaxFunc` が明示的に 0 を返すと、再生時間が取得されるまで折りたたまれた範囲（幅 0 の範囲）を許容します。アプリの値は保持され、現在の境界内に収めて表示されます。

```go
ui.Slider{Value: position.Get, OnChange: position.Set, MaxFunc: duration.Get, Label: "Position"}
```

TSX では、`min={minimum()}`、`max={duration()}`、`step={step()}` がそのままシグナルに追従します。`max={0}` は `MaxFunc` が 0 を返す場合と同様に範囲を折りたたみます。

## 方向

`Axis: base.Vertical` を指定するとスライダーが縦向きになり、下端が最小値になります。この場合は `Style` で高さを指定してください。`Inverted` を指定すると最小値が反対側の端（縦向きスライダーでは上端、横向きスライダーでは終端）に配置されます。トラックに沿った矢印キーの操作もこの方向に追従します。`ui.Progress` も同じ `Axis` と `Inverted` を受け取ります。

```go
ui.Slider{Value: level.Get, OnChange: level.Set, Axis: base.Vertical, Label: "Level", Style: core.Style{Height: core.Pt(160)}}
```

## 動作

ドラッグ操作はポインターに追従し、移動中に `OnChange` を報告します。矢印キーはフォーカス中のノブを 1 ステップずつ移動し、Home キーと End キーは両端へ、Page Up キーと Page Down キーはより大きく移動します。`Step` が 0 より大きい場合、`ui.Slider` はトラックの下に最大 60 個までステップの目盛りを表示します。

## アクセシビリティ

スライダーは `RoleAdjustable`、`Label`、およびその値を報告します。範囲指定の場合は両方のノブがそれぞれのラベルを持ちます。数値の代わりに `ValueText` が読み上げられます。キーボードおよびスクリーンリーダーのインクリメント／デクリメントアクションで値を変更できます。

## プラットフォーム

トラックはアプリ側で描画されるため、どの環境でも同じサイズになります。押下、フォーカス、スクリーンリーダーはプラットフォームの機能を使用します。タッチでのドラッグは同じステップでポインタードラッグとして動作します。

## 関連

- [フォーム](../forms.md)：フォーム内のスライダー。
- [アクセシビリティ](../accessibility.md)：値と調整可能（adjustable）ロール。
- ギャラリーの Slider、RangeSlider、Rating ページ。
- [スタイルなしコンポーネント](../authoring/base.md#slider)：`base.Slider`、縦向き配置や独自の目盛り描画。
