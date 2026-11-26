---
title: 画布与着色器
description: 在引擎上绘制 2D 图形，并用 GPU 片元着色器填满一个视图。
group: 核心概念
order: 59
---

## 画布

`core.Canvas` 在引擎上记录 2D 绘图，再由平台播放，像网页的 `<canvas>`。`Draw` 会在它读取的 signal、画布尺寸或可用字体变化时再次运行，平台在主线程上绘制记录。用 `cx.RegisterFont` 加载的字体可用后，`Draw` 会再运行一次，文字和按 `TextMetrics` 算出的几何都随之更新。

用 `Em` 或 `EmFunc` 让画布按继承字体的 em 定尺寸。内容高度随宽度变化时，`MeasureEm(width, fontSize)` 按可用宽度（点数，无约束时为 `core.Auto`）和解析后的字号返回 `EmBox`。它读取的 signal 会被跟踪；布局期间也会调用它，因此不能在里面修改 signal。`Ref.ContentBox()` 跟踪已挂载视图扣除内边距和边框后的布局区域，坐标相对于视图自身。

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

记录接口遵循网页的 `CanvasRenderingContext2D`：`Save` 和 `Restore`、变换、直线与曲线路径、矩形和圆弧、颜色和渐变填充与描边、裁剪、虚线、阴影、文本和图片。`core.ParsePath` 从 SVG 路径数据生成 `core.Path`，`FillFromView` 和 `StrokeFromView` 使用画布视图继承的 `Fill` 和 `Stroke` 样式，因此图表会跟随应用主题。

```go
d.SetFillGradient(core.CanvasLinear(0, 0, float32(size.Width), 0,
	core.GradientStop{Offset: 0, Color: core.RGB(0x4da3ff)},
	core.GradientStop{Offset: 1, Color: core.RGB(0x007aff)},
)).FillRect(0, 0, float32(size.Width), 40)
```

## 分辨率

`ResolutionScale` 让画布用更少的像素绘制，再拉伸到它的区域，像游戏的画质设置：0.5 在屏幕像素密度及画布外的缩放或变换之上，每边只画一半的像素。零和 (0, 1) 之外的值保持屏幕的像素密度。拉伸默认平滑，`Sampling` 设为 `core.ImageSamplingNearest` 时取最近的像素，像素画保持方块。`ResolutionScaleFunc` 和 `SamplingFunc` 跟随 signal，重新播放上一次的绘制而不再运行 `Draw`。命中测试、`OnPress` 和 `Draw` 拿到的尺寸仍以点为单位。HarmonyOS 仍按屏幕的像素密度绘制。

```go
core.Canvas{
	Style:               core.Style{FlexGrow: 1},
	ResolutionScaleFunc: func() float32 { return quality.Get() }, // 低画质时为 0.5
	Sampling:            core.ImageSamplingNearest,
	Draw:                drawScene,
}
```

## 文本、图片与部件

`MeasureText`、`TextMetrics` 和 `SetTextAlign` 用平台的文本引擎排版，因此标签与应用其余部分使用相同的字体。`DrawImage`、`DrawImageIn` 和 `DrawImageFrom` 绘制图片，`DrawSVG` 和 `DrawSVGIn` 将解析后的 `*core.SVG` 文档（`core.ParseSVG`）按固有尺寸或适配到目标矩形绘制到画布上——其中的 `<text>` 和 `<tspan>` 元素由平台自身的文本引擎测量和绘制，`currentColor` 取画布当前的填充色——`OnPress` 可以用 `IsPointInPath` 和 `IsPointInStroke` 测试最后一次绘制。

```go
badge := core.MustParseSVG([]byte(`<svg viewBox="0 0 120 40">
	<rect width="120" height="40" rx="8" fill="#007aff"/>
	<text x="60" y="25" text-anchor="middle" font-size="14" fill="#fff">Live</text>
</svg>`))

d.DrawSVGIn(badge, 12, 12, 180, 60)
```

位图缩放默认使用平滑采样。绘制像素画时，在 `DrawImage`、`DrawImageFrom` 或 `DrawAtlas` 前调用 `d.SetImageSmoothing(false)`，或在 TSX 中设置 `ctx.imageSmoothingEnabled = false`。`Save` 和 `Restore` 会与其余绘图状态一起保存和恢复此设置。Apple 平台、Windows、Linux、Android 和 Web 支持最近邻采样；HarmonyOS 和小程序也会应用它，但其输出尚未在设备上验证。

`d.DrawVertices(image, vertices)` 绘制带纹理的网格。每三个连续的 `core.CanvasVertex` 构成一个三角形。`Position` 以画布点为单位，`TexCoord` 与 `DrawImageFrom` 一样以图像点为单位。每个顶点的 `Color` 会在三角形内插值后乘到纹理颜色上，零值保留纹理原色。修改顶点并重新记录绘图即可使网格变形。图像可以是任意 `ImageSource`，包括像素缓冲区和 `OffscreenImage` 的 `Source()`；Android 画布和小程序暂不绘制离屏图像。`SetImageSmoothing(false)` 同样作用于网格。Web 绘制任意网格时使用内联或同源图像。小程序用 Web 的播放器绘制网格，尚未在微信开发者工具或真机上验证。HarmonyOS 逐个裁剪三角形，按其仿射变换绘制图像；与 `DrawImage` 一样，它绘制编码图像和像素缓冲区，不绘制离屏图像，并且只应用每个三角形顶点的平均透明度，不应用颜色着色。它只消除不透明三角形之间的接缝，尚未在设备上验证。

`DrawTintedImage`、`DrawTintedImageIn` 和 `DrawTintedImageFrom` 在合成前将图像的 sRGB 和透明度通道分别乘以着色颜色的对应通道。默认的 source-over 模式会透过透明像素显示背景。TSX 使用 `drawTintedImage(image, tint, ...args)`，坐标形式与 `drawImage` 相同。网页上的跨域图像须允许匿名 CORS 请求才能着色。HarmonyOS 可为来自字节、应用资源或像素缓冲区的图像着色，但尚未在设备上验证；离屏图像在 HarmonyOS 上暂不绘制。

用 `d.DrawAtlas(source, rects, xforms, alphas)` 按顺序绘制同一张图集的多个精灵。每个 `core.Bounds` 以图片自身的点为单位选择一个区域。对应的 `core.RSXform` 将区域的左上角放在 `TX, TY`，`SCos` 和 `SSin` 分别是缩放倍数乘以旋转角的余弦和正弦。例如，`core.RSXform{SCos: 0.5, TX: 40, TY: 20}` 将 32 点的区域缩到半宽半高，绘制在 (40, 20)。透明度传 `nil` 时沿用当前绘图透明度，也可以为每个精灵指定相乘的系数。

`Drawing.Node` 给绘图的一部分命名，例如图表的柱或地图的区域，带边界、角色、标签和选中状态。辅助技术把这些部分当作画布的子元素，测试可以按标签、文本或 test ID 找到它们；按下其中一个就是按下它的中心。

```go
d.Node(core.CanvasNode{
	Bounds: core.Bounds{Origin: core.Point{X: x, Y: y}, Size: core.Size{Width: w, Height: h}},
	Role:   core.RoleButton,
	Label:  "打开报告",
	TestID: "report",
})
```

## 动画参数

`Drawing.Bind` 把最后一次操作的某个参数绑定到动画节点，平台在帧时钟上只重绘那部分，不需要提交。`core.ArgX`、`ArgY`、`ArgR`、`ArgA` 和 `ArgColor` 指定参数；macOS、iOS 和 Linux 只重绘变化的区域，Windows 重新记录，Android 和 Web 重绘整个画布。

```go
needle := core.NewAnimatedValue(cx, 0)
d.Save().Translate(cx, cy).Rotate(0).Bind(core.ArgA, needle).
	BeginPath().MoveTo(-14, -3).LineTo(80, 0).LineTo(-14, 3).ClosePath().Fill().Restore()
```

## 着色器

`core.Shader` 用 GPU 为每个像素运行的片元着色器填满盒子。`core.CompileShader` 接受用 Go 语法写的程序，其中有一个 `Fragment` 函数；`core.CompileWGSL` 接受 WGSL。`Uniforms` 设置程序导出的变量，`Images` 给它最多四张纹理。

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

Go 着色器在 `dst` 中收到像素中心，在 `pos` 中收到同一个点的点坐标，并可以使用 GLSL 的内建函数；`rectSize`、`pixelScale`、`image0At` 到 `image3At` 及其尺寸是 NSGo 的扩展。WGSL 程序写 `fn shade(uv: vec2f) -> vec4f`，读取 `nsh.time` 和 `nsh.size`；`Paused` 停止它的时钟。没有 GPU 着色器路径的平台在那里不绘制内容。

画布也能运行着色程序，按绘制顺序执行，并遵循画布的变换、裁剪、透明度和合成方式。`program.Paint(uniforms, images...)` 生成一个 `core.ShaderPaint`。`d.DrawShader(paint, x, y, w, h)` 着色一个矩形：`pos` 是矩形自身的点坐标，`rectSize` 是它的尺寸，`pixelScale` 是它的一个点在屏幕上占的像素数。`d.SetShader(paint)` 着色此后 `DrawImage`、`DrawImageIn`、`DrawImageFrom`、`DrawAtlas` 和 `DrawVertices` 绘制的图片，直到 `d.ClearShader()`；`Save` 和 `Restore` 会保存和恢复这项设置。被绘制的图片就是 `image0`，在 `dst.xy` 处读取，单位是它的像素；`pos` 是同一点，单位是它的点；paint 自带的图片从 `image1` 开始。程序输出的颜色会乘上每个精灵的透明度、每个顶点的颜色和着色色调。一次 `DrawAtlas` 调用就能在一个 pass 里着色一整串来自有向距离场（SDF）图集的字形：

```go
var sdf = core.MustCompileShader(`
package main

var Color vec4
var Glow float

func Fragment(dst vec4, pos vec2) vec4 {
	d := image0At(dst.xy).a
	w := fwidth(d)
	return vec4(Color.rgb*Color.a, Color.a) * smoothstep(0.5-w-Glow, 0.5+w, d)
}
`)

d.SetShader(sdf.Paint(core.Uniforms{"Color": core.RGB(0xff3b30), "Glow": pulse}))
d.DrawAtlas(glyphs, rects, xforms, nil)
d.ClearShader()
```

`image0At` 读取某点所在的纹素，因此放大绘制的程序需要自己混合相邻纹素。uniform 中的 `AnimatedNode` 会让画布在帧时钟上重绘，无需提交，和 `Bind` 一样。WGSL 程序在这里读到的 `nsh.time` 为 0。Apple 各平台、Windows、Linux 和 Web 支持画布着色；Android、HarmonyOS、watchOS 和小程序会跳过 `DrawShader`，着色的图片按普通图片绘制。

`core.ShaderEffect` 用 WGSL 绘制子元素，`core.ScreenEffect` 作用于整个界面。`core.ShaderTransition` 和 `core.ScreenTransition` 对 `Value` 变化前后的画面采样，`core.TransitionShader` 用于导航和对话框转场。在 Android 上，效果捕获需要 Android 10 或更新版本，并且 OpenGL ES 3.0 支持外部纹理。`core.Canvas` 和 `core.Shader` 都接受 `Label`，子元素会绘制在它们之上。

## 下一步

- [图片与图标](images.md)：画布可以绘制的图片。
- [动画](animation.md)：画布绑定的值。
- [无障碍](accessibility.md)：画布暴露的部件。
