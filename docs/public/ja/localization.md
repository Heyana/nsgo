---
title: ローカライゼーション
description: アプリのメッセージを翻訳し、表示言語に追従します。
group: コアコンセプト
order: 62
---

## メッセージ

`nsgo.yaml` の隣にある `locales/` ディレクトリには、アプリのメッセージを言語ごとの YAML ファイル（`en.yaml`、`de.yaml`）として ICU MessageFormat 形式で格納します。

```yaml
greeting: Hello, {name}!
appName:
  message: Hello
  description: The app's name, for translators.
```

メッセージは単なる文字列、または翻訳者向けの `description` と `placeholders` を含む `message` オブジェクトで記述します。

パッケージは `locales/` をインポートします。`dev`、`build`、`test` はカタログを検証し、その隣にメッセージごとの Go 関数（例: `locales.Greeting(cx, "Ada")`）を生成します。生成された関数はアプリが表示中の言語で文言を返し、言語が切り替わると追従します。関数の生成だけを行う場合は `go tool nsgo i18n` を実行します。メッセージでは数値や日付をフォーマットでき（`{d, date, medium}`）、`appName` はパッケージ内で各言語におけるアプリ名を設定します。同様に、`NSCameraUsageDescription` のように `apple.info` キーを持つメッセージは、対応する Info.plist エントリを翻訳します（[パーミッション](permissions.md#利用目的の説明)）。

## 言語

`cx.SetLocale("de")` で言語を選択すると、レイアウトはその言語の書字方向に従って配置されます。スタンドアロンアプリでは、テキスト選択メニュー、ファイルダイアログ、GTK のウィジェットなど、システム自身が返す文言も指定された言語に追従します（Android 13 以降、Windows、Linux では即座に反映され、macOS と iOS では次回起動時から反映されます）。`cx.SetLocale("")` を呼ぶと、再びユーザーのシステム言語に追従します。

```sh
go tool nsgo dev -lang en-XA
```

を実行すると、未翻訳のテキストや見切れたテキストを確認できる疑似ロケールでアプリが起動します。すべてのビルドで起動時に `--nsgo-lang=en-XA` を指定することもできます。

## 週、時刻、単位

`i18n.Regional(cx)` は、言語とは別にユーザーが週、時刻、単位をどのように読むかを返します。具体的には、週の開始曜日、12 時間制または 24 時間制、計量単位系、温度単位を、プラットフォームでのユーザー設定に従って（未設定の場合は地域設定に従って）取得します。アプリが `FirstDayOfWeek` や `HourCycle` を明示的に設定しない限り、カレンダーと日付ピッカーはその開始曜日を使用し、時刻フィールドはその時間制で表示され、カタログのメッセージが出力する時刻もユーザーの時間制に従います。

## NSGo 自身の文言

NSGo のコンポーネントは、NSGo が翻訳されている各言語で文言を表示します。それ以外の言語に対応する場合は、`github.com/nsgo-dev/nsgo/base/locales` のようにパッケージ名をキーとして指定することで、そのファイルの対象言語でそのパッケージのメッセージを提供できます。プラットフォームのメニューや、タブや検索ボックスなどシステム側に対応する文言がない箇所で Android および Windows のスクリーンリーダーが読み上げるロールと状態には、`github.com/nsgo-dev/nsgo/desktop/locales` の文言が使用されます。
