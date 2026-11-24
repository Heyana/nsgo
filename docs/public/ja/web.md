---
title: Web での公開
description: ドキュメントのセマンティクス、ページメタデータ、CDN アセットを備えて NSGo アプリを公開します。
group: ツールチェーン
order: 24
---

## アプリのビルド

```sh
go tool nsgo build -target web
```

生成されたディレクトリを HTTP で配信します。リリース用アセットの名前にはコンテンツハッシュが含まれるため immutable としてキャッシュし、`index.html`、`nsgo.json`、`manifest.webmanifest` は再検証してください。Web アプリは NSGo のレイアウト、スクロール、フォーカス、ポインターの挙動をそのまま維持します。

## 見出しとリンク

`Role: core.RoleHeader` を持つ `core.View` または `core.Text` は、`AccessibilityLevel` に応じて `h1` から `h6` になります。レベルの指定がない場合は `h2` になります。`base/textview` の Markdown 見出しはそれぞれのレベルを引き継ぎます。ブラウザーの見出しの余白やフォントによってアプリのスタイリングが変わることはありません。

`base.Link` と `ui.Link` は `Href` をアンカーとして公開します。通常のクリックではアプリの `OnOpen` と `OnPress` が実行され、中クリックや修飾キー付きのクリックではブラウザーのリンク動作が使われます。アプリが `OnContextMenu` を処理しない限り、ブラウザーがリンク用のコンテキストメニューを提供します。無効化されたリンクには有効なアドレスがありません。カスタムのリンクロールを持つビューでは、`core.View.Web` に `core.WebViewProps{Href: "/notes"}` を設定するか、`HrefFunc` をバインドしてください。TSX では `web={{href: "/notes"}}` を、見出しには `accessibilityLevel={1}` を使用します。安全でない実行可能な URL スキームは拒否されます。閉じ込められたコンテンツ（confined content）はナビゲーションをスコープ内の押下ハンドラー内に留め、ブラウザー向けのアドレスを公開しません。

## ページメタデータ

`github.com/nsgo-dev/nsgo/web` をインポートし、ページのコンテンツの隣に `web.Head` を配置します：

```go
core.Group{
	web.Head{Title: "Notes", Description: "Your saved notes"},
	pageContent,
}
```

`TitleFunc` と `DescriptionFunc` はシグナルに追従します。後から表示されたヘッドは設定したフィールドを上書きし、他のフィールドは継承します。ページを非表示にするか削除すると前のヘッドが復元され、すべてのヘッドを削除すると元のドキュメントが復元されます。空の静的フィールドは継承し、空文字列を返す関数はそのフィールドをクリアします。ヘッドはレイアウトノードを追加しません。ネイティブターゲット、組み込みアプリ、および閉じ込められたコンテンツはホスト側のドキュメントを変更しません。

TSX では、`nsgo/web` から `Head` をインポートして `<Head title={title()} description="Your saved notes" />` のように記述します。静的 HTML プレビューの場合はサイトのデプロイの一部として各 URL をプリレンダリングしてください。`Head` は実行中のドキュメントを更新します。

## CDN からのアセット配信

ローダーはすべてのアプリファイルを `nsgo.json` からの相対パスで解決します。ページは別のオリジンからアプリを読み込むこともできます：

```js
const url = new URL("https://cdn.example.com/notes/nsgo.json");
const config = await (await fetch(url)).json();
const { mount } = await import(new URL(config.loader, url).href);
await mount(null, url.href, { config });
```

ページのオリジンが CORS 経由で CDN のファイルを取得できるように許可し、WebAssembly を `application/wasm` として配信してください。アプリが使用する任意のディープルートに対してアプリのエントリ HTML を配信します。代わりに要素内へマウントする方法については、[組み込み](embedding.md#web)を参照してください。
