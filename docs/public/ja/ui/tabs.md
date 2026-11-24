---
title: タブ
description: タブリスト、セグメンテッドコントロール、およびシステムタブバー。
group: コンポーネント
order: 40
---

`ui.Tabs[T]` はセグメンテッドコントロールまたはタブリストとして少数の値から 1 つを選択し、アプリはその横や近くに選択された値に対応するコンテンツを表示します。タブリストは 1 つの Tab 停止点となり、矢印キーでタブ間を移動できます。

```go preview=tabs/basic
section := core.NewSignal(cx, "overview")
ui.Tabs[string]{
	Options: []ui.Option[string]{
		{Value: "overview", Label: "Overview"},
		{Value: "activity", Label: "Activity"},
		{Value: "settings", Label: "Settings"},
	},
	Value:    section.Get,
	OnChange: section.Set,
	Label:    "Sections",
}
```

## バリアントとサイズ

`Variant` には `TabsSegmented`（デフォルト。くぼんだトラック内の立体的なサム）または `TabsUnderline`（選択されたタブの下のアクセント線）を指定できます。アンダーライン形式では `IndicatorColor` を設定したり、`IndicatorFit: TabsIndicatorContent` を使ってタブ全体ではなくラベルとアイコンの幅に合わせたりすることができます（TSX では `indicatorColor` と `indicatorFit="content"` です）。`Size` は `SizeDefault`、`SizeSmall`、または `SizeLarge` を指定でき、`Equal` を有効にするとラベル幅に合わせる代わりにすべてのタブを同じ幅にします。

```go preview=tabs/underline
ui.Tabs[string]{Variant: ui.TabsUnderline, Equal: true, Size: ui.SizeSmall, Options: tabs, Value: section.Get, OnChange: section.Set}
```

各オプションには `Icon`、`Disabled` 関数、`Content` ノード、`TestID` を設定できます。`Content` はラベルを置き換え、タブのテキストスタイルを継承します。

タブがシグナルに追従する場合は `OptionsFunc` を使用します。NSGo は値によって行をその場で更新するため、選択中のタブの値が選択肢に残っていれば選択状態とキーボードフォーカスが維持されます。その値が消えた場合は、アプリが変更するまで未選択のままになります。

## タブバー

`ui.TabShell` はあらゆるウィンドウ幅でトップレベルのセクションをタブバーのそばに配置します。デフォルトでは下端に配置され、`Edge`（または `EdgeFunc`）が `mobile.TabBarLeading`、`mobile.TabBarTrailing`、または `mobile.TabBarAutomatic`（`cx.Env().VerticalBarEdge()` に追従）の場合は先頭側または末尾側の端に沿って縦方向に配置されます。すべてのタブが `SystemIcon` または SVG の `Icon` を提供している場合、iOS ではどの端でも、Android では下端でシステム標準のバーが使用されます。iOS の SF Symbol または Android の drawable を指定する `SystemIcon` が優先され、そうでない場合は SVG が表示されます。それ以外の環境では、指定された SVG アイコンを使ってバーを描画します。独自の領域を占有するバーは自身でその端のセーフエリアを避けるため、その隣に配置されるセクション（`base.Nav` のページを含む）の該当する端の `core.EnvSafeArea` は 0 になります。iOS のフローティングな下部システムバーの下では、`ui.TabBarInset` を使ってコンテンツが重ならないようにしてください。

ロケールやアプリの状態に追従するタブタイトルには `ShellTab.TextFunc` を、バーのアクセシブルな名前には `TabShell.LabelFunc` を設定します。アイコンが同じである限り、タイトルを変更しても選択中のセクションと既存のタブは保持されます。

`mobile.TabBar` は iOS および Android 上のシステムタブバーであり、`Items`、`Selected`、`OnChange` と、オプションで `Edge`（`mobile.TabBarBottom`、`mobile.TabBarLeading`、`mobile.TabBarTrailing`、または `mobile.TabBarAutomatic`）／`EdgeFunc` を受け取ります。各 `mobile.TabBarItem` は `Title` と `Icon` によるプラットフォームアイコン名を受け取ります。`Icon` が iOS の SF Symbol や Android の drawable を指していない場所では `SVG: lucide.House()` が表示され、24 ポイントに収まって選択状態に応じたティント色が適用されます。TSX では、`svg` が SVG ドキュメントを受け取り、`edge` が `"bottom"`、`"leading"`、`"trailing"`、または `"automatic"` を受け取ります。それ以外のプラットフォームでは `mobile.TabBar` は領域を占有しないため、代わりに `ui.Tabs` を表示してください。

`ui.SidebarShell` はサイドバー、フッター、ヘッダー、コンテンツ、右パネルが保持する内容を自身で左右のセーフエリアから避けつつ、それぞれの背景はウィンドウの端まで広げるため、それらの内部では左右の `core.EnvSafeArea` が 0 になり、`ui.NavHeader` は横方向のセーフエリアを自身のパディングに加えます。`ui.NavPage` や `TabShell` セクションのコンテンツは `PaddingEnv` によって自身でセーフエリアを避けるため、全面表示の画像のように端まで広げるべき要素は端まで描画できます。

## アクセシビリティ

リストは `RoleTabList`、各タブは `RoleTab`、選択中のタブはその状態を報告します。リストは 1 つの Tab 停止点となり、左／上、右／下、Home、End キーで有効なタブ間を移動し、到達したタブが押下を報告します。`Label` がリストの名前になります。

## プラットフォーム

iOS と Android ではそれぞれの見た目とジェスチャーを備えたシステムのタブバーが表示され、macOS、Windows、Linux、Web では `ui.Tabs` が表示されます。アプリのコンテンツはすべてのターゲットで共通です。

## 関連

- [ナビゲーション](../navigation.md)：アプリ内のページとタブバー。
- [メニュー](menu.md)：セクションではなくコマンド向け。
- [セレクト](select.md)：リスト内の多数の選択肢向け。
- ギャラリーの Tabs および NavStack ページ。
- [スタイルなしコンポーネント](../authoring/base.md#tabs)：`base.Tabs`、独自の見た目を持つタブ用。
