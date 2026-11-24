---
title: ウィジェット
description: iPhone や iPad のホーム画面およびロック画面、また Mac のデスクトップ上にアプリのビューを表示します。
group: プラットフォーム機能
order: 47
---

## ウィジェットの種類

ウィジェットは、アプリが実行中かどうかに関わらずアプリの概要を表示します。iPhone や iPad のホーム画面とロック画面、Mac のデスクトップと通知センター、そして Android のランチャーに表示されます。種類（kind）とはシステムのギャラリーが提供する 1 つのウィジェットを指し、対応するファミリーと併せて定義します：

```yaml
widgets:
  - kind: counter
    name: Counter                 # システムのウィジェットギャラリーに表示される名前
    description: Shows the count.
    families: [small, medium, circular]  # large、extra_large、rectangular、inline も指定可能
# apple: { app_group: group.dev.example.hello }  # 共有 App Group（デフォルトは group.<id>）
```

Apple プラットフォームではウィジェットエクステンションが描画を行い、`build` および `dev` が Xcode の Swift コンパイラでコンパイルします（[ビルドと配信](building.md#widgets-and-live-activities)）。Android ではビルド済みの Java ランタイムが同じタイムラインを App Widgets として描画します。どちらもアプリが停止した後も公開済みのコンテンツを表示し続けます。

サンドボックス化された Mac App Store ビルドでは、`apple.app_group` を設定し、`sign.macos_profile` と `sign.widgets_profile` を通じて macOS プロビジョニングプロファイルを指定してください。アプリ本体と `<id>.widgets` の両方が同じチームに属し、そのグループのエンタイトルメントを持つ必要があります。タイムラインと画像はその共有コンテナに保存されます。直接配布および開発用の macOS ビルドでは、引き続き従来のホームディレクトリ内の場所が使用されます。ビルドオプションについては[ストア署名](building.md#store-packages)を参照してください。

`name` と `description` にはアプリのカタログ内のメッセージ名を指定することもできます。その場合、ウィジェットギャラリーとプレースホルダーはユーザーの言語を使用し、翻訳が見つからない場合はソースカタログのテキストにフォールバックします。カタログキーではない値はそのまま表示されます。

## タイムライン

`widget.Update` は、指定した種類のウィジェットが今後表示する内容をエントリーのタイムラインとして書き込みます。システムはアプリを起動することなく、現在時刻より直前に日時を迎えたエントリーを表示し、その後は各エントリーの日時に合わせて順次切り替えます。ゼロ値の `Date` は現在時刻を意味します。エントリーがない場合、ウィジェットはその種類の名前と説明を表示します。Apple プラットフォームでは、フォアグラウンドにあるアプリは好きなだけ頻繁にウィジェットを更新でき、バックグラウンドでは 1 日に数十回程度更新できます。

```go
err := widget.Update("counter", widget.Entry{
	View: widget.Column{Style: widget.Style{Gap: 4}, Children: []widget.View{
		widget.Text{Content: "Count", Style: widget.Style{TextStyle: widget.Caption, Color: widget.Secondary}},
		widget.Text{Content: strconv.Itoa(n), Style: widget.Style{FontSize: 44, FontWeight: core.FontWeightBold}},
	}},
})
```

`Entry.Families` を使うと、特定のファミリーに対して `Entry.View` の代わりに専用のビューを表示できます。`Entry.Background` はシステムが確保する余白を越えてウィジェットの背面全体を塗りつぶし、`Entry.URL` はウィジェットがタップされたときにアプリで開かれます。ウィジェットなしでビルドされたアプリで `widget.Update` を呼ぶと `widget.ErrNoWidgets` を返し、macOS、iOS、Android 以外では `widget.ErrUnsupported` を返します。書き込み前にどちらに該当するかは `widget.Available` で確認できます。`Update` は任意のゴルーチンから実行でき、`nsgo.Run` の前でも呼び出せます。

## ビュー

ウィジェットのビューは `core` の要素ではありません。SwiftUI と同様にレイアウトされ、`Style` でサイズを指定したり領域いっぱいに広げたりしない限り、ビューはコンテンツと同じ大きさになり、コンテナは `AlignItems` と `Justify` で指定しない限り保持している要素を中央に配置します。`Text`、`Symbol`、およびバイト列や `Update` が読み込むファイルによる `Image` がコンテンツを表示し、`Column`、`Row`、`Stack`、`Spacer`、`Divider` がそれらを配置します。`Rectangle`、`Progress`、`Gauge` は図形やインジケーターを描画し、`Date` はシステムの書式で時刻を表示します。`Relative`、`Offset`、`Timer` は毎秒のエントリーを用意しなくても自動的にカウントを続けます。

```go
widget.Entry{
	View: widget.Column{Style: widget.Style{Gap: 4}, Children: []widget.View{
		widget.Text{Content: "Next hour in", Style: widget.Style{TextStyle: widget.Caption, Color: widget.Secondary}},
		widget.Date{Time: hour, Format: widget.Timer, Style: widget.Style{TextStyle: widget.Title2, FontVariant: core.FontTabularNums}},
		widget.Progress{Value: 0.4, Style: widget.Style{Color: widget.Accent}},
	}},
}
```

`Style` には、ギャップ、パディング、サイズまたはフィル、配置、角丸付きの背景、システムのテキストスタイルまたはフォントサイズ・ウェイト・デザイン・バリアント、行数制限、アプリで開く `Link`、およびシステムがティントカラーを適用する対象にビューを含める `Accented` を指定できます。`widget.Primary`、`Secondary`、`Accent` は外観モードに追従し、`RGB` と `RGBA` には `Dark` を使ってダークモード時の別カラーを指定できます。

## ボタン

`widget.Button` は iOS 17 および macOS 14 以降で、アプリを開かずに `widget.OnAction` を実行します。それより古いシステムでは子要素のみが表示されます。ハンドラーは `nsgo.Run` より前の `main` 内で登録してください。システムがビューなしでアプリをバックグラウンド起動する場合があるためです。ウィジェットの押下では `Activity == nil` となり、`Action` にボタンの値が渡されます。

```go
widget.Button{Action: "refresh", Children: []widget.View{
    widget.Text{Content: "Refresh"},
}}
```

ハンドラーが戻る前にアクションを処理して `widget.Update` を呼び出すことで、システムによる自動タイムラインリロードが更新結果を参照できるようになります。ハンドラーはビューのゴルーチン外で実行されるため、ビューを変更するには `core.Scope.Post` を使用してください。この仕組みは[ライブアクティビティ](live-activities.md)と同じハンドラーとインテントを使用しており、`nsgo.yaml` に `widgets` さえあれば `live_activities` なしで動作します。

## Android

ランチャーのウィジェットピッカーからウィジェットを追加し、そこでサイズを変更できます。レンダラーはその境界サイズから許可されているファミリーを選択します。縦横ともに 240 ポイント未満の場合は small、横 240 以上は medium、縦 240 以上は large、横 500 以上は extra-large となり、該当するファミリーがリストに含まれていない場合は最初にリストされたファミリーが使用されます。`Entry.URL` と `Style.Link` は、`url_schemes` で宣言されたスキームを通じてアプリを開きます。ネストされたビューに設定されたリンクは、そのコンテナのリンクよりも優先されます。

将来のタイムラインエントリーには Android の不正確なアラーム（inexact alarm）が使用されるため、アプリがバックグラウンドにある間はシステムによって遅延する場合があります。タイマービューはネイティブのクロノメーターを使用し、アプリなしでカウントを進めます。現在はサイズと色を反映しますが、カスタムのフォントウェイト、デザイン、イタリック体のスタイリングは今後の目標です。相対日時はシステムのスケジュールに従って更新されます。汎用レンダラーはテキスト、画像、レイアウト、図形、バー、リング、および一般的なシンボルグリフのセットをサポートしています。両プラットフォームで一致させる必要があるアートワークには `Image` を使用してください。任意の SF Symbol 名への対応は Android における今後の課題です。Android ウィジェットプロバイダーはスタンドアロンの APK および AAB に対して生成され、組み込みホストには生成されません。

## ライブアクティビティ

同じエクステンションが、ロック画面および Dynamic Island 上にアプリのライブアクティビティを描画します（[ライブアクティビティ](live-activities.md)）。

## 次のステップ

- [ライブアクティビティ](live-activities.md): アプリがリアルタイムの状況を追従する仕組み。
- [通知](notifications.md): ウィジェットの隣に表示される通知とバッジ。
- [ビルドと配信](building.md): エクステンションが共有する App Group。
