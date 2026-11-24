---
title: メニュー
description: トリガーから開くメニュー、コンテキストメニュー、およびそのメニュー項目。
group: コンポーネント
order: 42
---

`ui.Menu` はボタンから `ui.MenuItem` を開きます。デバイスを問わず、ボタンの横に開くことも、`Presentation: base.PresentSheet` を指定して下からのシートとして開くこともできます。`ui.ContextMenu` は同じ項目をポインターの位置に開きます。

```go preview=menu/basic height=280
ui.Menu{Trigger: ui.Button{Text: "Actions", Variant: ui.ButtonSecondary}, Children: []core.Node{
	ui.MenuItem{Text: "Rename", Icon: lucide.Pencil, OnSelect: rename},
	ui.MenuItem{Text: "Duplicate", Shortcut: "mod+d", OnSelect: duplicate},
	ui.MenuSeparator{},
	ui.MenuItem{Text: "Delete", Variant: ui.MenuItemDanger, OnSelect: remove},
}}
```

メニューは自身で開閉を行いますが、`Open` と `OnOpenChange` を使うとアプリ側で開閉状態を制御できます。`Trigger` はボタンの見た目と内容であり、メニューがその押下イベントを受け取ります。ポップアップはトリガーの横の空きスペースからはみ出すことはありません。全体が収まる側に開き、そのスペースや `MaxHeight` を超える場合は項目がスクロールします。

## メニュー項目

`ui.MenuItem` は `Text`、任意の `Icon`、および `Shortcut`（`"mod+c"` のように指定し、ポップアップ内ではそのプラットフォームの表記で表示されます）を表示し、`OnSelect` を通知して、特に `NoCloseOnSelect` が指定されていない限りメニューを閉じます。`Disabled` は項目を無効化し、`Checked` は横にチェックマークを表示し、`Variant: ui.MenuItemDanger` は破壊的な操作であることを示します。`ui.MenuLabel` はグループの見出しとなり、`ui.MenuSeparator` はグループ間を区切ります。`ui.Submenu` はさらに項目を保持し、その項目の横に開くか、シート内では周囲の項目と入れ替わって開きます。

```go preview=menu/checked height=320
ui.Menu{Trigger: ui.Button{Text: "View", Variant: ui.ButtonGhost}, Children: []core.Node{
	ui.MenuLabel{Text: "Sort by"},
	ui.MenuItem{Text: "Name", Checked: func() bool { return sort.Get() == "name" }, OnSelect: func() { sort.Set("name") }},
	ui.MenuItem{Text: "Date", Checked: func() bool { return sort.Get() == "date" }, OnSelect: func() { sort.Set("date") }},
	ui.MenuSeparator{},
	ui.Submenu{Text: "Share", Children: []core.Node{
		ui.MenuItem{Text: "Copy link", OnSelect: copyLink},
		ui.MenuItem{Text: "Email", OnSelect: emailLink},
	}},
}}
```

## コンテキストメニュー

`ui.ContextMenu` は、`Trigger` 上でのセカンダリクリック、またはタッチスクリーンでの長押しによって、ポインター位置に項目を開きます。

```go preview=menu/context height=220
ui.ContextMenu{Trigger: fileRow, Children: []core.Node{
	ui.MenuItem{Text: "Open", OnSelect: open},
	ui.MenuItem{Text: "Delete", Variant: ui.MenuItemDanger, OnSelect: remove},
}}
```

## 振る舞い

Escape キー、戻る操作、またはメニュー外側への押下でメニューが閉じます。メニューは `Placement` と `Align` で配置されるポップアップとして表示されるか、そのように指定された場合は下からのシートとして表示されます。デスクトップでは、Down、Up、Home、End、Page Down、Page Up キーによって（ポインターが乗っている項目を起点に）有効な項目間を移動し、Right でサブメニューを開き、Left で閉じ、Return で項目を選択します。キーボードで開いたメニューでは最初の項目にフォーカスが当たり、メニューを閉じるとフォーカスはトリガーに戻ります。

## アクセシビリティ

メニューは `RoleMenu`、各項目は `RoleMenuItem` を報告します。コンテンツに名前がない場合は、`Label` がトリガーに名前を付けます。スクリーンリーダーは項目のテキストと、無効化されているかどうかを読み上げます。

## プラットフォーム

トリガーと項目はアプリ側が提供し、シート、ポップアップ、フォーカス、Escape キー、戻る操作はプラットフォーム側が担います。ポップアップとシートのどちらを使うかはアプリ側が選択し、NSGo がデバイスに応じて自動で選び分けることはありません。

## 関連項目

- [セレクト](select.md): リストから 1 つの値を選択します。
- [ダイアログ](dialog.md): モーダルサーフェス。
- [タブ](tabs.md): 複数のセクションを一度に表示します。
- ギャラリーの Menu、ContextMenu、Command ページ。
- [スタイルなしコンポーネント](../authoring/base.md#menu): 独自の見た目を持つメニュー向けの `base.Menu`。
