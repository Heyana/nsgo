---
title: テスト
description: ヘッドレスまたはネイティブウィンドウで、ユーザーと同じ操作でアプリをテストする。
group: ツールチェーン
order: 21
---

## テスト

`nsgotest` はウィンドウを開かずにアプリを起動します。テストではテキストやロールによってコントロールを見つけ、ユーザーと同じように操作します：

```go
func TestMain(m *testing.M) { nsgotest.Main(m) }

func TestPress(t *testing.T) {
	app := nsgotest.Start(t, Root)
	app.Press("text:Press me")
	app.Find("text:Pressed 1 times")
}
```

`TestMain` は、ターゲット上でテストを実行する際にプラットフォームへメインスレッドを渡します。

パッケージでプロセス全体のセットアップとティアダウンが必要な場合は、`MainFunc` の中で `m.Run` の前後にそれらを記述します。`defer` されたクリーンアップはテストプロセスが終了する前に実行されます：

```go
func TestMain(m *testing.M) {
	nsgotest.MainFunc(func() int {
		setup()
		defer teardown()
		return m.Run()
	})
}
```

`Main(m)` は `MainFunc(m.Run)` の 1 行形式です。ターゲットでは各テストが独自のプロセスで実行されるため、このセットアップとティアダウンはテストプロセスごとに 1 回実行されます。


関数で起動するアプリ `nsgo.Run(func(app *core.Scope) {...})` は、`nsgotest.StartApp(t, fn)` でテストします。これは `Run` と同様にアプリ自身のスコープ内でその関数を 1 度実行するため、アプリケーションメニューや終了ハンドラがそのまま維持されます。デスクトップの `nsgotest.Environment` を指定すると、`nsgo.Run` がウィンドウを開く場所（macOS、およびアプリの関数に対する Windows と Linux）においてアプリのメインビューがメインウィンドウに表示されます。そこでは `desktop.CurrentWindow` と `OnClosing` が動作します。`app.CloseWindow()` はウィンドウの閉じるボタンのリクエストを送信してウィンドウが閉じたかどうかを返し、ハンドラ単体はハンドラを呼び出して `WindowClosing.Canceled()`（または `OnQuitRequest` に対して `QuitRequest.Canceled()`）を読むことでテストします。`app.RequestQuit(false)` はユーザーによる終了を送信し、`app.RequestQuit(true)` はログアウトなどの強制終了を送信します。どちらもヘッドレスおよび macOS、Windows、Linux で動作し、承認されたリクエストを `app.Quits` にカウントします。`app` 上のステップはメインウィンドウに対して実行されます。`app.Opened` はアプリが開いたウィンドウ、パネル、トレイビューを要求された通りに列挙し（`Transparent` や `SkipTaskbar` も含まれます）、`app.Window` はメインウィンドウのリクエストを `nsgotest.WindowRequest` 値として保持します。サイズ制限を確認するには `nsgotest.WindowSetConstraints` とリクエストの `Constraints` を使用します。ヘッドレスでは、`app.Shares` が共有リクエストをファイル、テキスト、アンカー付きの `nsgotest.Share` 値として記録します。`app.DropFiles` はドロップゾーン内の任意の要素を指定でき、それを囲む最も近い `OnFileDrop` を持つビューがファイルを受け取ります。`app.OpenLinks("myapp://settings")` はヘッドレステストおよび iOS と Android ターゲット上で、アプリ独自のリンクを `desktop.OnOpenLinks` または `mobile.OnOpenLinks` に配信します。`app.ReceiveShare(mobile.SharedItem{Text: "https://example.com", MIME: "text/plain"})` はヘッドレステストおよび iOS と Android ターゲット上で、`mobile.OnShared` に共有データを配信します。ビューの `Bounds()` は `Ref.Bounds` が報告するのと同様に、ウィンドウのポイント単位でのフレームを返します。画像ビューの `Image.Source` はそのビューが表示しているソースであるため、`app.Find("id:video").Image.Source.Version` とその `Pixels.Rect` によって、どのピクセルフレームがどのサイズで到達したかを確認できます。ヘッドレスでは、`app.Press` はターゲット上と同様にシステムスイッチ（`ui.SwitchSystem`）を切り替え、`app.Emit("id:switch", "change", true)` は値を直接報告します。`app.Navigate` と `app.PostMessage` は、マウント済みだが非表示状態の Web ビューが読み込みを続けている間でもそのビューに到達します。`View.Web` は Web ビューのソース、設定、ナビゲーションポリシーの `nsgotest.WebConfig` スナップショットを保持します。`View.WebCommands` はアプリが Web ビューに送信したコマンドを記録します。その `Command` を `nsgotest.WebLoad` や `nsgotest.WebSendInput` などの値と照合して確認できます。

マウントされた画像、入力、モーダル、画面、スクロール、動画、ピクチャ・イン・ピクチャの各設定には `nsgotest` の設定レコードが使われます。`View.Sticky` は `nsgotest.StickyCell` 値を保持します。画像バリアントマップとスクロールリストは、モデルがバッチを受け取った際にコピーされます。

開いているウィンドウ、パネル、トレイビューをメインウィンドウを先頭に開いた順で取得するには `app.Surfaces()` を使用します。`app.Surface("title:Settings")`、`app.Surface("id:settings-root")`、`app.Surface("index:1")` は、それぞれ現在のタイトル、ルートビューのテスト ID、またはリスト内の位置でサーフェスを 1 つ選択します。セレクタは開いているサーフェスと正確に 1 つだけ一致する必要があります。

macOS、Windows、Linux ターゲットでは、`app.Surface("id:palette").WindowState()` は安定（settle）した後に UI スレッド上で実際のネイティブウィンドウの `Frame`、`Visible`、`Focused` を読み取ります。パネルと元のウィンドウを比較することで、`NoActivate` パネルの表示時にキーボードフォーカスが元の位置に留まったかを確認できます。Wayland などネイティブ位置が取得できない場合、`Frame.Move` は false になります。ヘッドレステストやその他のターゲットはこのステップをスキップし、アプリが要求した位置やフォーカスで代用することはありません。

サーフェスには `Exists`、`Texts`、`Enabled`、`Clear`、`Focus`、`Hover`、`RightClick`、`DropFiles`、`Resize`、`Settle` などの `App` のビュークエリ、入力・ジェスチャーステップ、メディアレポート、キャプチャ、リサイズ、待機メソッドが備わっています。これらはそのウィンドウを選択して同じ `App` の振る舞いを再利用し、待機やステップはアプリ全体を安定させます。`Post` は元のアプリ経由でグラフ処理をキューに積み、ディスパッチ前に選択されたサーフェスがまだ開いているかを確認します。アプリ全体のクロック、サービスフィクスチャ、メニュー、アラート応答は引き続き元の `App` 上で管理してください。

`Config` はオープン時のオプションを `nsgotest.SurfaceConfig` として返します（`app.Opened` にも記録されます）。`Screenshot`、`CapturePNG`、`Capture` はネイティブターゲット上でそのサーフェスをキャプチャします。`Close` は `OnClosing` を通じてそのウィンドウを閉じるよう要求し、閉じたかどうかを返します。閉じられたハンドルに対するクエリ、ステップ、キューされた `Post` は失敗し、サーフェスリストから除外されます。親ウィンドウを閉じると子ウィンドウも閉じられます。各ステップの通常のターゲットサポートに従い、ヘッドレスおよび macOS、Windows、Linux のテストホストで同じステップが動作します。単一サーフェスターゲットではメインサーフェスのみが列挙されます。

`app.Menu`、`app.DockMenu`、`app.PopupMenu` は、ラベル、ロール、チェック状態、ネストされた項目を含む `nsgotest.MenuItem` スナップショットを公開します。項目を呼び出すには、対応する選択ステップにラベルのパスを渡してください。

`app.ClickTray("index:0", core.MouseButtonLeft)` や `app.ClickTray("title:Tools", core.MouseButtonRight)` は、ヘッドレスデスクトップテストにおいて稼働中のトレイアイコン上のクリックを報告します。`ClickTrayIndex` と `ClickTrayTitle` は簡易形式です。プラットフォームが報告するアイコン矩形をモデル化する必要がある場合は、ボタン引数の後ろに `core.Bounds` を 1 つ渡してください。指定しない場合、`Tray.ClickBounds` は利用できません。その後、トレイメニューは `ChoosePopup` で操作できるようになります。

`Drag` は、ポインタが押下位置から 8 ポイントを超えて移動した時点でその下のプレスをキャンセルします。同じルールは Web にも適用されるため、元のビューに戻るドラッグでは `OnPress` は発火しません。`app.DragTo("id:source", "id:target")` は通常のタッチレスポンダーを通じてビューの中央間をドラッグします。選択したサーフェスにも `DragTo` があり、TSX テストでは `await app.dragTo("id:source", "id:target")` を使用します。どちらのビューも表示状態に保ってください。このステップはドラッグ中にスクロールしたりドロップハンドラを直接呼び出したりしません。[ドラッグ＆ドロップ](drag-drop.md)を参照してください。

`app.KeyCommands()` は、現在のフォーカスと表示されているビューで発火可能なタイトル付きキーボードショートカットを一覧表示します。ヘッドレスおよびターゲット上でキー、タイトル、グループを報告します。

`app.PressWith("id:row", core.Modifiers{Shift: true})` は 1 回のプレス中修飾キーを押し続けます。`PressAtWith(selector, core.Point{X: 20, Y: 10}, mods)` はビュー内の指定した点を押します。`Shift`、`Control`、`Alt`、`Platform`（Apple では Command、それ以外では Windows/Super キー）は、ヘッドレスおよびネイティブターゲット入力を通じて `OnPressDown`、`OnPressUp`、`OnPress` に届きます。ただしネイティブプラットフォームの規約は引き続き適用されます（macOS の Chrome では Control+クリックがコンテキストメニューになります）。後続の `Press` や `PressAt` には修飾キーは付きません。watchOS にはキーボード修飾キー入力がないため、修飾キー付きのプレスは拒否されます。

ターゲットステップは、ビューポートの端を覆うネイティブバーの上へスクロールされたコントロールをネストされたビューポートとスクロール軸を考慮して表示させます。`PressAt` は指定されたビューのビューポート内のポイントを使用します。ポインタのターゲティング、可視性、スクロール制限は、ヘッドレスおよびターゲットモデルの両方で、ネストされたスケールや小数スケールを含むスクロールコンテンツのズームを考慮します。ビューの `Bounds()` は `Ref.Bounds` と同様に、変換前のレイアウトフレームを保持します。

アプリがポインタをロックした後、`app.PointerDelta(dx, dy)` はロックされたビューに相対移動量を送信します。ヘッドレステストでは `nsgotest.Environment(core.Environment{CanLockPointer: true})` で有効化します。Escape キーはヘッドレスとターゲットテストのどちらでもロックを解除します。

プレスではテキストフィールドを直接指定できます。ヘッドレスおよびすべてのターゲットで、プレスの直後に `app.Find("id:field").Focused` を読み取ることができます（押されたビューのハンドラがフィールドにフォーカスを移した場合も含みます）。

`app.SelectTab("id:tabs", 1)` はマウントされた `mobile.TabBar` の 2 番目の項目を選択します。これは iOS または Android の `Environment` を指定したヘッドレス環境で動作し、iOS および Android ターゲット上ではネイティブのタブバーを使用します。インデックスは 0 から始まり、既存の項目と一致する必要があります。電話のナビゲーションにはこれを使用してください。`Emit` はモデルイベントを報告するもので、ヘッドレスでのみ動作します。

`app.Focus(selector)` はタップと同様にフィールドにフォーカスを当て、デスクトップや Web の `Environment` またはそれらのターゲット上では Tab キーを通じて pressable または `Focusable` なビューにもフォーカスを当てるため、続いて `app.Key("Return")` でそのビューを押すことができます。アプリが `Ref.Focus()` を呼び出すと、Android、iPhone、iPad でもソフトウェアキーボードを表示せずに有効な pressable または `Focusable` ビューにフォーカスを当てます。ネイティブモバイルテストでは `app.Post(ref.Focus)` を使ってフォーカスを待機してください。モバイルターゲットではフォーカスアサーションはネイティブのフォーカス報告に従うため、拒否されたリクエストがヘッドレスモデルをすり抜けてパスすることはありません。デスクトップおよび Web ターゲット上の通常のビューに対しては、`app.Focus` ステップは引き続き Tab を使用します。プラットフォーム上と同様に、pressable なビューは `OnPress`、`OnPressDown`、`OnPressUp`、`OnLongPress` のいずれかを持ちます。その上で Return または Space を押すと、それがハンドルしているダウン、アップ、プレスイベントが送信されるため、`OnLongPress` のみを持つビューはフォーカスは受け取りますがキーで押されることはありません。

ヘッドレス実行では、プロジェクトのターゲットに関わらずデフォルトで phone イディオムが使用されるため、`cx.Env().Desktop()` は false になりホバーツールチップは表示されません。環境の他の部分（`Environment` オプションのものを含む）を保ったままデスクトップ表示をテストするには `nsgotest.Idiom(core.IdiomDesktop)` を渡してください。キーボードフォーカスはイディオムではなく、環境の OS に従います。

macOS ターゲットテストでは、テストウィンドウは AppKit の画面サイズ制約を無視するため、`nsgotest.Size` と `Resize` は表示画面より背が高くても要求されたコンテンツサイズをそのまま維持します。iOS と tvOS では、`app.Resize(width, height)` はテストホストの `UIWindow` をリサイズし、ネイティブレイアウトを待ちます。各テストはそのシーンのサイズで開始します。これはさまざまなビューポートサイズでのコンテンツを確認するためのものであり、Split View、Slide Over、Stage Manager のウィンドウ配置は行いません。ライブオートメーションは実行中シーンのサイズに従うため `Resize` を拒否します。

実ターゲット上では、`nsgotest.Environment` は外観、ロケール、テキストサイズ、視差効果を減らす設定、コントラストを上げる設定、タイミング付き触覚サポート、サーマルステート、低電力モード、ログイン起動理由、前面のアプリとプラットフォームがそれに追従するかどうか、およびデバイスの名前とモデルを固定します。`DisplayID` とともに `Displays` を設定すると、リフレッシュレートを含めてディスプレイ一覧とウィンドウの現在のディスプレイを固定できます。残りの項目（OS、イディオム、セーフエリア、システムヘッダーの高さ）はターゲット自身の値が使われます。したがって、`core.OSWindows` を指定した `Environment` でも `go tool nsgo test -target macos` が Windows のようにアプリをレイアウトすることはありません。そのようなテストは `Environment` 全体が適用されるヘッドレスで実行するか、`-target windows` で実行してください。同様に `nsgotest.Idiom` もヘッドレスでのみ適用されます。`nsgotest.Size` は macOS、Windows、Linux、Web でテストウィンドウのサイズを設定し、Android、iOS、tvOS、watchOS ではデバイスの画面サイズが維持されます。`-target macos` では、`Environment` を指定しなくてもアプリは `nsgo.Run` と同じようにウィンドウを開きます。ルートビューまたは `SetMainView` が設定したビューがメインウィンドウに表示され、`desktop.CurrentWindow` と `OnClosing` が機能します。`nsgotest.Size` でサイズ指定しない限り 480 × 640 ポイントで開き、ヘッドレス時と同様にステップがそのウィンドウに作用します。閉じるボタンまたは Close 項目のショートカットで閉じると `OnClosing` ハンドラに問い合わせます。閉じられた後は操作できるビューは残らず、最後のウィンドウが閉じる際にアプリが要求する終了は、テストを終了させる代わりに `app.Quits` にカウントされます。

`app.Key("mod+a")` はフォーカスされたフィールドのテキストを選択し、`mod+c`、`mod+x`、`mod+v` はバインディングがそのキーを消費せず、かつアプリケーションメニュー項目がそのショートカットを持たない場合に `app.Clipboard` を使ってコピー、切り取り、貼り付けを行います。macOS、Windows、Linux の `Environment` では、どのバインディング、フォーカスされたビュー、フィールドも消費しないキーは、それをショートカットとして持つ有効なアプリケーションメニュー項目を選択します。そのため `app.Key("mod+r")` でメニューのショートカットがその項目に届くかをテストできます。無効化された項目は何も消費せず、フィールドにフォーカスがある間は `NotInText` の項目は何も消費せずに後続項目へのキーの受け渡しを遮りません。Linux では Control、Alt、ロゴキーのいずれかを伴わないキーはどの項目も消費しません。ヘッドレスでは、標準ロールを持ち `OnSelect` を持たない項目が `app.ChooseMenu` またはショートカットで選択された場合、デスクトップと同様の動作を行います。Close は `OnClosing` ハンドラを通じてメインウィンドウを閉じ、Quit は `OnQuitRequest` ハンドラに問い合わせて承認されたリクエストを `app.Quits` にカウントし、Minimize、Zoom、Toggle Full Screen は環境が報告するメインウィンドウの状態を変更します（アプリの他のウィンドウはそのまま残り、`app.SetWindowEnvironment` で個別のウィンドウを変更できます）。編集ロールはフォーカスされたフィールドに作用します。それ以外のロール、およびフィールド内での Undo や Redo は、キーを飲み込むのではなくステップを失敗させます。`-target` 指定時はそのような項目があるとテストをスキップします。ヘッドレス、デスクトップターゲット、Web では、`KeyAlways` バインディングはフィールドが動作する前に Return、Backspace、すべて選択を受け取り、`KeyAuto` と `KeyNotEditing` はそれらをフォーカスされたフィールドに委ねます。Android と iOS のテストホストでは、Backspace とすべて選択を最初に `KeyAlways` バインディングに渡します。Android の Control ショートカットはその後ネイティブ編集に届き、iOS は未バインドのキーをソフトウェアキーボードにルーティングします。iOS のソフトウェア Return はまだキーマップへ先にキーを渡しません（[#1351](https://github.com/nsgo-dev/nsgo/issues/1351)）。フォーカスされたビューの `OnKeyDown` は、修飾キー単独（`app.Key("shift")`）を含め `app.Key` を最初に受け取り、そこで消費されたキーはヘッドレス、デスクトップ、Web ターゲットにおいてバインディングに届きません。`app.Key` は完全なプレスであるため、その後にビューの `OnKeyUp` がキーが離されたことを受け取ります。`app.KeyDown` と `app.KeyUp` は 1 つのキーを個別に押して離すものであり、コード（同時押し）ではありません。`KeyDown` で押された修飾キーは `KeyUp` されるまで後続のすべてのキーに加わるため、`app.KeyDown("shift")` の後に `app.Key("a")` を実行すると Shift+A が押され、押されていないキーに対する `KeyUp` はステップを失敗させます。テスト終了時点でまだ押されたままのキーはその時点で解放され（`-target` ではプラットフォームを通じて行われます）、押下を受け取ったビューが解放を受け取り、次のテストに押しっぱなしのまま持ち越されることはありません。`core.KeysDown` と `KeyHeld` は現在押されているキーを読み取ります。`-target` 指定時、watchOS はこれらを含むテストをスキップします。`app.MouseDown`、`app.MouseDownAt`、`app.MouseDownWith`、`app.MouseDownAtWith` およびそれらの `MouseUp` 版は、5 つのマウスボタン（`core.MouseButtonLeft` から `core.MouseButtonForward`）のいずれかを押して離し、`core.MouseButtonsDown` と `core.MouseButtonHeld` を更新します。`app.BlurWindow()` はウィンドウのフォーカス喪失をシミュレートし、押されたままのすべてのキーとマウスボタンをクリアします。

`KeyDown` と `KeyUp` は Android のネイティブハードウェアキー経路と HarmonyOS の Go キーブリッジも駆動します。Android および明示的にフォーカスされた iPhone/iPad のビューでは、`OnKeyDown` はバインディングやネイティブのアクティベーションより前に実行されます。消費されたダウンイベントでも引き続き `OnKeyUp` を受け取り、リリースは元のビューに留まり、`core.KeysDown` はネイティブエディタの外で押されたままのキーを追跡します。Android の非同期の消費応答により UI は応答性を保ちます。フォーカス喪失、ティアダウン、または 2 秒経っても応答がない場合は、ネイティブアクションを再実行することなく保留中のキーをクリアします。HarmonyOS は報告されたキーを追跡し Go バインディングを抑制できますが、そのホストにはまだネイティブのプレーンビューフォーカスがなく、非同期ブリッジから戻った後に ArkUI アクションを抑制することはできません。

`app.KeyCode("KeyW")` は物理的な位置を押し、`KeyCodeDown` と `KeyCodeUp` はそれを個別に押して離します。ヘッドレステストでは、`map[string]string{"KeyW": "z"}` のように `core.Environment.KeyCodeNames` を設定して別のレイアウトを試すことができます。未指定の位置には US レイアウトが使われます。`-target` ではプラットフォーム自身のキーイベントがスキャンコードやキーコードなどの位置情報を運ぶため、右側の修飾キーやテンキーの数字は左側や数字列と区別して読み取られます。macOS、Windows、Linux、Web ではプラットフォームの現在のキーボードレイアウトがそのキー名を決定するため、AZERTY 配列では `KeyW` は `z` になります。iOS、Android、HarmonyOS は US レイアウトでその位置にあるキーを報告します。`KeyDown` と `KeyCodeDown` の両方で押された修飾キーは、両方が解放するまで押されたままになります。watchOS など物理位置を押せないターゲットではテストをスキップします。

`-target` では、`app.Select(selector, start, end)` がネイティブフィールドの選択範囲を移動します。オフセットは UTF-16 単位でテキスト長にクリップされ、逆順の端点は昇順に並べ替えられます。iOS と Android はそれぞれのネイティブテキスト入力 API を使用し、HarmonyOS はフィールドのコントローラを使ってその選択コールバックを待ちます。`OnSelectionChange` は通常通り実行され、テストモデルはそのコールバックがなくても実際の範囲を読み取ります。macOS、Windows、Linux、Web はすべて選択と編集用の矢印キーを使用するため、それらのキーを奪うバインディングやアプリケーションメニュー項目があるとそのステップからキーが奪われます。その後の入力やネイティブクリップボード編集は選択範囲に対して作用します。HarmonyOS のテスト入力は報告されたネイティブ範囲を置換しますが、そのテストホストはまだネイティブのクリップボードキーを駆動しません。

ヘッドレスの Linux および Android テストでは、インストーラに渡されたパッケージパスを `app.Installs` で確認できます。シミュレートされたインストーラからエラーを返すには `app.InstallPackage` を設定してください。

`app.DoublePress(selector)` は 1 ステップで 2 回プレスを送信し、プラットフォームがダブルクリックまたはダブルタップとしてカウントできるようにします。`app.Pan` はターゲットのドラッグ入力を使用し、各オフセットは開始点からの相対値になります。`app.Pinch` はヘッドレスで動作し、macOS、iOS シミュレータ、Android、Web 上のプラットフォームジェスチャー経路を駆動します。iOS シミュレータでは、各スケール値によって 2 本の指の距離が設定され、UIKit の認識しきい値を超えると報告が開始されます。Android では、まず 2 本の指がネイティブレコグナイザを起動するのに十分な距離まで広がり、その後要求されたスケールを通過します。ピンチ入力のないターゲットで `Pinch` に到達すると、テストの残りをスキップし、サポートが欠けているターゲットを報告します。

`app.Orientation` は有効な `Scope.RequestOrientation` マスクを保持します。アプリが通常の向きポリシーを使用しているときはゼロになります。

`app.NowPlaying()` はアプリが最後に公開したシステムメディア情報を返し、クリアされた後は nil を返します。`app.MediaCommand(core.MediaCommand{Kind: core.MediaSeek, Position: 30 * time.Second})` はその公開情報を通じてライブハンドラへコマンドを送信します。ヘッドレスでは `nsgotest.Environment(core.Environment{NowPlaying: true})` を使用するか、macOS ターゲット上で実行してください。

## ウィンドウなしの描画

macOS では、`nsgotest.PlatformTextEngine()` によりヘッドレステストに CoreText のメトリクスを提供できます。`nsgotest.TextEngine` を通じて渡してください。他の環境では nil を返し、モデルは近似メトリクスを維持します。アプリを起動する前に `nsgotest.RegisterFont` でフォントのバイト列を登録してください。

`nsgotest.Render(app.Find("id:score").Drawing, core.Size{Width: 600, Height: 800}, 2)` は、ウィンドウを開かずに記録されたキャンバスを所有された `*image.RGBA` にリプレイします。`image/png` でエンコードするかピクセルを比較してください。サイズはポイント単位、スケールはポイントあたりのピクセル数であり、透過出力は最大 16 Mi ピクセルに制限されます。どちらも有限かつ正の値でなければなりません。画像は 30 秒のデッドラインで同期的に読み込まれ、アニメーション画像は最初のフレームが使われ、失敗時はエラーを返します。レンダリングの前にアニメーションスロットとビューペイントを解決してください。

ウィンドウなしのレンダリングとフォント登録は現在 macOS をサポートしています。他のターゲットでは `nsgotest.ErrNoPlayer` を返します。ネイティブターゲットの `Screenshot` はそれとは独立して利用できます。

`View.Style` は `nsgotest.ViewStyle` を記録し、`View.Animated` は `nsgotest.AnimatedValues` を記録します。`app.Animated(selector)` はバインドされた値を記録済みのベーススタイルに適用します。エフェクトやカスタムカーソルはアプリ側の値の同一性を保ち、`Radii`、`Rounded`、`Rim`、`Matrix` は同じジオメトリポリシーを維持します。

`app.Find(selector).Kind` は `nsgotest.ViewKind` です。マウントされたビューの種類をテストで検査する必要がある場合は、`nsgotest.KindText`、`KindInput`、`KindCanvas`、`KindGPU` などの定数と比較してください。

ヘッドレス GPU テストでは、`app.Surface("title:Inspector").GPUReadPixels("id:viewport")` を使ってウィンドウのソフトウェア GPU ピクセルを取得できます。`Surface.TriggerGPUDeviceLost` と `Surface.RestoreGPUDevice` は、そのウィンドウのピクセル比を用いてそのウィンドウ内でのデバイスロストと復旧をシミュレートします。ウィンドウ間でビュー ID が重複してもデバイスとリードバックコールバックは分離されたまま保たれ、`app.GPUFrames()` はアプリ全体のサブミッションを記録します。これらのヘルパーはヘッドレスのソフトウェア GPU モデル上で動作します。

`nsgotest.UpdaterState(core.UpdaterState{...})` は、ヘッドレスおよびネイティブターゲット上で最初のレンダリング前にアップデータの機能と設定をシードします。各チェック結果を提供するには `app.Update` を設定します。設定の変更は更新フィードへ接続することなくモデル化されます。

ヘッドレステストでは `app.EditMenu` と `app.EditMenuItems()` を検査し、`app.ChooseEditAction(core.EditCopy)` で提示されたアクションを選択し、`app.CloseEditMenu()` で閉じます。貼り付けにはテストのクリップボードが使われます。`app.LongPressAt(selector, point)` は特定の点を長押しし、ヘッドレスの `app.Touch(selector, point).Hold(duration)` は指を離す前にロングプレスを発火させるため、続く `Move` で選択スイープを試すことができます。メニューレコードとアクション選択は挙動をモデル化したものであり、システムツールバーを検証するにはネイティブの受け入れテストを使用してください。

`View.Props` は識別子、アクセシブル名と状態、アクション、フォーカスおよびポインタ設定の `nsgotest.ViewProps` スナップショットです。`HelpText`、`HasAction`、`ActionList`、`FocusStop` がそのポリシーを公開し、ラベル ID、アクション、リップル設定はマウントされた値からコピーされます。

## ウィジェットとライブアクティビティ

`nsgotest.Widgets(true)` はアプリが `widget.Update` で公開した内容を記録します。各 kind の最新タイムラインは `app.Widgets()` で読み取ります。日付のないエントリにはアプリのウォールタイムが使われます。`widget.Available()` を false にして更新が `widget.ErrNoWidgets` を返すようにするには `nsgotest.Widgets(false)` を使用します。

`nsgotest.LiveActivities(true)` は開始、更新、アラート、終了を記録します。`app.LiveActivities()` は開始順にアクティビティを返し、そのコンテンツ変更と最終コンテンツを含みます。開始時に `widget.ErrActivitiesDisabled` を返すようにするには `nsgotest.LiveActivities(false)` を使用します。これらのオプションは、ネイティブターゲットを含めテスト中に対応するプロセス全体のサービスを置き換えます。これらのサービスを呼び出すテストは順次実行してください。

ヘッドレステストでは `app.ConnectGamepad(id, name)`、`app.SetGamepadInfo(id, family, vendorID, productID, canRumble)`、`app.DisconnectGamepad(id)`、`app.SetGamepadButton(id, btn, value)`、`app.SetGamepadAxis(id, axis, value)` でゲームコントローラをシミュレートできます。接続、情報更新、切断は安定して `gamepad.Pads` を読むスコープを再レンダリングし、ボタンと軸の更新は `gamepad.State` 向けに次のフレームで反映されます。`app.GamepadRumbles()` と `app.LastGamepadRumble(id)` は記録された `gamepad.RumbleEvent` の開始および停止コマンドを検査します。`-target` ではこれらはテストをスキップします。

`app.FindMounted("id:tab-bar").TabBadge(index)` は、ヘッドレスおよびターゲットテストでネイティブタブのテキストバッジまたはドットバッジを返します。描画された `ui.TabShell` タブでは、`Props.Value.Text` がバッジとして通知される値になります。

## テストを実行する

```sh
go tool nsgo test
```

`test` はテストごとに 1 行出力します。`-run` は `go test` と同様に `-run 'TestGallery/dark-home'` のようなサブテストを含めてテストを選択します。ターゲット上では、`-timeout` でテストごとのデッドライン（デフォルト 3 分、`0` で無効化）を設定でき、そのデッドラインに達すると結果に報告されます。シミュレータや実機では、`TestMain` が `nsgotest.Main` を呼び出すテストのデッドラインはテストホスト起動時からカウントされるため、アプリの起動が遅くても消費されません。起動自体にも 5 分という独自の制限があり、それに達すると起動タイムアウトとしてテストが失敗します。サブテストがすべてスキップされたテストはスキップ扱いになります。`-junit report.xml` は各スキップ理由を保持した JUnit レポートを出力し、`-trace` は各ステップのマウント項目とコミット時間を出力し、`-bench` は代わりにベンチマークを実行して結果ごとに 1 行出力します。ヘッドレス実行では `--` 以降のフラグが `go test` に渡され、ターゲット実行では以下のランタイムフラグを受け付けます。コンパイルエラーは、`-- -c` でヘッドレス実行のテストをコンパイルのみ行う場合であってもコンソールと JUnit レポートに出力されます。

ライブラリやモジュールのルートなど `nsgo.yaml` のないディレクトリを指定した場合、`test` はその配下のすべてのパッケージを対象とします。ディレクトリは Go のパターンと同様に `/...` で終えることもできます。ディレクトリは 1 つだけ指定でき、2 つ目を指定するとエラーで停止します。複数をテストするには、それらの上位ディレクトリを指定するか、それぞれに対して個別に実行してください。`go tool nsgo check` も同様にパッケージを受け取り vet を実行します。

テストはリアクティブグラフの所有権をチェックします。シグナルとスコープにはアプリのゴルーチン上でアクセスし、ワーカーからは `cx.Post` を使用してください。アプリがアイドル状態のときに別のゴルーチンからの呼び出しが到達すると `Scope.Post` 診断メッセージとともにパニックし、それ以外のケースは `-race` が検出します。内部エンジンのパニックは引き続きテストを失敗させ、出荷済みアプリでは影響を受けたエンジンを停止してホストへスタックを報告します。開発ビルドでも所有権はチェックされますが、リリースビルドではチェックが省略されます。WebAssembly の開発ビルドとテストビルドでは、ブラウザのコールバックがブラウザ管理のゴルーチン上で再開される場合があるためゴルーチン ID のアサーションは省略されますが、その他のテストチェックはそこでも実行されます。

`app.Post(func() { state.Set(value) })` は、次のステップまたは待機が安定（settle）する際にアプリのグラフゴルーチン（ヘッドレスではテストゴルーチン、ターゲットではエンジンゴルーチン）上で実行される処理をキューに積みます。テストやスタブされたワーカーからこれを呼び出し、その後 `app.Settle()` または待機メソッドを使って結果のビューを読み取ってください。渡した関数からさらに `Post` することもでき、同じステップ内で実行されます。ターゲット上ではテストゴルーチン外で実行されるため、この関数内でステップを実行したり `t.Fatal` を呼び出したりしないでください。

ヘッドレスでは `cx.ReadAsset` はプロジェクトの `resources/` ディレクトリを読み取ります。`-target` ではテスト用にパッケージ化されたリソースを読み取ります。

各ステップはその後、終了するアニメーションがすべて終わり、各スタイル遷移が `OnTransitionEnd` を実行し終えるまで、最大 2 秒間 1 フレームずつクロックを進めます（ターゲット上ではプラットフォームがアニメーションを実行するため遷移のみが対象です）。`EndingStyle` を通じて閉じるダイアログやポップオーバーは、`app.Press` が戻る時点ではすでに消えています。途中の状態を確認するには `nsgotest.ManualFrames()` を指定して起動してください。そうすると `app.Advance` がクロックを進めた分だけ動作するようになります。

## 時間

ヘッドレスおよびターゲット上では、アプリは 2026-01-01 00:00 UTC から始まりテストが進めた分だけ進む `core.ManualClock` である `app.Clock` 上で動作します。`app.Advance(d)` は時間を `d` だけ進め、期限に達した各タイマーを実行し、アプリがタイマーを待っている間は実際のディスプレイと同様にフレーム間隔ごとにディスプレイフレームを実行します。`app.FastForward(d)` はラップトップをしばらく閉じてから開いたときのように一気に先へ進みます。途中で期限に達した各タイマーは末尾で 1 度だけ実行され、ディスプレイフレームも 1 度だけ実行されます。ヘッドレステストにおけるシミュレートされたディスプレイレートでは、コールバックはサーフェスの `SetPreferredFrameRate` 設定にも従います。

アプリはこのクロックを `cx.Now()` で読み取り、表示する日時を `cx.WallTime()` で読み取るため、ジェスチャーの速度やダブルタップの判定時間などアプリ自身が計測する時間にはこれらを使用してください。`time.Now()` と `time.Sleep` はシステムクロックに従い `Advance` では進まないため、`time.Now()` で計測するアプリはテストが進めた時間を見落とし、テスト内の `time.Sleep` はテストを遅くするだけです。返信後の `cx.Post` など別のゴルーチンから届く処理は、実時間でタイムアウトを計測する `app.WaitFor`、`app.WaitForGone`、`app.WaitUntil` で待機してください。

ヘッドレスでは、これらの待機メソッドはチェックの合間に 1 フレームずつクロックを進めます。待機中にクロックを止めておくには `nsgotest.ManualFrames()` を指定して起動してください。ターゲット上では、プラットフォームが独自のアニメーションを実行する間に待機メソッドがアプリを安定させ、ステップと同様にスタイル遷移が `OnTransitionEnd` を待つ間だけチェックの合間にクロックを進めます。

今日が期限の項目など日付に依存する画面は、`nsgotest.WallTime(t0)` を指定して起動します。最初のレンダリングの `cx.WallTime()` はそのタイムゾーンにおける `t0` となり、タイマーは引き続きクロック自身の時間で動作します。`nsgotest.Clock(c)` はテスト側が用意したクロック上でアプリを実行するため、アプリ外のコードと時間を共有でき、`c.Advance` が進めるにつれてアプリのタイマーが発火します。`nsgotest.Measure()` で開始したターゲット上の計測では、後述の通り代わりにシステムクロック上でアプリが実行されます。

ステップが用意されていないヘッドレスのプラットフォーム報告を行うには、`app.Report(nsgotest.Event{Kind: nsgotest.EventPress, ID: view.ID})` を呼び出してください。この報告はスライス、ナビゲーション状態、および任意の `nsgotest.TouchGeometry` スナップショットをコピーし、`Data` はテストのネイティブペイロードを保持します。

## プラットフォーム上で実行する

テストはプラットフォーム自身の入力を使って実際のウィンドウ内で実行されます。テストの検出は、通常はポータブルなパッケージ内のターゲット専用テストファイルを含め、ターゲットのビルドタグに従います：

```sh
go tool nsgo test -target macos
```

macOS では、テストウィンドウはテストホスト外からのマウス、キーボード、トラックパッド入力を無視するため、テストスイートの実行中にマシンを操作してもテストのシーンが変わることはありません。テスト自身のステップは通常通り動作します。

テストウィンドウはアプリをアクティブ化せずにテストのキー入力を受け取るため、作業中のアプリが前面に留まります。他のアプリがアクティブな間、AppKit がフィールドへのクリックを拒否することがあり、`Focus` と `Type` はその拒否を診断して報告します。`go tool nsgo test -target macos -activate` を使うと、テストアプリがフィールドをクリックしたりキーを押したりする前にアクティブ化してキーボードを取得できるようになります。これにはロック解除されたデスクトップセッションと、`github.com/nsgo-dev/nsgo` をインポートし（ブランクインポートで十分です）`TestMain` から `nsgotest.Main` または `nsgotest.MainFunc` を起動するテストが必要です。

クリーンアップは開いたまま残されたネイティブのコンテキストメニューを閉じるため、次のサブテストの最初のクリックやキーが奪われることはありません。macOS では、WindowServer のキャプチャにはウィンドウの境界でクリップされたテストウィンドウの開いているネイティブポップアップが含まれ、他のウィンドウは除外されます。

ネイティブの `WebView` 内部での信頼されたイベントには、`Env.CanSendInput` を確認し、アプリのグラフ上で `app.Post(func() { page.SendInput(core.WebClick(12, 16)) })` のように入力を送信します。クリックする前にページが準備完了を通知するのを待ち、`core.WebText` で入力する前にフォーカスを当ててください。Linux では X11 と XTest が必要です。そのテストホストは物理入力を引き続き拒否しつつ、これらのスクリプト化されたイベントを受け付けます。

ターゲットには `macos`、`windows`、`linux`、`web`（Chrome、Chromium、または Edge 内）、`ios`（接続された iPhone 上。複数ある場合は `-device` で指定し、Xcode が最後に使用したチームまたは `-team` のチームで署名）、`ios_simulator`、`tvos_simulator`（リモコンの矢印キー、選択ボタンとしての Return、Menu としての Escape）、`watchos_simulator`、`android`、および `all` があります。各テストは独自のプロセスで実行されます。`app.Capture()` は現在のピクセルを `image.Image` として返し、`app.CapturePNG()` は PNG バイト列を返します（どちらもヘッドレス時はテストをスキップします）。macOS では、これらは Core Animation のシャドウ、マスク、フィルタ、アニメーション、キャレットを含め WindowServer が合成したウィンドウそのものになります。最小化されている場合や別の操作スペースにある場合などウィンドウをキャプチャできないときは、代わりに AppKit を通じてビュー自身が描画され（上記の効果は含まれません）、初回のみその旨がログに出力されます。NSGo はエンコード前にどちらの macOS キャプチャ経路も sRGB に変換します。iOS と tvOS も sRGB でキャプチャするため、これらのターゲット上のピクセル値はディスプレイが Display P3 を使用しているかどうかに依存しません。Android はシェーダーサーフェスを含めレンダリングされたウィンドウをキャプチャします。`app.Screenshot("en/library")` はスクリーンショットを `.nsgo/test/screenshots/<target>/en/library.png`（`macos` では `.nsgo/test/screenshots/macos/en/library.png`）として保存します。名前にはスペースを含められますが、絶対パスや `..` を含むことはできません。`-screenshots dir` を指定すると作業ディレクトリからの相対パスである `dir` 配下に保存されます。デバイスのログによって一部が欠落または途切れたスクリーンショットはファイルが書き込まれる前にテストを失敗させ、以前のテストが既に使用したパスのスクリーンショットもテストを失敗させます。`-fps` は `macos`、`windows`、`linux`、`ios_simulator` ではパフォーマンスモニターの計測値を 1 秒ごとにログ出力し、`web` と `android` ではエンジンの計測値のみを出力します。`-env KEY=VALUE` はテスト環境の変数を設定し、実機では他の方法で渡せない環境変数を参照できます。シミュレータや実機では、`nsgo.yaml` のないディレクトリのテストはディレクトリ名にちなんだアプリとしてインストールされ、ID は `dev.nsgo.` の後ろに名前の英数字を続けたもの（`base/formula` なら `dev.nsgo.formula`）になります。`android` とシミュレータでは、テストはアプリの ID の後ろに `.nsgotest` を付けた ID（App Clip のテストもその ID）で、インストール済みのアプリに上書きするのではなく並んでインストールされ、実行終了時にアンインストールされます。`ios` ではプロビジョニングプロファイルが名付けるアプリ自身の ID でインストールされるため、iPhone にインストール済みのビルドとそのデータを置き換えます。

iPhone 実機ターゲットテストでは、テストホスト起動後の Go パニック診断を含め、アプリの stderr が保持されます。

`android` と `ios` では、テストは NSGo のテストホスト内でのみ実行されます。Android のランタイムはテストプロセスの標準出力からプロトコルを読み取り、ツールはホストが出力するマークからテストの出力と終了コードを取得します。これらでテストを実行するパッケージは、テストファイルで `github.com/nsgo-dev/nsgo` をインポートし（ブランクインポートで十分です）、`TestMain` から `nsgotest.Main` または `nsgotest.MainFunc` を起動してホストをリンクする必要があります。ビルドやインストールの前に、`nsgo test` はホストがリンクされており `TestMain` が宣言されていることを確認します。その関数がホストを迂回した場合、Android は `nsgotest.Main` のヒントを添えて不正な stdout プロトコルを報告するため、ホストが起動するまでログとセットアップ出力は stderr に出してください。`ui/chat` のようなライブラリパッケージも、テストファイルに両方を追加すれば実機上で実行でき、そのままでもヘッドレスや他のターゲットでは実行できます。

プラットフォームバックエンドをリンクしないパッケージは、ターゲットのランタイム内でヘッドレス実行されます。`-target web` では、この方法でブラウザ内のライブラリコントロールをテストできます。ブラウザにはファイルシステムがないため、ストレージはメモリ内に留まり、`DataDir`、`CacheDir`、`LogsDir` は空になります。ファイルに裏打ちされたフェイクである `nsgotest.MicrophoneInput` は実際の WAV ファイルが必要なためそこではスキップされます。

結果を報告する前にパニックまたは終了した Android テストは、プロセスの終了ステータスとパニックスタックを含むキャプチャ済み出力を伴って失敗します。ホストを起動せずに終了した `TestMain` も、ステータス 0 であっても同様に失敗します。Android ターゲットテストがタイムアウトに達すると、結果にはホストが開始マークを出力したかどうかが記載され、ランナーがクリアする前のそのテストの `logcat` 出力の末尾も含まれます。

`web` では、テストはツールが見つけたヘッドレスの Chrome、Chromium、Edge、または `NSGO_BROWSER` が指定するブラウザ実行ファイルで実行されます。macOS では、Chrome for Testing がインストールされていればツールはそれを優先します。ツールはブラウザの起動を最大 30 秒待機し、コマンドがキャンセルされると起動待機を中止し、ブラウザが終了するか応答しない場合はブラウザのエラー出力を報告します。

各ステップと `app.Settle()` は、エンジンの保留中コミットが完了するのを待ってから、バッチや有限アニメーションのない状態で 100 ms をカウントします。待機全体はアプリが変化し続けていても 5 秒で停止します。時間がかかる最初のダイアログのためにテストへ余分な `Post` や `sleep` を入れる必要はありません。

Android では、ビルド前および各テスト前にツールがスリープ中の画面を起こし、認証情報のないロック画面を解除します。PIN、パスワード、パターンでロックされたデバイスは実行を続行する前にロック解除が必要であり、エラーにデバイス名が表示されます。一時的な `adb` 状態読み取りエラーはリトライされ、到達不能のままのデバイスは残りのすべてのテストを失敗させるのではなくターゲット実行を 1 回で停止します。

確認ではなく計測を行うには、`nsgotest.Measure()` で開始します。アプリはシステムクロック上で動作し、`Advance` は進めた時間だけ実際に待機し、ステップはアニメーションしているものが静止するまでのみ待機します。`app.Mark("name")` はそこから先の計測対象に名前を付け、`app.Mark("-")` で終了します。マークは `-fps` の有無に関わらず出力されます。

NSGo のフラグはディレクトリの後ろではなく前に置いてください（`go tool nsgo test -target web ./app`）。ヘッドレス実行では `--` 以降のフラグが `go test` に渡されます。ターゲット実行ではそこに `-short`、`-count`、`-cpu`、`-parallel`、`-failfast`、`-fullpath` のみを指定できます（例：`go tool nsgo test -target web ./app -- -short -count=3`）。それ以外の Go test フラグは明示的に拒否され、`-run` や `-timeout` はディレクトリの前に置く必要があります。`-count=0` ではテストを実行しません。NSGo のネイティブホストをリンクするテストは、count と CPU 設定の組み合わせごとに新しいプロセスで繰り返され、結果には `[run N]` が付き、スクリーンショットは `run-N` サブディレクトリに保存され、呼び出しごとにタイムアウトが適用されます。ライブラリは Go のプロセス内反復を維持します。

## メディアレポート

`app.Play(selector, core.VideoInfo{Duration: time.Second, RateChangeable: true})` は、再生時間、本来のサイズ、再生速度変更の可否を伴う動画の読み込みを報告します。速度コントロールが利用できないプレイヤーをテストするには `RateChangeable` を false に設定してください。

`tui` では、テストに対話型ターミナル、リンクされた NSGo バックエンド、および `nsgotest.Main` を呼び出す `TestMain` が必要です。各プロセスが 1 つのアプリケーションを開き、そのステップによってターミナルの編集、フォーカス、マウスおよびキールーティングを検証します。テスト出力はターミナルが復元されるまでバッファリングされます。PNG キャプチャはポータブルな固定書体でセルをレンダリングします。[ターミナルターゲット](building.md#terminal)

## TSX テスト

`nsgo/test` で書かれた TSX アプリの `*.test.tsx` ファイルは、Go のテストと一緒に実行されます。

`open("App.tsx", { clock: {} })` はマニュアルクロック上でアプリを実行し、`app.clock` で時間を進めます。`app.clock.runFor(ms)` は `app.wait(ms)` と同様に `Advance` のように 1 フレームずつ時間を進め、`app.clock.fastForward(ms)` は `FastForward` のように一気に先へ進みます。`clock: { time }` は `Date.now()` と `new Date()` が返す時刻を `time`（`Date` または 1970 年からのミリ秒）から開始し、`app.clock.setSystemTime(time)` は後からそれを設定します。どちらもタイマーは進めません。`clock` を指定しない場合でも、ヘッドレスのアプリでは `waitFor` と `waitUntil` の待機中に実機と同様に 1 フレームずつ時間が経過します。

`open("App.tsx", { camera: true })` はテスト環境にカメラを提供するため、`Picker.canTakePhoto()` が `true` に解決され、`Picker.takePhoto` が `app.picks()` に（`camera: true` 付きで）記録されて `app.onPick` で応答できるようになります。撮影した写真として `PickFile` または `PickFile[]` を返すか、キャンセルとして `null` または `[]` を返すか、`Picker.takePhoto` を拒否（reject）させるためにエラーをスローしてください。`camera: true` がない場合、`Picker.canTakePhoto()` は `false` になり、`Picker.takePhoto()` は拒否されます。

テストファイルなしで TSX アプリをスクリプト実行するには、エントリーポイントと、`app` をそのアプリとして `*.test.tsx` のテスト本体のように書いたステップを標準入力から渡します：

```sh
go tool nsgo test App.tsx < steps.ts
```
