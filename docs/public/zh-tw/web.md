---
title: 在網頁上發佈
description: 以文件語意、頁面中繼資料與 CDN 資源發佈 NSGo 應用程式。
group: 工具鏈
order: 24
---

## 建置應用程式

```sh
go tool nsgo build -target web
```

透過 HTTP 提供產生的目錄。發佈版資源的檔名包含內容雜湊值，因此可將它們快取為不可變（immutable）；`index.html`、`nsgo.json` 與 `manifest.webmanifest` 則需進行重新驗證（revalidate）。網頁應用程式會保留 NSGo 的版面配置、捲動、焦點與指標行為。

## 標題與連結

帶有 `Role: core.RoleHeader` 的 `core.View` 或 `core.Text` 會依 `AccessibilityLevel` 轉為 `h1` 到 `h6`；若未指定層級，則為 `h2`。來自 `base/textview` 的 Markdown 標題會保留其層級。瀏覽器預設的標題邊距與字型不會改變應用程式的樣式。

`base.Link` 與 `ui.Link` 會將其 `Href` 發佈為超連結錨點（anchor）。一般點擊會執行應用程式的 `OnOpen` 與 `OnPress`；中鍵點擊或搭配修飾鍵的點擊則使用瀏覽器的連結動作。除非應用程式自行處理 `OnContextMenu`，否則瀏覽器會提供其連結右鍵選單。停用的連結不會有作用中的網址。若要建立自訂的連結角色視圖，請將 `core.View.Web` 設為 `core.WebViewProps{Href: "/notes"}` 或繫結 `HrefFunc`。TSX 請使用 `web={{href: "/notes"}}`；標題則使用 `accessibilityLevel={1}`。不安全的可執行網址協定（URL scheme）會遭到拒絕。受限制的內容（confined content）會將導覽保留在其作用域內的按下處理函式中，不會向瀏覽器公開網址。

## 頁面中繼資料

匯入 `github.com/nsgo-dev/nsgo/web`，並在頁面內容旁放置一個 `web.Head`：

```go
core.Group{
	web.Head{Title: "Notes", Description: "Your saved notes"},
	pageContent,
}
```

`TitleFunc` 與 `DescriptionFunc` 會跟隨訊號（signal）更新。較後出現且可見的 `Head` 會覆寫其所設定的欄位，並繼承其餘欄位。隱藏或移除頁面時會還原先前的 `Head`；移除所有 `Head` 則會還原原始文件。靜態欄位留空會繼承先前的設定；函式回傳空字串則會清除該欄位。`Head` 不會新增任何版面配置節點。原生目標平台、嵌入式應用程式與受限制的內容都不會更動其宿主的文件。

在 TSX 中，從 `nsgo/web` 匯入 `Head`，並撰寫 `<Head title={title()} description="Your saved notes" />`。針對靜態 HTML 預覽，請在部署網站時預先轉譯（prerender）每個網址；`Head` 則會更新執行中的文件。

## 從 CDN 提供資源

載入器會相對於 `nsgo.json` 解析所有應用程式檔案。頁面可以從其他來源（origin）載入應用程式：

```js
const url = new URL("https://cdn.example.com/notes/nsgo.json");
const config = await (await fetch(url)).json();
const { mount } = await import(new URL(config.loader, url).href);
await mount(null, url.href, { config });
```

請透過 CORS 允許頁面的來源擷取 CDN 上的檔案，並以 `application/wasm` 提供 WebAssembly。針對應用程式使用的任何深層路由（deep route），請提供應用程式入口 HTML。[嵌入](embedding.md#網頁)說明了如何改為掛載至特定元素中。
