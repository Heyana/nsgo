---
title: スイッチ
description: オンまたはオフを切り替える設定コントロール。3 種類のサイズに対応しています。
group: コンポーネント
order: 35
---

`ui.Switch` は即座に反映される設定コントロールです。内部に値は保持せず、アプリが値を所有して `OnChange` で更新します。

```go preview=switch/basic
notifications := core.NewSignal(cx, true)
ui.Switch{Checked: notifications.Get, OnChange: notifications.Set, Label: "Notifications"}
```

## サイズ

`ui.Switch` は `Checked`、`OnChange`、`Label`、`Size`（`SizeDefault`、`SizeSmall`、または `SizeLarge`）を受け取ります。プラットフォームのフレームクロックに合わせてノブがスライドし、トラックの色が変化します。

```go preview=switch/small
ui.Switch{Checked: sync.Get, OnChange: sync.Set, Label: "Sync automatically", Size: ui.SizeSmall}
```

スイッチは即座に適用される設定のためのコントロールです。フォーム内の選択肢には[チェックボックス](checkbox.md)を、複数の選択肢から 1 つを選ぶ場合は `ui.RadioGroup` 内の `ui.Radio` または[セレクト](select.md)を使用してください。

## アクセシビリティ

スイッチは `RoleSwitch` とオン／オフの状態を報告します。`Label` が支援技術によって読み上げられる名前であり、通常はスイッチの横のテキストがラベルになります。キーボードフォーカスがあるときは Space キーまたは押下で状態が切り替わります。

## プラットフォーム

デフォルトのコントロールはアプリによって描画されるため、すべての環境で同じ見た目になります。`Variant: ui.SwitchSystem` を指定するとプラットフォーム標準のコントロール（macOS では NSSwitch、iOS では UISwitch、Android では Material 3 の 52 × 32 dp スイッチ）が使用されます。その他のターゲットではデフォルトの見た目が使われます。押下、フォーカス、スクリーンリーダーはプラットフォームの機能を使用します。スマートフォンのデフォルトのスイッチがデスクトップと同じサイズなのは仕様によるものであり、`SizeSmall` はデバイス別ではなく密度の高い行向けのサイズです。

## 関連

- [チェックボックス](checkbox.md)：1 つ以上の選択肢。
- [フォーム](../forms.md)：フォーム内の設定項目。
- [アクセシビリティ](../accessibility.md)：ラベルとフォーカス。
- ギャラリーの Switch ページ。
- [スタイルなしコンポーネント](../authoring/base.md#switch)：`base.Switch`、独自の見た目を持つスイッチ用。
