---
title: 提交變更
description: 向 NSGo 提交 pull request 的規範，從各後端同步修改到測試、文件與更新日誌項目。
group: 參與貢獻
order: 82
---

## 開始之前

任何規模大於單純錯誤修正的改動，請先開啟 issue 或 discussion，以便在撰寫程式碼前先對設計達成共識。設計上尚未定案的問題皆列於 [ARCHITECTURE.md](https://github.com/nsgo-dev/nsgo/blob/main/ARCHITECTURE.md#open-questions)，並會先在該處決定答案。

## 涵蓋所有後端

在某個後端中修正問題或新增功能，通常也應同步實作於其他後端。請在所有需要的目標平台上進行相同變更，或將尚缺的部分記錄在[開發路線圖](https://github.com/nsgo-dev/nsgo/blob/main/docs/roadmap.md)中。元件在每個平台上的外觀與行為皆應保持一致。

## 檢查

請使用 `nsgo` 工具，而非直接執行 `go vet`、`go test` 或 `go build`：

```sh
go tool nsgo check   # vet every package
go tool nsgo test    # every package's tests
```

為你的變更補上測試：使用者可見的行為使用 `nsgotest`，其餘則使用套件自身的測試。並在你改動到的平台上執行元件展覽館（參閱[開發環境設定](../contributing.md#元件展覽館)）。

使用 `go.mod` 中指定 Go 版本的 `gofmt` 格式化 Go 檔案；`gofmt -l .` 會列出需要格式化的檔案。CI 會檢查所有受追蹤的 Go 檔案，若有未格式化的檔案便會失敗並列出其檔名。

## Pull request 規範

標題請採用 Conventional Commit 格式，例如 `fix(linux): keep the caret after a paste`：由於採 squash merge，PR 標題會直接成為 commit 的主旨，若格式不符檢查將不會通過。PR 範本會要求填寫變更所觸及的目標平台、任何視覺變更的前後截圖，以及你執行過的檢查項目。

## 文件與更新日誌

- 更新因你的變更而不再準確的文件：若涉及 NSGo 內部運作方式請更新 `docs/architecture/` 中的主題文件，若涉及應用程式與元件的使用方式請更新 `docs/public/` 中的頁面（參閱[文件與網站](docs.md)）。
- 保持 `README.md` 與 `README.zh-CN.md` 內容一致；其餘文件以英文為主，中文頁面置於 `docs/public/zh/`。
- 將使用者可見的變更寫成一行，附上例如 `(#12 by @you)` 的參照，並存放在 `changelog.d/` 內一個獨立檔案中、歸類於對應的標題（例如 `### Fixed`）之下，而不要直接修改 `CHANGELOG.md`——發佈流程會自動彙整這些檔案填入 `CHANGELOG.md`（參閱[格式說明](https://github.com/nsgo-dev/nsgo/blob/main/changelog.d/README.md)）。
