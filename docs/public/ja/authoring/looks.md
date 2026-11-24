---
title: 見た目とテーマ
description: 有効なテーマから色を取得し、コンポーネントにバリアントとサイズを持たせる。
group: コンポーネントを書く
order: 72
---

スタイル付きコンポーネントはスタイルの選択だけを行い、それ以外のことはしません。`chipsui` のような専用パッケージ内でスタイルなしコンポーネントをラップし、振る舞いの実装はスタイルなし側に残します。色はテーマから取得するため、アプリ全体のテーマ、パネル独自のテーマ、およびライト／ダーク外観に追従し、コンポーネントを再構築する必要がありません。

## テーマからの色の取得

色は `StyleFunc` の中で読み取ります。`StyleFunc` は読み取った値が変化すると再実行されるため、テーマや外観が切り替わってもコンポーネントをその場で再スタイルできます。`cx.Palette()` はそのコンポーネントが配置された場所のパレットを返し、`ui.ColorsOf(cx)` は `ui` が使用するロール名を提供します。

```go
// Toggle is chips.Toggle in the theme's colors.
type Toggle struct {
	On       func() bool
	OnChange func(bool)
	Label    string
}

func (t Toggle) Render(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	on := func() bool { return t.On != nil && t.On() }
	return chips.Toggle{On: t.On, OnChange: t.OnChange, Label: t.Label,
		Class: "flex-row items-center gap-2 rounded-full px-3 py-1",
		StyleFunc: func() core.Style {
			if on() {
				return core.Style{Background: colors.Accent()}
			}
			return core.Style{Background: colors.Inset()}
		},
		DotClass: "size-2 rounded-full",
		DotStyleFunc: func() core.Style {
			if on() {
				return core.Style{Background: colors.OnAccent()}
			}
			return core.Style{Background: colors.Faint()}
		},
		Children: []core.Node{ui.Text(t.Label, 13, ui.Medium, colors.Text)},
	}
}
```

ハードコードされた色はダークモードやアプリのカスタムテーマで破綻します。すべての色をロールから取得してください。

## 独自のカラーロール

パレットに適切なロールがない色は、独自の名前で `Palette.Colors` に追加し、ライトとダークの両方の外観で設定します。アプリ側ではコード内、またはテーマファイルの `light` と `dark` の下で上書きできます。

```go
theme := core.DefaultTheme()
theme.Light.SetColor("chip", core.RGB(0xeef5ff))
theme.Dark.SetColor("chip", core.RGB(0x1b2a3d))
cx.SetTheme(theme)
```

コンポーネントは `cx.Palette().Color("chip")` でその色を読み取ります。このメソッドはテーマにその色が定義されているかどうかも返すため、定義されていない場合は既存のロールにフォールバックできます。グラデーション、シャドウ、その他の仕上げ表現は色ではないため、コード内に置き、`cx.Dark()` に応じて選択してください。

## バリアント

複数の見た目を持つコンポーネントには、独自の型である `Variant` を持たせ、そのゼロ値をデフォルトの見た目にします。ブール値フィールドや共通の色型を使うのではなく、色も含めてそのコンポーネントが描画する見た目だけを表す名称にします。

```go
// ToggleVariant is a toggle's look.
type ToggleVariant uint8

const (
	// ToggleSolid fills the pill with the accent when on.
	ToggleSolid ToggleVariant = iota
	// ToggleOutline draws its edge in the accent instead.
	ToggleOutline
)
```

## サイズ

ほとんどのコンポーネントは 1 つのサイズだけを持てばよく、それによりアプリ側がサイズを指定しなくてもコントロール同士が揃います。複数のサイズが必要なコンポーネントでは、`ui.Button` の 28、24、32 ポイントに揃うように、デフォルト・小・大を表す `ui.Size` 型の `Size` を受け取ります。他のコントロールと高さを揃えるには `Size.Height()` を使います。アバターのようにどのスケールでも同じ比率・形状を保つコンポーネントでは、代わりにポイント単位の `Size` を受け取ります。
