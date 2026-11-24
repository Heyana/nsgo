---
title: 画像とアイコン
description: URL、アプリのリソース、バイト列、またはアプリが描画したピクセルから画像を表示し、Lucide アイコンや SVG を描画します。
group: コアコンセプト
order: 54
---

## 画像を表示する

`core.Image` はビットマップや SVG を表示します。URL、ファイル、アプリのリソース内の `asset:` パス、あるいはアプリが保持するバイト列を指定できます。プラットフォームがメインスレッド外で読み込み・デコード・キャッシュを行い、メインスレッドで結果を描画します。

```go
core.Image{
	Source: core.Src("https://example.com/cover.jpg"),
	ResizeMode: core.Cover,
	Style: core.Style{Width: core.Pt(160), Height: core.Pt(100), CornerRadius: 12},
}
```

`core.Cover` はボックス全体を満たしてはみ出た部分を切り抜きます。`core.Contain` は画像全体を収め、`core.Center` は本来のサイズを保ち、`core.ScaleDown` はボックスより大きい場合を除いて本来のサイズを保ちます。`BlurRadius` はポイント単位で画像をぼかし、`TintColor` は不透明なピクセルを別の色で着色し、`ObjectPosition` はボックス内の配置位置を指定します。

スケーリングされたビットマップはデフォルトで滑らかなサンプリングを使用します。ピクセルアートやピクセルバッファのくっきりした輪郭を保つには、Go では `Sampling: core.ImageSamplingNearest` を、TSX では `sampling="nearest"` を設定します。Apple プラットフォーム、Windows、Linux、Android、Web はこの設定に対応しています。HarmonyOS とミニプログラムでも適用されますが、実機での出力確認はまだ行われていません。CSS の `pixelated` を使うブラウザで正確なピクセルエッジを得るには、元画像の 1 ピクセルが整数倍のデバイスピクセルになるように拡大してください。

`Source` はレイアウトが使用するサイズを宣言します。片方の辺を指定するともう一方の辺が導出され、サイズを持たない画像はスタイルでサイズ指定されるまで 0×0 になります。`core.ImageSource{Data: data, Size: core.Size{Width: 24, Height: 24}}` は `go:embed` で埋め込んだバイト列を表示し、`Size` は SVG に固有のサイズを与えます。

[`desktop.WindowPointerPainted`](services.md) を使用している場合、`HitTestAlpha: 0.5` を指定すると macOS と Windows で不透明度が半分未満のピクセルをポインタが透過するようになります。TSX では `hitTestAlpha` で同じしきい値を設定します。

## アプリが描画するフレーム

`Pixels` は、動画プレイヤーやエミュレータのフレームなど、アプリが描画した `*image.RGBA` をエンコード・読み込み・デコードなしで表示します。各フレームを同じバッファに書き込み、`Version` を増やすだけで、ビューはロードイベントやフェードなしで、アルファ値を保持したまま表示し、macOS、iOS、tvOS、watchOS、Android、Windows、Linux では間に空白フレームを挟むことなくその場で差し替えます。`Size` を指定しない場合はピクセル数をポイント単位として扱います。

```go
frame := image.NewRGBA(image.Rect(0, 0, 960, 540))
version := core.NewSignal(cx, uint64(0))
var tick func(time.Time)
tick = func(at time.Time) {
	if decoder.WriteDue(at, frame) { // 次のフレームを frame.Pix に書き込む
		version.Set(version.Get() + 1)
	}
	cx.RequestAnimationFrame(tick)
}
cx.RequestAnimationFrame(tick)

core.Image{ResizeMode: core.Contain, SourceFunc: func() core.ImageSource {
	return core.ImageSource{Pixels: frame, Version: version.Get()}
}}
```

NSGo は新しい `Version` を伴うコミットが実行されるタイミングでピクセルをコピーするため、アプリは次の `RequestAnimationFrame` コールバックから次のフレームを書き込めます。専用のゴルーチン上で動くデコーダは 2 つ目のバッファへ書き込み、差し替えを通知します。同じ `Version` は再描画されません。`cx.ImageSize` はピクセルソースに対してバッファのサイズを即座に返します。`core.Canvas` はすべてのプラットフォーム上で同じようにコピーされる `DrawImage` を使ってピクセルを描画します（ミニプログラムでは WeChat 2D キャンバスが必要です）。`core.Shader` の `Images` も同じ方法でピクセルを受け取るため、シェーダーを描画できるすべてのプラットフォームで、エミュレータの画面などアプリが書き込んだフレームにシェーダーでフィルタをかけることができます。

Apple プラットフォーム、Android、Windows、Linux、Web ではピクセル画像を直接更新し、WeChat ミニプログラムでは 2D キャンバスを使用します。HarmonyOS ではバージョンごとに `PixelMap` を生成し（実機で確認済み）、コピーが文字列ブリッジを経由するため、毎フレーム書き換えられる大きなバッファは 60 fps を大きく下回ります（[ピクセルバッファ](https://github.com/nsgo-dev/nsgo/blob/main/docs/architecture/images.md#pixel-buffers)）。

## ビューをキャプチャする

マウント済みビューの `core.Ref` を渡して `cx.Capture(&ref, done)` を呼び出すか、現在のサーフェス全体を対象にする場合は `cx.Capture(nil, done)` を呼び出します。コールバックはエンジンゴルーチン上で、サーフェスのピクセル倍率に応じた PNG バイト列またはエラーを受け取ります。結果を表示するには `core.ImageSource{Data: png}` を使用します。共有するには、バイト列をアプリのキャッシュディレクトリ内の `.png` ファイルに書き出し、そのパスを `core.ShareOptions.Files` に渡してください（[ファイルの共有](services.md#リンクとファイルを開く)）。

キャプチャはビューの境界内にある表示中のサーフェスピクセルを取得します。macOS、iOS、tvOS、Windows、Linux、Android、Web で利用できます。Android ではアクティビティのウィンドウをキャプチャします。Web のキャプチャでは、NSGo シェーダーの出力を含めてアプリの DOM をキャンバスに描画しますが、埋め込みフレーム、読み取れない画像、および一部の CSS エフェクトは除外されます。

## リソースと解像度スケール

`resources/` 配下のファイルはアプリに同梱されます。`core.Src("asset:images/cover.png")` で読み込み、`Scales: core.ScalesOf(1, 2, 3)` を指定すると画面のピクセル比に応じて `cover.png`、`cover@2x.png`、`cover@3x.png` が選択されます。

```go
core.Image{Source: core.ImageSource{
	URI:    "asset:images/cover.png",
	Scales: core.ScalesOf(1, 2, 3),
}}
```

`Placeholder` は画像の読み込み中に別のソースを表示し、`FadeDuration` は読み込まれた画像をその上にフェードインさせます。0 にするとプラットフォームの判断に委ねられ、1 ナノ秒を指定すると即座に表示されます。`OnLoadStart`、`OnProgress`、`OnLoad`、`OnError`、`OnLoadEnd` で読み込みの進行を追跡できます。`cx.ImageSize` は画像を表示せずにピクセル単位のサイズを読み取り、`cx.PrefetchImage` は画像をキャッシュに事前ダウンロードします。`cx.SetImageCacheLimit` はメモリ上のエンコード済み画像バイト数の上限を設定し（デフォルトは `core.DefaultImageCacheLimit` の 32 MiB、負の値は 0 に丸められます）、`cx.KeepImage` は戻り値の `release` が実行されるかスコープが破棄されるまで、アトラスやテクスチャをキャッシュの追い出し対象から外して固定します。この上限は固定されたもの、読み込み中のもの、現在の描画パスを除いた各キャッシュに適用され、デコード済みピクセル、プロセスメモリ、ブラウザキャッシュには上限を設けません。0 を指定すると固定されていない画像の保持を無効にします。ネイティブバックエンドはリモート画像を独自のディスクキャッシュに保存します。

## アニメーション画像とベクター画像

GIF および WebP アニメーションはビュー内で各フレームの遅延時間どおりに再生され、ファイルの指定に従ってループします。SVG ドキュメントは画像として読み込まれ、どのサイズでも鮮明に保たれます。単色の SVG は `TintColor` で着色できます。

## アイコン

`core.Icon` は継承したテキスト色で SVG を描画し、スタイルでサイズを指定しない限りフォントサイズと同じ高さになります。そのボックスは SVG のアスペクト比を維持するため、正方形のボックスに入れても引き伸ばされません。

```go
core.Icon{SVG: lucide.Search, Style: core.Style{FontSize: 20}, Label: "Search"}
```

`icons/lucide` には各 Lucide アイコン用のアクセサが PascalCase（`lucide.ArrowUpRight`）で用意されています。`Label` で名前を付けない限り、アイコンは支援技術に対して装飾として扱われます。`ui.Icon` は `ui` のデフォルト設定を備えた同様のコンポーネントであり、`core.SVG` と `core.SVGView` はアプリ自身が構築したパスを描画します。

## スタイル付き画像

`ui.Image` は角丸の枠内に画像を表示し、読み込み中はプレースホルダーを、読み込み失敗時は淡い画像アイコンを表示します。スタイルで縦横の両方が指定されていない限り、ソースのアスペクト比（不明な場合は 3:2）を維持します。

```go
ui.Image{
	Source:      core.Src("asset:images/cover.png"),
	Label:       "Cover",
	ResizeMode:  core.Cover,
	AspectRatio: 16.0 / 9.0,
	Placeholder: ui.Skeleton{Style: core.Style{Height: core.Pt(180)}},
}
```

## 次のステップ

- [テキスト](text.md): スパンとして行内に配置する画像。
- [スタイルとテーマ](styles.md): 角丸、シャドウ、ティント。
- [リストとスクロール](lists.md): 再利用される画像行。
