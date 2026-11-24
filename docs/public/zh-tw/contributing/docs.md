---
title: 文件與網站
description: 撰寫文件頁面、進行翻譯、執行網站伺服器，並讓代理程式技能維持最新狀態。
group: 參與貢獻
order: 83
---

## 頁面格式

這些文件是位於 `docs/public/` 下的 Markdown 檔案，在 GitHub 上即可直接閱讀。`en/` 與 `zh/` 的路徑一一對應；若中文頁面缺失，網站會顯示英文版本並附上提示。每個頁面開頭皆包含 front matter：

```yaml
---
title: Testing
description: Test an app the way a user uses it, headless or in a native window.
group: Toolchain
order: 21
---
```

- `group` 是該頁面在側邊欄所屬的分組（使用該頁面的語言），`order` 則決定其排序位置。分組順序依循讀者的學習路徑：教學範例、工具鏈、元件、核心概念，接著是撰寫元件與參與貢獻。沒有設定分組的頁面（例如簡介）會排在最前面。
- 頁面版面會自動顯示 `title` 與 `description`，因此正文請從 `##` 開始。

頁面之間的連結請使用相對 `.md` 路徑（例如 `[測試](testing.md#run-on-a-platform)`）；網站建置時會將其轉換為對應的網址。每個頁面只回答一個主題問題；NSGo 內部運作原理的說明則留在 `docs/architecture/`。

## 網站建置

位於 `website/` 的網站由 Astro 整合套件 [nsgo-website](https://github.com/nsgo-dev/nsgo-website) 依據 `website/site.config.ts` 的設定建置而成。需要 [Bun](https://bun.com) 1.4 或更新版本：

```sh
cd website
bun install
bun run dev    # at http://localhost:4321/
bun run build  # type-check and build, as the deployment does
```

`website/README.md` 說明了網站的建置與發佈方式。

## 供代理程式使用

網站也會為語言模型發佈英文文件頁面：`/llms.txt` 列出所有頁面、`/llms-full.txt` 包含全部內容，而 `/docs/<page>.md` 則提供單一頁面，並附有絕對連結且移除了程式碼區塊的標記（`src/lib/llms.ts`）。這些檔案會自動隨頁面產生，無須額外步驟。

`skills/nsgo/` 是代理程式透過 `bunx skills add nsgo-dev/nsgo` 安裝的技能包：其中 `SKILL.md` 記載應用程式必須遵守的規則，`references/` 則在任務需要時才讀取。當應用程式寫法、命令列工具指令、測試或打包方式有所變更時，請在同一次變更中同步更新此技能；只保留代理程式必須遵守的規範，其餘細節則連結至文件。`go tool nsgo test -run TestSkills .` 會檢查其 front matter、連結以及所有提及的 NSGo 名稱。
