---
title: ドキュメントとウェブサイト
description: ドキュメントのページを執筆・翻訳し、ウェブサイトを実行してエージェント用スキルを最新に保つ。
group: コントリビュート
order: 83
---

## ページ

これらのドキュメントは `docs/public/` にある Markdown ファイルであり、そのまま GitHub 上で読める形式になっています。`en/` と `zh/` はパス構成が1対1で対応しており、中国語版が未翻訳のページでは注記付きで英語版が表示されます。各ページは次のフロントマターで始まります：

```yaml
---
title: Testing
description: Test an app the way a user uses it, headless or in a native window.
group: Toolchain
order: 21
---
```

- `group` はサイドバーでのページのグループ名（そのページの言語で記述）であり、`order` は表示順を決めます。グループは読者の導線に沿って、チュートリアル、ツールチェーン、コンポーネント、コアコンセプト、コンポーネントを書く、コントリビュートの順に並びます。はじめにページのように `group` を持たないページはそれらの前に表示されます。
- レイアウト側でタイトルと説明文を表示するため、本文は `##` から始めます。

ページ間のリンクには相対 `.md` リンク（`[テスト](testing.md#run-on-a-platform)`）を使ってください。ウェブサイトがこれを適切な URL に変換します。各ページは1つの疑問に答える構成にし、NSGo の内部実装の解説は `docs/architecture/` に置きます。

## ウェブサイト

`website/` にあるウェブサイトは、`website/site.config.ts` の設定をもとに Astro インテグレーションである [nsgo-website](https://github.com/nsgo-dev/nsgo-website) でビルドされます。[Bun](https://bun.com) 1.4 以降が必要です：

```sh
cd website
bun install
bun run dev    # at http://localhost:4321/
bun run build  # type-check and build, as the deployment does
```

ビルドと公開の方法は `website/README.md` に記載されています。

## エージェント向け

ウェブサイトでは言語モデル向けに英語ページも公開しています。`/llms.txt` が一覧を提供し、`/llms-full.txt` に全ページがまとめられ、`/docs/<page>.md` が絶対リンク付き・コードブロック修飾なしで各ページを提供します（`src/lib/llms.ts`）。これらはページに自動で追従するため、個別の更新手順は不要です。

`skills/nsgo/` はエージェントが `bunx skills add nsgo-dev/nsgo` でインストールするスキルです。アプリが従うルールをまとめた `SKILL.md` と、タスクに応じて読み込まれる `references/` で構成されています。アプリの書き方、ツールのコマンド、テスト、パッケージングに変更がある場合はドキュメントと同じ変更内で更新してください。内容はエージェントが守るべき事項に絞り、詳細はドキュメントへリンクしてください。`go tool nsgo test -run TestSkills .` はフロントマター、リンク、および言及されているすべての NSGo 名を検証します。
