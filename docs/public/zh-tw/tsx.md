---
title: TSX 應用程式
description: 使用 TSX 與 Tailwind 類別撰寫應用程式，預先編譯而不需 Node。
group: 核心概念
order: 63
---

## TSX 應用程式

TSX 應用程式是一個內嵌原始碼並執行其進入點的 Go 套件：

```go
//go:embed *.tsx nsgo.yaml
var src embed.FS

var Root = compiler.App(src, "App.tsx")

func main() { nsgo.Run(Root) }
```

```tsx
import { mount, signal, Text, View } from "nsgo";

function App() {
  const count = signal(0);
  return (
    <View class="gap-3 p-6" onPress={() => count.update((n) => n + 1)}>
      <Text class="text-lg font-bold">Pressed {count()} times</Text>
    </View>
  );
}

mount(App);
```

`go tool nsgo init -template tsx` 可建立一個範本專案，而 `examples/tsx` 是另一個範例。`nsgo dev` 與 `nsgo build` 會預先編譯 TSX，因此套件發布時不包含編譯器；`go run` 與 `go test` 則會在應用程式啟動時進行編譯。無論哪種方式都不需要 Node。

## 元件與模組

TSX 會從 `nsgo` 匯入基礎原語、從 `nsgo/base` 匯入 `base` 的元件、從 `nsgo/gamepad` 匯入遊戲控制器（在已簽署的套件中需具備 `gamepad` 權限能力）、從 `nsgo/audio` 匯入音效（需具備 `audio` 權限能力），以及一旦應用程式匯入 `tsx/ui` 後，即可從 `nsgo/ui` 匯入 `ui` 的元件；`examples/gallery-tsx` 展示了這些元件。

若要使用 Mermaid 圖表，請在 Go 宿主中匯入 `github.com/nsgo-dev/nsgo/tsx/diagram` 以啟用其副作用（effect），並搭配 `tsx/textview` 或 `tsx/ui`。這會將 `Diagram` 加入 `nsgo/base`，而不會將圖表剖析器連結到未使用它的應用程式中。它接受 `source`、`fontSize`、部分 `theme`、`accessibilityLabel` 以及常見的樣式屬性。可直接單獨使用，或提供給 TextView 作為算繪器：

```tsx
import { Diagram, TextView } from "nsgo/base";

<TextView markdown={markdown}
  renderDiagram={(code) => <Diagram source={code.code} errorClass="font-mono" />} />
```

相同的 `renderDiagram` 屬性也適用於 `nsgo/ui` 的 TextView。非 Mermaid 程式碼區塊會保留其一般的算繪器；無效的圖表則會顯示其原始碼。

若要使用排版數學公式，請在 Go 宿主中匯入 `github.com/nsgo-dev/nsgo/tsx/formula` 以啟用其副作用，並搭配 `tsx/textview` 或 `tsx/ui`。這會將 `Formula` 加入 `nsgo/base`；省略該套件的應用程式則不會將數學引擎與字型納入二進位檔。它接受 `source`、`dialect`（預設為 `tex`，或設為 `typst`）、`display`、`accessibilityLabel` 與樣式屬性。字型大小與顏色會跟隨周圍文字。可直接單獨使用，或提供給 TextView 作為算繪器：

```tsx
import { Formula, TextView } from "nsgo/base";

<TextView markdown={markdown}
  renderMath={(math) => <Formula source={math.tex} display={math.display} />} />
```

相同的 `renderMath` 也適用於 `nsgo/ui` 的 TextView。複製公式時會保留其 TeX 語法。Go 的 `Renderer` 仍為宿主設定。

`UrlScheme.url()` 會讀取追蹤中的應用程式網址；`UrlScheme.setUrl(url, { history: "push" })` 會新增一筆網址項目，若省略選項則會取代目前項目。`UrlScheme.on("change", fn)` 會監聽瀏覽器的上一頁、下一頁與 hash 變更，並回傳取消訂閱函式。關於路由、同源驗證與深層連結託管，請參閱[應用程式網址](addresses.md)。

## 在 TSX 中使用 Go

註冊 Go 項目以供 TSX 匯入：

```go
tsx.Register("app/native", map[string]core.Node{"Badge": Badge{}})
tsx.RegisterFunctions("app/native", map[string]any{
	"greet": func(name string) string { return "Hello, " + name },
})
```

接著 `import { Badge, greet } from "app/native"` 便能將 `Badge` 的欄位作為 props 傳入並呼叫 `greet`。接受 `context.Context` 的函式會成為 Promise，而 `tsx.RegisterClass` 會將 Go 型別轉為 TSX 可透過 `new` 建立的類別。

轉發為 `undefined` 的選用屬性會使用其預設值。當註冊帶有 getter 且其預設值不同於型別零值的 Go 元件時，請加上例如 `tsx.Default("Max", float64(100))` 的選項；當響應式屬性回傳 undefined 時也會套用相同的預設值，而不會替換該元件。

具有文件記載之 Go 預設值的資料欄位同樣可以省略：`Flow` 節點的 `width` 與 `height` 在未提供時會量測其 JSX 內容、省略邊線的 `width` 時會使用預設筆畫寬度，而配額列省略 `pace` 時則會保持隱藏進度標記。

註冊 Go 元件與類別會在應用程式執行時透過反射尋找其匯出的欄位與方法。該查詢機制會使 Go 連結器無法移除未被呼叫的匯出方法——範圍涵蓋整個應用程式而非僅限於 TSX 所註冊的項目——因此連結 `tsx` 會使已剝除符號的桌面端二進位檔增加約 20 MB，且應用程式連結的套件越多，增加的體積越大。

## 已簽署的套件

應用程式在驗證套件後，即可執行下載取得的 TSX，例如遠端重新派送的畫面或外掛程式。請在其 `nsgo.yaml` 中宣告套件程式碼可存取的權限範圍；除此之外的一切請求都會遭到拒絕：

```yaml
capabilities:
  network:
    hosts: [api.example.com, "*.cdn.example.com"]
  open_url: true # or { schemes: [https] }
  host: [search, "player.*"] # the app's handlers it may call
  # webview: { hosts: [...], input: true, media: [microphone, camera] }, clipboard, picker, speech, text_to_speech, status_bar, notifications, biometrics, sign_in_with_apple, purchases, secrets, audio, gamepad, auth_session
```

請將根金鑰離線保存並用它簽發發行金鑰憑證，然後在該目錄中簽署應用程式。`go tool nsgo keygen` 會印出一組金鑰對且不會寫入任何檔案；請為根金鑰與每個發行金鑰各執行一次。公開金鑰放進旗標與應用程式程式碼中；私密金鑰則不應放入儲存庫，僅透過 `NSGO_ROOT_KEY` 與 `NSGO_RELEASE_KEY` 傳遞給工具：

```sh
go tool nsgo keygen # once for the root, once for the release key
NSGO_ROOT_KEY=<root secret> go tool nsgo certify -key <release public> -apps 'com.example.*'
NSGO_RELEASE_KEY=<release secret> go tool nsgo publish -cert release.cert plugin
go tool nsgo verify -root <root public> plugin/dist/Plugin-1.0.0.nsgopkg
```

應用程式信任寫在程式碼中的根公開金鑰，在引擎的 goroutine 之外驗證套件，並透過 `tsx.View` 在自身檢視旁的沙箱中顯示它。套件只能透過檢視所提供的宿主通道與應用程式互動，並保有應用程式儲存空間與密鑰中的專屬區塊：

```go
trust, err := pkg.OpenTrust(filepath.Join(cx.DataDir(), "trust"))
trust.AddRoot(root)
p, err := trust.Verify(data) // refuses tampered, revoked and older builds
channel := core.NewHost()
channel.Handle("search", func(c *core.HostCall) { c.Resolve(results(c.Data)) })
view := tsx.View{Package: p, Sandbox: tsx.Sandbox{Host: channel}}
core.Catch{Try: view.Render, Fallback: failed}
trust.Accept(p) // once it ran, older builds are refused
```

`Sandbox.Grant` 可授予比套件宣告更少的權限，`Sandbox.Limits` 可指定其他時間預算與配額，而 `Sandbox.Modules` 則可提供 NSGo 內建項目之外的 Go 函式與元件。其鍵盤快捷鍵、返回處理函式與導覽堆疊僅在鍵盤焦點位於該檢視內時生效，且僅當焦點已於該檢視內或回應使用者於其中的按壓操作時，套件才能自行移動焦點（`focus()`、`autoFocus`、`Keyboard.dismiss`、`setAccessibilityFocus`）。它會在獨立的檢視中顯示，僅填滿父層給予的空間且不超出，並對內容進行裁切（包含選單、彈出視窗與吐司通知）；在捲動檢視中，請將它置於一個檢視內，讓它採用自身內容的高度。其對話框、警示、右鍵選單或無障礙播報也僅以相同方式顯示，且返回鍵即使未移除該對話框也會將其關閉。`View.Export` 指定套件進入模組所匯出的元件名稱，用來取代進入點所掛載的根元件（此時進入點的 `mount` 既不會算繪也不會執行其元件），並傳入為每個檢視複製的可 JSON 編碼 map `View.Props`，讓單一套件能在相同的權限能力與限制下提供多個各自擁有獨立狀態的檢視；若缺少該匯出項目會以 `tsx.ErrNoViewExport` 發生 panic，若該匯出項目不是函式則會以 `tsx.ErrViewExportNotComponent` 發生 panic。進入點匯出函式的套件即為函式外掛程式：`tsx.Load(cx, p, tsx.Sandbox{})` 會在屬於檢視樹的 `cx` 下（若不屬於檢視樹則回傳 `tsx.ErrNoTree`），於呼叫者的 goroutine 上不顯示畫面並同步執行該套件，或回傳其進入模組在載入時拋出的錯誤，而 `Plugin.Call` 則負責呼叫這些函式。`go tool nsgo dev -limits` 會比照此類檢視的方式執行應用程式，而 `go tool nsgo revoke` 則會簽署一份供 `Trust.UpdateRevocations` 使用的已撤銷發行金鑰與套件清單。`examples/plugins` 會詢問使用者是否授予套件的權限能力、進行安裝並執行它。`Limits.Memory`（預設為 64 MiB）負責為陣列、字串、緩衝區與型別化陣列計量原生儲存空間預算。這包含 `concat`、`flat`、`slice`、陣列展開、字串串接、範本字串、`replace`、`replaceAll`、`TextEncoder` 與 `Blob` 複製。在 `-limits` 中使用 `memory=<bytes>` 可更改該預算，而 `memory=0` 則會將其關閉。檢查會在配置記憶體前執行。超過預算的套件在 Go 檢查完可回收的儲存空間後，會以 `ERR_NSH_QUOTA_EXCEEDED` 與資源 `memory` 中止執行。一般物件、閉包、堆疊、小型配置，以及未計量的內建項目與宿主 API 亦可能配置記憶體。瀏覽器則保有自己的堆積。透過 `pkg.Build` 自行簽署套件的應用程式，或使用 `go tool nsgo publish -extra <file>` 帶入該檔案位元組的指令，都能將自訂中繼資料放入 `pkg.Contents.Extra`（例如外掛程式加進其選單的項目）：它會隨套件一同簽署，且 `Verify` 會將其原封不動地作為 `p.Extra` 回傳給應用程式剖析，使應用程式無需執行套件程式碼即可讀取該資訊。

應用程式可使用 `pkg.SignDocument` 與 `nsgo keygen` 所印出的金鑰，自行簽署如外掛程式目錄等 JSON 文件，並以 `Signed.Open` 進行驗證。請為每種文件指定各自帶版本號的 `app:` 上下文（例如 `app:catalog-v1`）：文件絕不會通過其他類型、套件、憑證或撤銷清單的驗證。`Open` 僅完全信任傳入的公開金鑰，不使用憑證或撤銷清單，且會拒絕解碼目標型別所缺少的內容欄位。信封本身不包含時間或序號，因此較舊的文件與較新的文件同樣能通過驗證：請在內容主體中加入版本或到期時間並自行檢查。
在原生沙箱中，`JSON.stringify`、帶有 reviver 的 `JSON.parse`、`Array.prototype.join`、`toString`、`toLocaleString` 與 `flat`（以及陣列的 `String()`）一旦嵌套超過 1,000 層便會拋出可攔截的 `RangeError`，其中每一層彼此呼叫都會計入深度，例如陣列的 `toString` 與其內部呼叫的 `join` 會各自計數一次；更深層的資料會導致呼叫卡死或終止應用程式。網頁平台則使用瀏覽器的內建實作。

在原生目標平台上，`DefaultLimits.Stack` 為 1,000 個堆疊訊框。可設定 `Limits.Stack` 或 `-limits stack=<frames>` 來選擇最高至 10,000 個訊框的不同深度。小於或等於零的 Go 數值以及超過上限的輸入都會使用 10,000。網頁平台則使用瀏覽器的堆疊限制。

彼此呼叫的原生函式與 JS 呼叫一樣會計入 `Limits.Stack`：本身為內建函式的 getter、`toString` 或 `valueOf`、`exec` 為其自身另一方法的正規表達式、相互巢狀呼叫的 `Reflect.apply` 與 `Reflect.construct`，以及繫結函式或 Proxy 的呼叫鏈，一旦超過限制就會拋出可攔截的 `RangeError` 而非終止應用程式，且嵌套深度超過限制的外掛程式值在轉換為 Go 值時也會使該次呼叫以 `RangeError` 失敗。這項機制來自 NSGo 所需的 goja 分支版本 `github.com/nsgo-dev/goja`，您的應用程式無需更動自身的 `go.mod` 即可隨 NSGo 一併取得。

在原生目標平台上，`Limits.Call` 會限制每個 JavaScript 執行片段的時間，包含計時器回呼與 `await` 之後的 Promise 接續執行。當某個片段被中斷時，等待中的 `Plugin.Call` 會回報一次 `tsx.ErrBudget`、停止其剩餘的計時器，並忽略遲來的宿主回應。獨立的呼叫仍會繼續執行，包含在遭中斷片段中排入佇列的 Promise 反應。去敲定另一個呼叫之 Promise 的呼叫，依然會回報自身的結果或錯誤。具回溯行為的正規表達式會使用載入或呼叫預算中較短者作為其比對逾時時間；未處理的逾時同樣會成為 `tsx.ErrBudget`。

`Limits.CallTotal` 為每次外掛呼叫與沙箱檢視事件加上了掛鐘期限。這包含花在等待的時間，並會一併控管其計時器、間隔計時器、影格回呼、Promise 接續執行與宿主回應，即使在呼叫已敲定後亦然。到達期限時，待處理的工作會被捨棄，而仍在等待的呼叫會以 `tsx.ErrBudget` 失敗。進入模組的工作受 `Limits.Load` 限制，若 `Load` 為零則改用 `CallTotal`。獨立的宿主事件則從派送起採用 `CallTotal`。請使用 `Plugin.CallWithBudget` 取得指向等待中呼叫期限的控制代碼。在宿主的 `Scope.Post` 中，於該期限前呼叫 `budget.Extend(duration)` 即可僅為此次呼叫延長時間。

`Plugin.Interrupt` 可從任何 goroutine 停止執行階段、捨棄其待處理的計時器，並使目前及後續的呼叫失敗。`Plugin.Stopped()` 與 `Plugin.Err()` 亦可從任何 goroutine 讀取。執行階段運作期間 `Err()` 為 nil，一旦停止則回傳其首次停止的原因。正常關閉外掛程式或處置其擁有者時，會執行其清理作業並以 `tsx.ErrPluginClosed` 停止。呼叫 `tsx.Load` 時設定 `Sandbox.OnStop`，即可在等待中的呼叫失敗後，於引擎的 goroutine 上接收一次該停止原因。

載入函式外掛程式時設定 `Sandbox.OnError`，即可接收在等待中的 `Plugin.Call` 之外發生之未攔截計時器、動畫影格、`queueMicrotask` 與 `reportError` 失敗。每個外掛程式的處理函式都在引擎 goroutine 上執行，讓宿主能歸因錯誤並在斷路器開啟時呼叫 `Plugin.Interrupt`。原生片段預算中斷同樣會回報 `tsx.ErrBudget`，包含其呼叫已敲定的 Promise 工作。一般的回呼錯誤會讓外掛程式繼續執行。載入與呼叫結果的錯誤仍會回傳給各自的呼叫者，而檢視仍使用 `core.Catch`；若未設定處理函式，則維持既有的邊界與應用程式回報行為。

設定 `Sandbox.OnViewRuntime` 可在每個檢視的進入模組執行前，於引擎 goroutine 上接收該檢視的 `*tsx.ViewRuntime`。從任何 goroutine 呼叫 `Interrupt` 可停止其工作並將最後一次算繪的檢視保留在畫面上。`Stopped` 與 `Err` 會回報其終止狀態，正常處置時為 `tsx.ErrViewClosed`。`Sandbox.OnStop` 也會回報一次檢視的終止原因。

`Plugin.JSTime` 與 `ViewRuntime.JSTime` 會回報在原生執行階段之 JavaScript 進入點中花費的累計時間，包含目前的進入點。進入點之間的等待時間以及已註冊的同步 Go 函式內部花費的時間不計入其中。`ViewRuntime.EngineTime` 亦包含同步 Go 呼叫以及所屬的響應式更新、掛載、提交與版面配置工作，包含仍在執行中的工作。它排除引擎進入點之間的等待、屬於宿主或其他檢視的工作、共用的提交紀錄作業與原生繪製。兩個計數器皆可安全地從任何 goroutine 讀取。瀏覽器在 NSGo 之外的 JavaScript 執行輪次無法被量測或搶占。在瀏覽器環境下，這兩個計數器都會回報不可用，而 `Interrupt` 會在宿主取回控制權時停止未來的 NSGo 回呼。

網頁平台目前尚無法將套件與頁面隔離：其程式碼會繞過宣告的權限能力，存取頁面自身的網路、剪貼簿與儲存空間（包含信任存放區）。因此 `tsx.View` 與 `tsx.Load` 在網頁平台上會以 `tsx.ErrUnisolated` 拒絕執行套件，除非您設定 `Sandbox.WebUnisolated` 以頁面權限執行它——此選項僅適用於您願意賦予與應用程式自身相同信任度的套件。

## 編輯器與測試

繼承 `.nsgo/tsconfig.json` 的 `tsconfig.json` 能為編輯器提供 TSX 的型別定義，該檔案由 `go tool nsgo dev` 與 `go tool nsgo check` 產生。位於應用程式旁的 `*.test.tsx` 檔案使用 `nsgo/test` 撰寫，並會隨其 Go 測試一同執行（[測試](testing.md)）。
