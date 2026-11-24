---
title: キャンバスとシェーダー
description: エンジン上で 2D グラフィックスを描画し、GPU フラグメントシェーダーでビューを塗りつぶす。
group: コアコンセプト
order: 59
---

## キャンバス

`core.Canvas` はエンジン上で 2D 描画コマンドを記録し、Web ページの `<canvas>` のようにプラットフォーム側で再生します。`Draw` は読み取ったシグナル、キャンバスのサイズ、または利用可能なフォントが変化すると再実行され、プラットフォームがメインスレッドで記録内容を描画します。`cx.RegisterFont` でフォントを読み込むと利用可能になった時点で `Draw` が再実行され、テキストと `TextMetrics` に基づくジオメトリの両方が更新されます。

継承フォントの em 単位でサイズ指定するキャンバスには `Em` または `EmFunc` を使用します。幅に応じて高さが変わるコンテンツには、`MeasureEm(width, fontSize)` が利用可能なポイント単位の幅（非制限時は `core.Auto`）と解決済みフォントサイズに対する `EmBox` を返します。ここでの読み取りは追跡され、レイアウト中にも実行されるため、シグナルの書き込みは行わないでください。`Ref.ContentBox()` はマウント済みビューのパディングとボーダーの内側にあるレイアウトボックスを、ビュー自身の座標系で追跡します。

```go
core.Canvas{
	Style: core.Style{Height: core.Pt(220)},
	Draw: func(d *core.Drawing, size core.Size) {
		d.SetFillColor(core.RGB(0xf1f5f9)).FillRect(0, 0, float32(size.Width), float32(size.Height))
		d.SetStrokeColor(core.RGB(0x007aff)).SetLineWidth(2).SetLineCap(core.CapRound)
		d.BeginPath().MoveTo(24, 160).LineTo(90, 80).LineTo(160, 120).LineTo(230, 40).Stroke()
	},
}
```

記録 API は Web の `CanvasRenderingContext2D` に準拠しており、`Save` と `Restore`、座標変換、直線・曲線・矩形・円弧のパス、単色およびグラデーションによる塗りと線、クリッピング、破線、シャドウ、テキスト、画像を扱えます。`core.ParsePath` は SVG パスデータから `core.Path` を生成し、`FillFromView` と `StrokeFromView` はキャンバスビューが継承している `Fill` と `Stroke` スタイルを取り込むため、チャートをアプリのテーマに追従させられます。

Web では、ズームされた `core.Scroll` や拡大縮小された `Style.Transform` / `AnimatedStyle` の内側にあるキャンバスは、動きが静止した時点で累積スケール（画面のピクセル比の最大 4 倍まで）で再描画されます。ズーム後のビットマップが 4096 × 4096 ピクセルを超えるキャンバスは可視部分だけを描画してスクロールやパンに合わせて更新するため、`Draw` を再実行せずに線やグリフを鮮明に保てます。Apple、Android、Windows、Linux でも同様に、スクロールズームとスタイル変換の双方でズームされたキャンバスとテキストを鮮明に保ちます。

```go
d.SetFillGradient(core.CanvasLinear(0, 0, float32(size.Width), 0,
	core.GradientStop{Offset: 0, Color: core.RGB(0x4da3ff)},
	core.GradientStop{Offset: 1, Color: core.RGB(0x007aff)},
)).FillRect(0, 0, float32(size.Width), 40)
```

## テキスト、画像、パーツ

`MeasureText`、`TextMetrics`、`SetTextAlign` はプラットフォームのテキストエンジン上でテキストをレイアウトするため、ラベルにはアプリ内の他の箇所と同じフォントが使われます。`DrawImage`、`DrawImageIn`、`DrawImageFrom` は画像を描画し、`DrawSVG` と `DrawSVGIn` はパース済みの `*core.SVG` ドキュメント（`core.ParseSVG`）を本来のサイズまたは矩形内にフィットさせて描画します（`<text>` と `<tspan>` 要素はプラットフォーム自身のテキストエンジンで計測・描画され、`currentColor` は描画の塗りつぶし色から取得されます）。また `OnPress` 内では `IsPointInPath` と `IsPointInStroke` を使って直前の描画に対する当たり判定を行えます。ネイティブバックエンドは保持指定されていない直近 64 個のキャンバス画像ソースをキャッシュします。スプライトアトラスや長期利用するテクスチャをこのエビクション対象から外して固定するには、`release := cx.KeepImage(src)` を呼び出し、`release()` を呼ぶか `cx` が破棄されるまで保持します。

```go
badge := core.MustParseSVG([]byte(`<svg viewBox="0 0 120 40">
	<rect width="120" height="40" rx="8" fill="#007aff"/>
	<text x="60" y="25" text-anchor="middle" font-size="14" fill="#fff">Live</text>
</svg>`))

d.DrawSVGIn(badge, 12, 12, 180, 60)
```

拡大・縮小されるビットマップにはデフォルトでスムースサンプリングが使われます。ドット絵などのピクセルアートでは、`DrawImage`、`DrawImageFrom`、`DrawAtlas` の前に `d.SetImageSmoothing(false)` を呼ぶか、TSX では `ctx.imageSmoothingEnabled = false` を設定してください。`Save` と `Restore` は他の描画状態とともにこの設定も保存・復元します。最近傍補間（Nearest-neighbor）サンプリングは Apple プラットフォーム、Windows、Linux、Android、Web で利用でき、HarmonyOS とミニプログラムでも適用されますが出力は実機で未検証です。

`d.DrawVertices(image, vertices)` はテクスチャ付きメッシュを描画します。連続する 3 つの `core.CanvasVertex` 値が 1 つの三角形を構成します。`DrawImageFrom` と同様に、`Position` はキャンバスポイント単位、`TexCoord` は画像ポイント単位で指定します。各頂点の `Color` はテクスチャに乗算され、三角形全体で補間されます。ゼロ値の場合はテクスチャがそのまま使われます。ポイントを更新して描画を再記録することでメッシュを変形できます。画像にはピクセルバッファや `OffscreenImage` の `Source()` を含む任意の `ImageSource` を指定できます（Android のキャンバスとミニプログラムではまだオフスクリーン画像を描画できません）。メッシュにも `SetImageSmoothing(false)` が適用されます。Web で任意のメッシュを描画するにはインライン画像または同一オリジンの画像が必要です。ミニプログラムでは Web プレイヤーでメッシュを描画しますが、WeChat DevTools および実機では未検証です。HarmonyOS は各三角形をクリップしてアフィン変換で画像を描画します。`DrawImage` と同様にエンコード済み画像とピクセルバッファは描画できますがオフスクリーン画像は描画できず、色ティントは使えず各三角形の平均頂点アルファのみが適用されます。不透明な三角形同士の継ぎ目のみ隠蔽でき、実機では未検証です。

`DrawTintedImage`、`DrawTintedImageIn`、`DrawTintedImageFrom` は、合成前に画像の sRGB チャンネルとアルファチャンネルへ色を乗算します。デフォルトの source-over モードでは、透明ピクセルは背後の映像をそのまま透過させます。TSX では `drawImage` と同じ座標形式で `drawTintedImage(image, tint, ...args)` を使用します。Web 上のクロスオリジンソースは、ピクセルを着色するために匿名 CORS リクエストを許可している必要があります。HarmonyOS はバイト列、アプリのリソース、ピクセルバッファからの画像を着色できますが実機では未検証であり、オフスクリーン画像はまだ描画されません。

1 枚の画像から複数のスプライトを順番に描画するには `d.DrawAtlas(source, rects, xforms, alphas)` を使用します。各 `core.Bounds` は画像自身のポイント単位でタイル領域を選択します。対応する `core.RSXform` はタイルの左上角を `TX, TY` へマッピングし、`SCos` と `SSin` にはスケールに回転角のコサインとサインを掛けた値を設定します。たとえば `core.RSXform{SCos: 0.5, TX: 40, TY: 20}` は 32 ポイントのタイルを半分のサイズで `(40, 20)` に描画します。不透明度をそのままにする場合は `nil` を渡し、現在の描画アルファに乗算する係数配列を渡すこともできます。

`Drawing.Node` はチャートの棒や地図のリージョンなど描画内の一部に対し、境界矩形、ロール、ラベル、選択状態を指定して名前を付けます。支援技術はこれらのパーツをキャンバスの子要素として読み上げ、テストからはラベル・テキスト・テスト ID で検索できます。いずれかのパーツに対する押下は、その中心点への押下として扱われます。

```go
d.Node(core.CanvasNode{
	Bounds: core.Bounds{Origin: core.Point{X: x, Y: y}, Size: core.Size{Width: w, Height: h}},
	Role:   core.RoleButton,
	Label:  "Open the report",
	TestID: "report",
})
```

## オフスクリーン画像

`cx.NewOffscreenImage(size, scale, draw)` は、`core.Drawing` を保持可能なオフスクリーン画像へ一度レンダリングし、以降のフレームで `DrawImage`、`DrawImageIn`、`DrawImageFrom`、`DrawAtlas`、または `core.Image` から描画できるようにします。`scale <= 0` を渡すと現在のディスプレイピクセル比が使用されます。描画は `Invalidate()` または `Resize(size, scale)` が呼ばれたとき（あるいは新しいフォントが登録されたとき）にのみ再レンダリングされ、`cx` が破棄されるか `Release()` が呼ばれると自動的に解放されます。

```go
tiles := cx.NewOffscreenImage(core.Size{Width: 512, Height: 512}, 0, func(d *core.Drawing) {
	// Drawn once into the offscreen image until tiles.Invalidate() is called.
	for y := range 16 {
		for x := range 16 {
			d.SetFillColor(tileColor(x, y)).FillRect(float32(x*32), float32(y*32), 32, 32)
		}
	}
})

core.Canvas{
	Style: core.Style{Width: core.Pt(320), Height: core.Pt(240)},
	Draw: func(d *core.Drawing, _ core.Size) {
		// Reuses the cached surface in a single DrawImage operation per frame.
		d.DrawImage(tiles.Source(), -cameraX.Get(), -cameraY.Get())
	},
}
```

## アニメーション引数

`Drawing.Bind` は直前の描画操作の引数をアニメーションノードにバインドし、プラットフォームはコミットを発生させずにそのフレームクロック上で該当箇所だけを再描画します。引数は `core.ArgX`、`ArgY`、`ArgR`、`ArgA`、`ArgColor` で指定します。macOS、iOS、Linux では変更された領域だけを再描画し、Windows は再記録を行い、Android と Web はキャンバス全体を再描画します。

```go
needle := core.NewAnimatedValue(cx, 0)
d.Save().Translate(cx, cy).Rotate(0).Bind(core.ArgA, needle).
	BeginPath().MoveTo(-14, -3).LineTo(80, 0).LineTo(-14, 3).ClosePath().Fill().Restore()
```

## シェーダー

`core.Shader` は、GPU がピクセルごとに実行するフラグメントシェーダーで自身のボックスを塗りつぶします。`core.CompileShader` は `Fragment` 関数を持つ Go 構文のプログラムを受け取り、`core.CompileWGSL` は WGSL を受け取ります。`Uniforms` でプログラムのエクスポート済み変数を設定し、`Images` で最大 4 つのテクスチャを渡せます。

```go
var program = core.MustCompileShader(`
package main

var Tint vec4

func Fragment(dst vec4, pos vec2) vec4 {
	v := 0.5 + 0.5*sin(pos.x/40)
	return Tint * vec4(v, v, v, 1)
}
`)

core.Shader{
	Program:  program,
	Uniforms: core.Uniforms{"Tint": core.RGB(0x007aff)},
	Style:    core.Style{Height: core.Pt(220), CornerRadius: 16},
}
```

Go シェーダーは `dst` にピクセル中心、`pos` にポイント単位の同じ座標を受け取り、GLSL の組み込み関数に加えて NSGo 独自の `rectSize`、`pixelScale`、`image0At`〜`image3At` および各画像サイズを利用できます。WGSL プログラムでは `fn shade(uv: vec2f) -> vec4f` を記述し、`nsh.time` と `nsh.size` を参照できます。`Paused` を指定するとクロックが停止します。シェーダー用の GPU パスを持たないプラットフォームでは何も描画されません。

`core.ShaderEffect` は子要素を WGSL 経由で描画し、`core.ScreenEffect` はサーフェス全体を覆います。`core.ShaderTransition` と `core.ScreenTransition` は `Value` が変化する前後の画像をサンプリングします。`core.TransitionShader` はナビゲーションやダイアログに対して同じ仕組みを適用します。Android でエフェクト向けにビューをキャプチャするには、Android 10 以降と外部テクスチャ対応の OpenGL ES 3.0 が必要です。`core.Canvas` と `core.Shader` はどちらも `Label` を受け取り、子要素はその上に描画されます。

## 次のステップ

- [画像とアイコン](images.md): キャンバスが描画する画像。
- [アニメーション](animation.md): キャンバスがバインドする値。
- [アクセシビリティ](accessibility.md): キャンバスが公開するパーツ。
