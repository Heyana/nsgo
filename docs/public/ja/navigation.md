---
title: ナビゲーション
description: ネイティブのナビゲーションスタック、システムヘッダー、モーダル、メニュー、タブバー。
group: コアコンセプト
order: 56
---

## スタック

`base.NewNav` は 1 つの型のルートからなるスタックを管理します。`Push` でページを追加し、`Pop` で先頭のページを取り除き、`PopToRoot` で最初のページに戻り、`Replace` で先頭のページを入れ替えます。その `Stack` はすべてのルートをページとして描画し、先頭のページを表示しながら、その下にあるページも状態を保ったままマウントし続けます。`ui.NavPage` は各ページに `ui` の背景と、タイトル・戻るボタン・各種ボタンを備えた `ui.NavHeader` を与えます。

watchOS にはシステムのナビゲーションバー、タブバー、スイッチがないため、watchOS では `ui.NavPage` が戻るボタンを含む `ui.NavHeader` をページの上部に描画し、`ui.TabShell` が独自のタブバーを描画し、`ui.SwitchSystem` は `ui` 独自のスイッチになります。

```go
nav := base.NewNav(cx, "inbox")

return nav.Stack(core.Style{FlexGrow: 1}, func(cx *core.Scope, route string, depth func() int) base.NavPage {
	if route == "inbox" {
		return ui.NavPage(cx, base.NavPage{Title: "Inbox", Children: []core.Node{
			ui.Button{Text: "Open a message", OnPress: func() { nav.Push("message") }},
		}}, ui.NavHeader{})
	}
	return ui.NavPage(cx, base.NavPage{Title: "Message"}, ui.NavHeader{
		OnBack: func() { nav.Pop(1) }, BackLabel: "Inbox",
		Trailing: []core.Node{ui.IconButton{Icon: lucide.Archive, Label: "Archive", Variant: ui.IconButtonGhost, OnPress: archive}},
	})
})
```

iOS と Android では、ページのタイトルとヘッダーのボタンからプラットフォームがシステムのヘッダーを戻るボタンやトランジションとともに描画します。macOS、Windows、Linux、Web では `ui` が描画します。

Android では Material 3 のトップアプリバーを使用します。通常のバーは高さ 64 dp、タイトル 22 sp です。`HeaderLargeTitle` を指定すると高さ 152 dp、タイトル 28 sp のラージバーになり、ページ先頭のスクロールビューがコンテンツをその下へスクロールすると通常のバーへと折りたたまれ、コンテンツが先頭に戻ると展開されます。

iOS と Android でネイティブの通常（コンパクト）タイトルおよび展開タイトルのフォントをスタイリングするには、`HeaderTitleStyle` と `HeaderLargeTitleStyle`（`FontSize`、`FontWeight`、`FontFamily`、`FontStyle`）を設定します。未設定のフィールドは各プラットフォームのデフォルト値を維持し、`HeaderTitleStyleFunc` と `HeaderLargeTitleStyleFunc` は変更に追従します。背景色、タイトル色、ボタンのティントカラーは引き続き `HeaderStyle` が提供します。TSX ではこれらは `headerTitleStyle` および `headerLargeTitleStyle` のスタイルオブジェクトまたは配列になります。

Android でカスタムの `HeaderTitle` を指定するとコンパクト時のタイトルのみが置き換わり、展開時のラージタイトルには引き続き `Title` が使われます。`Android: core.AndroidScreenProps{HeaderRightWithLargeTitle: true}` を設定すると、そのラージタイトルの横に `HeaderRight` を配置できます。タイトルが折りたたまれると、そのコントロールは固定ツールバーへ移動し、折りたたみの前後を通じて押下可能な状態を保ちます。TSX では `android={{ headerRightWithLargeTitle: true }}` を使用します。フォントにさらにスペースが必要な場合、ラージ領域の高さは自動的に広がります。スタンドアロン Android ビルドではネイティブ RTL レイアウトが有効になりますが、組み込みホストではネイティブヘッダーがアプリの向きに従うように、アプリケーション側で `android:supportsRtl="true"` を設定する必要があります。

ページに独自の `Header` を指定すると `ui.NavHeader` が置き換わります。iOS と Android では、`HeaderTitle` の代わりにシステムヘッダーのタイトルビューとなり、ページの `HeaderLeft`、`HeaderRight`、戻るボタンの横に配置されます。検索フィールドのように内部のビューが伸長するかパーセンテージ幅を取る場合は両端の間のスペースを埋め、それ以外の場合はコンテンツのサイズに合わせられます。その他のプラットフォームではコンテンツの上部に描画されます。`ui.NavHeader.Scroll` は引き続き iOS または Android ページのスクロールとヘッダーのぼかし効果を連動させます。

`NoPopOnBack` を設定すると戻る操作が押されてもページを保持し、`OnBackPress` はその押下を受け取り、`NoPopOnSwipe` は iOS のスワイプで戻る操作を無効にします。`cx.OnBackPress(func() bool)` を使うと、未保存の変更がある場合などのためにスタックより先にアプリのハンドラーを実行できます。`OnDismiss` はユーザーがページをポップしたことを通知し、スタックはそのページを取り除きます。

## イマーシブ全画面表示

`mobile.Immersive(cx, true)` を呼び出すとウィンドウの全画面表示を要求し、`mobile.Immersive(cx, false)` で通常のシステムバー設定に戻ります。ページまたはダイアログでは、`StatusBar: core.StatusBarSetting{Immersive: core.ToggleOn}` を指定するとそのレイヤーが表示されている間だけ全画面表示を要求します。TSX ではページやダイアログに `statusBar={{ immersive: true }}` を、ウィンドウのデフォルト設定には `nsgo.yaml` で `status_bar: { immersive: true }` を使用します。

Android では、画面端からのスワイプでステータスバーとナビゲーションバーが一時的に表示されます。HarmonyOS では、ウィンドウがシステムに対してステータスバー、3 ボタンナビゲーションバー、およびナビゲーションインジケーターを非表示にするよう要求します。iOS ではステータスバーが非表示になり、ホームインジケーターをいつ隠すか表示するかは UIKit が決定します。`cx.Env().SafeArea()` はディスプレイの切り欠き（カットアウト）を含め、システムが提供するインセットを引き続き報告します。横画面で 3 ボタンナビゲーションを使用している HarmonyOS では、システムがボタンを画面側面に描画している間もボタン領域を下辺に沿って報告するため、ボタンが側面を覆っている一方でセーフエリアは下部に余白を残すことがあります。

## モーダル

`ui.Dialog` は暗くしたアプリの上にサーフェス（タッチスクリーンでは下から現れるシート）を表示し、`ui.Sheet` は下からパネルを表示します。どちらも無効化しない限り、Escape キーまたは背景の押下で閉じます。

```go
ui.Dialog{
	Open:         dialog.Get,
	OnOpenChange: func(open bool, _ base.Reason) { dialog.Set(open) },
	Title:        "Rename",
	Description:  "The new name shows everywhere.",
	Children:     []core.Node{ui.Input{Value: name.Get, OnChange: name.Set}},
	Footer: []core.Node{
		ui.Button{Text: "Cancel", Variant: ui.ButtonSecondary, OnPress: func() { dialog.Set(false) }},
		ui.Button{Text: "Save", OnPress: save},
	},
}
```

`ui.AlertDialog` はユーザーに確認を求め、`cx.ShowAlert` はシステム標準のアラートまたはアクションシートを表示します（[ダイアログ](ui/dialog.md)）。

## メニュー

`ui.Menu` はボタンから `ui.MenuItem` を開きます。どのデバイスでも、ボタンの横に表示されるか、`Presentation` の指定に応じて下からシートとして表示されます。`ui.ContextMenu` は副ボタンクリックや長押しで同様の動作を行い、`ui.Popover`、`ui.HoverCard`、`ui.Tooltip` はその他のアンカー付きコンテンツを表示します。

```go
ui.Menu{Trigger: ui.Button{Text: "Actions", Variant: ui.ButtonSecondary}, Children: []core.Node{
	ui.MenuItem{Text: "Rename", OnSelect: rename},
	ui.MenuItem{Text: "Duplicate", OnSelect: duplicate},
	ui.MenuItem{Text: "Delete", Variant: ui.MenuItemDanger, OnSelect: remove},
}}
```

## タブ

`ui.Tabs[T]` はセグメンテッドコントロールまたはタブリストであり、選択されたオプションへスライドするインジケーターを備えています。`TabStyle` は各タブのスタイルに、`Selected` は選択されたタブのスタイルに、`IndicatorStyle` はインジケーターのスタイルにそれぞれマージされます。アンダーラインタブでは `IndicatorColor` を設定でき、`IndicatorFit: TabsIndicatorContent` で下線をコンテンツの幅に合わせられます。`ui.Sidebar` はデスクトップ向けサイドバーのセクションと行を保持し、`ui.TabShell` はあらゆるウィンドウ幅においてトップレベルセクションをタブバー（デフォルトでは下辺、または `Edge` / `EdgeFunc` で先頭・末尾の辺に縦配置。`cx.Env().VerticalBarEdge()` に追従する `mobile.TabBarAutomatic` も指定可能）の横に配置します。すべてのタブが `SystemIcon` または SVG の `Icon` を持つ場合、iOS ではどの辺でも、Android では下辺でシステムのバーを使用し、iOS で `SystemIcon` が SF Symbol を指していない箇所や Android で drawable を指していない箇所では SVG を表示します。それ以外の場合は SVG アイコンを使って独自のバーを描画します。プッシュされた詳細ページの表示中にどちらのバーも非表示にしてセクションのセーフエリアを復元するには `BarHidden` を設定します。非表示または縦配置の間、`TabBarInset` は 0 になります。

`mobile.TabBar` は iOS と Android におけるシステム標準のタブバーで、Liquid Glass または Material の外観を持ちます。`mobile.TabBarItem{Title, Icon, SVG}` の `Items`、選択中のインデックス、`OnChange`、および省略可能な `Edge`（`mobile.TabBarBottom`、`mobile.TabBarLeading`、`mobile.TabBarTrailing`、または iOS 27.1 では `cx.Env().VerticalBarEdge()` に追従し他の環境では下辺にフォールバックする `mobile.TabBarAutomatic`）またはリアクティブな `EdgeFunc` を受け取ります。Android のパレットロール `tabBarBackground`、`tabBarIdle`、`tabBarSelected`、`tabBarIndicator` は、それぞれバーの背景、非選択時のアイコンとラベル、選択時のアイコンとラベル、選択時のピルを設定します。カスタムパレットではそれぞれ `Surface`、`TextMuted`、`Accent`、`accentContainer` にフォールバックし、`accentContainer` が未設定の場合は 12% 不透明度のアクセントカラーのピルが使われます。デフォルトパレットでは Material の背景色と非選択時の色を維持し、選択されたタブのみアクセントカラーで着色します。その他のプラットフォームでは領域を占有しないため、アプリでは代わりに `ui.Tabs` を表示します。バーが表示されるかどうかは `mobile.IsAvailable(cx)` で判定できます。`Icon` はプラットフォームアイコンの名前を指定します。`lucide.House()` などの `SVG` は、iOS と Android で `Icon` が SF Symbol や drawable を指していない場合に表示され、システムアイコンと同様にバーによって着色されます。

`TabBarItem.Badge` にテキストを表示する場合は `mobile.TabBadge{Text: "3"}`、ドットを表示する場合は `mobile.TabBadge{Dot: true}` を設定します。更新は `ItemsFunc` を通じて行い、ゼロ値を返すとバッジを消去できます。`ui.ShellTab.Badge` はシステムバーと独自描画バーのどちらに対しても `func() mobile.TabBadge` を受け取ります。両方のフィールドが設定されている場合はテキストが優先されます。UIKit ではドットに対して空白のネイティブバッジが表示されます。支援技術はバッジのテキスト、または新着コンテンツを示すローカライズされた語句を読み上げます。TSX ではネイティブアイテムに `badge: { text: "3" }` または `badge: { dot: true }` を、`TabShell` アイテムにはその値を返す関数を渡します。

```go
mobile.TabBar{
	Items: []mobile.TabBarItem{{Title: "Home", Icon: "house"}, {Title: "Search", Icon: "magnifyingglass"}},
	Selected: func() int { return tab.Get() },
	OnChange: func(i int) { tab.Set(i) },
}
```

## 次のステップ

- [フォーム](forms.md): スタックのページが保持する内容。
- [アニメーション](animation.md): ページやモーダルがどのように動くか。
- [レイアウト](layout.md): ヘッダー周囲のセーフエリア。
