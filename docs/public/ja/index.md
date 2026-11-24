---
title: イントロダクション
description: NSGo とは何か、どの環境で動作し、どのように機能するか。
order: 1
---

NSGo は Go で書かれたクロスプラットフォームのネイティブ UI フレームワークです。Go または TSX でアプリを一度書けば、iOS とその App Clips、tvOS、watchOS、Android、macOS、Windows、Linux、そして Web 上で動作し、各プラットフォーム独自のビューによって描画されます。また、実験的機能として WeChat、QQ、Alipay、Douyin のミニプログラムでも Go アプリが動作します。ただし、これらのプラットフォームではアプリのサイズ上限が 2 MB に制限されているため、規模の大きいアプリはこの上限を超えます。HarmonyOS ではプレビューとして Go アプリが動作し、ArkUI によって描画されます。

## 書くコード

アプリは Go の main パッケージです。そのコンポーネントは、ビュー、テキスト、画像、入力フィールド、`ui` に含まれるコントロールといった要素（element）を返します。状態はシグナルに保持され、シグナルが変化したときには、そのシグナルを読み取っている部分だけが更新されます。

```go
func Root(cx *core.Scope) core.Node {
	count := core.NewSignal(cx, 0)
	return core.View{Style: core.Style{Padding: core.All(24), Gap: 12}, Children: []core.Node{
		core.Text{ContentFunc: func() string { return fmt.Sprintf("Pressed %d times", count.Get()) }},
		ui.Button{Text: "Press me", OnPress: func() { count.Update(func(n int) int { return n + 1 }) }},
	}}
}
```

同じアプリを TSX で書き、Tailwind クラスでスタイルを設定することもできます（[TSX アプリ](tsx.md)）。

## 動作の仕組み

リアクティブグラフ、スタイル、フレックスボックスおよびグリッドレイアウト、テキストの準備といったアプリの中核は Go エンジンが管理します。各更新では変更のバッチが 1 つ生成され、薄いバックエンドがそれをプラットフォームのネイティブビューに適用します。アニメーションはプラットフォームのフレームクロック上で動作するため、フレームごとにコミット処理を行う必要がありません。

| プラットフォーム | 描画方式 |
| --- | --- |
| iOS | UIKit と CoreText |
| Android | Android Views |
| macOS | AppKit と CoreText |
| Windows | Direct2D と DirectWrite |
| Linux | GTK 4、Pango、Cairo |
| Web | WebAssembly と DOM 要素 |

Go をインストールした Mac があればすべてのターゲットをビルドできます（iOS には Xcode も必要です）。`nsgo` ツールはアプリの作成、実行、テスト、パッケージ化を行います。

## ステータス

NSGo はまだ初期段階にあります。0.1.0 が最初のリリースであり、API はまだ安定していません。未実装の機能は[ロードマップ](https://github.com/nsgo-dev/nsgo/blob/main/docs/roadmap.md)に記載されています。[Native Shell](https://github.com/longbridge/nsh) の設計を Go に移植したものです。

## ドキュメントの読み方

各ページは 1 つの疑問に答える構成になっています。特に断りがない限り、コード例はコンポーネントの一部です。`cx` はコンポーネントが受け取る `*core.Scope` を指します。アプリでは見た目の付いた `ui` のコンポーネントを使用します。`base` には同じコントロールが見た目のない状態で用意されており、コンポーネントの作者向けです。

- **はじめに**: まず[セットアップ](getting-started.md)を行い、次に段階的に構築するサンプルアプリ（[Todo アプリ](tutorials/todo.md)、[メモアプリ](tutorials/notes.md)、[人気の GitHub リポジトリ](tutorials/repos.md)）へ進みます。
- **ツールチェーン**: [実行とデバッグ](debugging.md)、[テスト](testing.md)、[ビルドとリリース](building.md)、[埋め込み](embedding.md)。
- **コンポーネント**: [よく使われるコンポーネント](ui.md)、そのバリアント、状態、アクセシビリティ。
- **コアコンセプト**: [コンポーネントと状態](components.md)、[レイアウト](layout.md)、[スタイル](styles.md)、[テキスト](text.md)、[リスト](lists.md)、[ナビゲーション](navigation.md)、[フォーム](forms.md)、[ローカライズ](localization.md)、[TSX](tsx.md) など。
- **プラットフォーム機能**: [プラットフォームサービス](services.md)、[通知](notifications.md)、[ハプティクス](haptics.md)、[ウィジェット](widgets.md)、[ライブアクティビティ](live-activities.md)、[権限](permissions.md)。
- **コンポーネントを書く**: [他の人のアプリ向けのコンポーネント](authoring/index.md)（スタイルなしおよびスタイル付き）。
- **コントリビュート**: [ソースから NSGo をビルド](contributing.md)し、コードベースを把握して変更を送る方法。
- [ライブデモ](https://nsgo.dev/gallery/): すべてのコンポーネントをブラウザ上で試せます。
