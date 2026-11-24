---
title: 変更を加える
description: 全バックエンドへの同一変更からテスト、ドキュメント、changelog の1行まで、NSGo へのプルリクエストに必要なこと。
group: コントリビュート
order: 82
---

## 始める前に

バグ修正より大きな変更を行う場合は、コードを書く前に設計を合意できるよう Issue または Discussion を開いてください。設計上未決の課題は [ARCHITECTURE.md](https://github.com/nsgo-dev/nsgo/blob/main/ARCHITECTURE.md#open-questions) に一覧されており、まずそこで結論を出します。

## すべてのバックエンド

あるバックエンドでの修正や機能追加は、通常ほかのバックエンドにも必要です。該当するすべてのターゲットに同じ変更を加えるか、足りないものを[ロードマップ](https://github.com/nsgo-dev/nsgo/blob/main/docs/roadmap.md)に記録してください。コンポーネントはどのプラットフォームでも同じ見た目・振る舞いになります。

## チェック

`go vet`、`go test`、`go build` ではなく `nsgo` ツールを使用してください：

```sh
go tool nsgo check   # vet every package
go tool nsgo test    # every package's tests
```

変更内容に対するテストを追加してください。ユーザーから見える振る舞いには `nsgotest`、それ以外にはパッケージ自身のテストを使います。触れたプラットフォーム上でギャラリーも実行してください（[開発セットアップ](../contributing.md#ギャラリー)）。

Go ファイルは `go.mod` に記載された Go バージョンの `gofmt` でフォーマットしてください。`gofmt -l .` でフォーマットが必要なファイルを確認できます。CI は追跡対象のすべての Go ファイルをチェックし、未フォーマットのファイル名を出力して失敗します。

## プルリクエスト

タイトルは `fix(linux): keep the caret after a paste` のように Conventional Commit 形式で付けてください。スカッシュマージ時にタイトルがそのままコミットメッセージの件名になるため、形式に合致しない場合はチェックが失敗します。PR テンプレートでは、変更が影響するターゲット、目に見える変更の前後スクリーンショット、実行したチェックの記載を求められます。

## ドキュメントと changelog

- 仕様が変わったドキュメントを更新してください。NSGo の内部仕組みについては `docs/architecture/` の該当トピックを、アプリやコンポーネントからの使い方については `docs/public/` のページを更新します（[ドキュメントとウェブサイト](docs.md)）。
- `README.md` と `README.zh-CN.md` の内容を揃えてください。その他のドキュメントは英語が基準で、中国語ページは `docs/public/zh/` に置きます。
- ユーザーから見える変更は `CHANGELOG.md` に直接書くのではなく、`changelog.d/` 内に専用のファイルを作り、`### Fixed` のような種別見出しの下に `(#12 by @you)` のような参照付きで1行で記録してください。リリース時にこれらのファイルから `CHANGELOG.md` が生成されます（[形式](https://github.com/nsgo-dev/nsgo/blob/main/changelog.d/README.md)）。
