---
title: ターミナル
description: ターミナルグリッド上でプログラムを実行したり、リモートセッションを表示したりします。
group: コアコンセプト
order: 63
---

`base/terminal` はセッション（`Terminal`）とその `View` を提供します。デスクトップではローカルプログラムを実行し、iOS やブラウザではリモートプログラムに接続された `io.ReadWriteCloser` である `Options.Conn` を使用します。アプリがセッションの所有権を持ち、終了時にクローズします。

```go
t, err := terminal.New(terminal.Options{CursorBlink: true})
if err != nil {
	return core.Text{Content: err.Error()}
}
cx.Cleanup(func() { t.Close() })
return terminal.View{
	Terminal: t, AutoFocus: true,
	MinimumContrast: 4.5, PaddingColor: terminal.PaddingExtend,
	Style: core.Style{FlexGrow: 1},
}
```

`Options.Cursor` はデフォルトの形状をブロック、バー、またはアンダーラインから選択します。`CursorBlink` はデフォルトの点滅を有効にし、ゼロ値の場合は点滅せず点灯したままになります。プログラムは DECSCUSR または DEC モード 12 を通じて形状と点滅を上書きできます。フォーカスが外れたカーソルは中抜き（ホロー）で点灯したままになり、文字変換中（composition）や視差効果を減らす設定（reduced motion）でも点滅は停止します。点滅はプラットフォームのアニメーションクロックによって駆動され、ターミナルテキストのエンジン側での再描画は発生しません。

`View.Font`、`Theme`、`DarkTheme` でフォントとカラーパレットを設定します。グリッドは Unicode の罫線素片・ブロック要素文字および Powerline セパレータ（U+E0B0–U+E0B7）を自身で描画し、隣接するセルをデバイスピクセルに合わせて整列させます。その他のテキストにはプラットフォームのフォントとそのフォールバックが使用されます。

`MinimumContrast` は、反転色、淡色（faint）テキスト、選択範囲を含め、実際のセル背景に対する通常テキストのコントラストを引き上げます。値 1–21 は WCAG のコントラスト比を使用し、ゼロの場合は元の色をそのまま保持します。グラフィックスや絵文字の色は変更されません。`Transparent` を使用している場合でも、コントラスト計算にはターミナルに設定された背景色が使われます。

`Padding` はグリッドの外側にポイント単位で余白を確保します。`PaddingColor` はその背景色を選択します：

| 値 | 動作 |
| --- | --- |
| `PaddingBackground` | OSC 11 による変更を含むターミナルの背景色 |
| `PaddingExtend` | 最も近いセルの背景色。プライマリ画面では、プロンプト行、Powerline 行、またはデフォルト背景のセルを含む行の隣で上下の拡張を抑制する |
| `PaddingExtendAlways` | それらの例外なくすべての端を拡張する |

TSX アプリでは、`nsgo/base` に `Terminal` を登録する副作用のために `tsx/terminal` をインポートします。props には `cursorBlink`、`minimumContrast`、`paddingColor` があり、後者は `background`、`extend`、`extend-always` を受け取ります。外観に関する props はセッションの継続中でも変更できます。

## ブラウザでの動作

アプリには固定バージョンの libghostty-vt WebAssembly モジュールが組み込まれており、同じターミナルビューと入力パスを実行します。アプリのビルドには Zig、ネイティブライブラリのインストール、ランタイムのダウンロードはいずれも不要です。

ブラウザの WebSocket を `io.ReadWriteCloser` に適合させます。バイナリバイト列を送信し、受信メッセージはストリームとして結合または分割します。認証、再接続、リサイズや制御メッセージの処理はアプリとそのサーバーが担当し、NSGo はターミナルのバイト列を処理します。[ブラウザ向け受け入れフィクスチャ](https://github.com/nsgo-dev/nsgo/blob/main/internal/testhost/testdata/terminalweb/socket_js_test.go)に、クローズ処理を含む完全なバイトストリームアダプタの例があります。

## アクションとズーム

`View.Keybinds` は `Keybind{Key, Action, Amount}` を使ってデフォルト設定を上書きします。キーには `mod+=` や `ctrl+shift+c` など、`Scope.OnKey` と同じ名前を使用します。後のエントリが優先されます。キーバインドは、このターミナルの入力にフォーカスがあり、かつインプットメソッドで変換中でないときに実行されます。

アクションには `CopyToClipboard`、`PasteFromClipboard`、`SelectAll`、`ClearScreen`、`IncreaseFontSize`、`DecreaseFontSize`、`ResetFontSize`、`SetFontSize`、`Ignore` があります。ウィンドウやタブのアクションはアプリのキーマップに配置します。`Ignore` はプログラムに送信せずにキー入力を消費します。

デフォルトでは、`mod+=` と `mod+shift+=` でフォントサイズが 1 ポイント大きくなり、`mod+-` で小さくなり、`mod+0` で `View.Font.Size`（未設定時は 13）にリセットされます。`Amount` は増減のステップ幅（ゼロの場合は 1）、または `SetFontSize` 時のサイズを指定します。サイズは 1–255 ポイントの範囲にクランプされ、有限でない値のリクエストは無視されます。ズームを行ってもセッションと外側のビューは維持され、グリッドの行数と列数が更新されてプログラムがリサイズされます。

```go
ref := &terminal.Ref{}
view := terminal.View{
	Terminal: session, Ref: ref,
	Keybinds: []terminal.Keybind{
		{Key: "ctrl+shift+k", Action: terminal.ClearScreen},
		{Key: "mod+=", Action: terminal.IncreaseFontSize, Amount: 2},
	},
}
```

`Ref.FontSize()` はサイズを表示するコントロール向けにトラッキングされており、`Ref.SetFontSize(size)` と `Ref.Perform(action, amount)` はイベントハンドラと同様にアプリのエンジン上で実行されます。`Perform` はアクションが利用可能かどうかを返します。マウントされていない ref はサイズ 0 を返し、アクションを拒否します。

## コンテキストメニュー

副ボタンクリック（右クリック）で、ローカライズされたコピー、ペースト、すべて選択、クリア項目を含むスタイル未設定の `base.ContextMenu` が開きます。コピーには選択範囲が必要です。クリアはプロンプトより上の内容を消去し、代替画面（alternate screen）では無効になります。タッチスクリーンではコンテキストメニューの長押し表示が使用されます。サーフェスは `MenuClass`/`MenuStyle` で、各項目は `MenuItemClass`/`MenuItemStyle` でスタイル設定します。

`RightClickAction` には `RightClickPaste`、`RightClickCopy`、`RightClickCopyOrPaste`（選択範囲があればコピー、それ以外はペースト）、または `RightClickIgnore` を設定できます。Ignore は外側のアプリメニュー用にイベントを残すため、ref のアクションを呼び出すスタイル設定済みの `ui.ContextMenu` を提供する場合に使用します。

`nsgo/base` に `<Terminal>` を登録するには `tsx/terminal` をインポートします。リアクティブな `keybinds` および `rightClickAction` props は、セッションを維持したままビューを更新します。アクション値には `increase_font_size` や `copy_to_clipboard` のような Ghostty 名を使用し、右クリック値には `context-menu`、`paste`、`copy`、`copy-or-paste`、`ignore` を使用します。メニューのスタイル設定用 props には、Go のフィールド名をローワーキャメルケースにした名前を使用します。

## マウスレポート

マウストラッキングを有効にしているプログラムは、選択されたプロトコルで libghostty-vt によってエンコードされた、要求済みのプレス、リリース、ドラッグ、ホバーイベントを受け取ります。Shift キーを押すと、選択操作のためにクリックおよびドラッグのキャプチャをバイパスします。`View.MouseReporting` は省略可能なリアクティブな `func() bool` で、nil の場合は true になります。プログラムがキャプチャを要求している間でもマウス選択やネイティブのスクロールバックスクロールを維持したい場合は false を返してください。レポートが有効な間は、プログラムの右クリックが外側のコンテキストメニューよりも優先されます。

代替画面上で代替スクロールモード（alternate scroll mode）が有効であり、かつマウストラッキングが要求されていない場合、垂直方向のホイール入力は通常モードまたはアプリケーションカーソルキーモードに従ってカーソルキー入力になります。`MouseScrollMultiplier{Precision: 1, Discrete: 3}` はタッチパッドとホイールのデフォルトのレポート回数を設定します。ゼロの場合はこれらのデフォルト値が使われ、正の値は 0.01–10,000 の範囲にクランプされます。端数の入力は 1 ステップ分に達するまで蓄積されます。通常のスクロールバックでは、プラットフォームネイティブのスクロールと慣性が維持されます。

`nsgo/base` に `<Terminal>` を登録するには `tsx/terminal` をインポートします。`mouseReporting` prop のデフォルトは true で、`mouseScrollPrecision` と `mouseScrollDiscrete` で乗数を設定します。
