---
title: 플랫폼 서비스
description: 알림 대화상자, 클립보드, 파일, 사진, 공유, 저장소, 비밀 정보, 음성 인식, 텍스트 읽어 주기, 네트워킹, 로그인, 인앱 구매 및 앱의 수명 주기.
group: 플랫폼 기능
order: 44
---

## 알림 대화상자와 클립보드

`cx.ShowAlert`는 플랫폼의 알림 대화상자 또는 액션 시트를 표시합니다. `Actions`는 인덱스로 결과가 돌아오며, 역할(role)에 따라 취소 버튼과 파괴적 동작 버튼이 플랫폼 방식대로 그려집니다. macOS에서는 창이 보이는 동안 알림 대화상자와 파일 선택기가 시트를 사용합니다. 창이 숨겨져 있거나 최소화되어 있거나 화면 밖에 있는 경우, 또는 창이 없는 트레이 앱에서는 대화상자가 별도로 열립니다. 요청한 창을 닫으면 그 창의 대화상자도 취소됩니다.

```go
cx.ShowAlert(core.AlertConfig{
	Title:   "Delete the file?",
	Message: "This cannot be undone.",
	Actions: []core.AlertAction{
		{Label: "Cancel", Role: core.AlertCancel},
		{Label: "Delete", Role: core.AlertDestructive},
	},
}, func(index int) {
	if index == 1 {
		remove()
	}
})
```

`SetClipboard`는 텍스트를, `SetClipboardImage`는 이미지를 씁니다. `ReadClipboard`, `ReadClipboardImage`, `ReadClipboardFiles`는 비동기로 응답하며, `HasClipboardText`와 `HasClipboardImage`는 붙여넣기 알림 없이 클립보드에 무엇이 들어 있는지 읽습니다. `core.FileFromURL`은 `ReadClipboardFiles`와 `View.OnFileDrop`이 제공하는 URL, 즉 `file:` URL과 웹에서의 `blob:` URL을 `core.File`로 엽니다.

tvOS에는 공용 페이스트보드가 없습니다. 클립보드 쓰기는 무시되고, 읽기는 아무 내용도 반환하지 않으며, 존재 여부 확인은 false를 반환합니다.

```go
cx.ReadClipboard(func(text string) { pasted.Set(text) })
cx.SetClipboard("Hello")
```

macOS와 Windows에서 `ReadClipboardItems`는 사용 가능한 모든 바이트 표현을 최대 48 MiB까지 스냅숏으로 제공합니다. 각 항목은 순서가 있는 `ClipboardRepresentation` 값을 가지며, 각 값에는 네이티브 `Type`과 원시 `Data`가 들어 있고, 데이터가 비어 있는 존재 표시용 형식도 포함됩니다. `ClipboardFormats`는 페이로드를 복사하지 않고 네이티브 형식 이름만 나열합니다. 스냅숏을 복원하려면 쓰기 오류를 받는 선택적 콜백과 함께 `WriteClipboardItems`에 전달하세요. 빈 슬라이스를 전달하면 클립보드가 비워집니다. 새 복사가 일어났는지 감지하려면 `ClipboardChangeCount`의 값이 같은지 비교하세요. 복사된 텍스트가 이전과 동일한 경우에도 감지됩니다.

macOS는 항목 경계와 페이스트보드 타입 이름을 유지합니다. Windows는 하나의 항목을 사용하며, `CF_HDROP` 같은 바이트 기반 Win32 이름과 `Rich Text Format` 같은 등록 형식을 보존합니다. 쓰기 시에는 서로 다른 형식을 가진 항목들을 합치고, 빈 페이로드에는 0 바이트 하나를 사용합니다. GDI 핸들, owner-display 형식, 비공개 숫자 형식 ID는 제외됩니다. 다른 플랫폼에서는 읽기 및 쓰기 콜백이 `CodeUnavailable`을 받고, 카운트 콜백은 `-1`을 받습니다.

## 링크와 파일 열기

`cx.OpenURL`은 시스템의 브라우저나 앱에서 URL을 열고, `core.CheckURL`은 거부하는 경우를 알려 줍니다. 스킴이 없는 경우, 16 KiB를 넘는 경우, 공백이 있는 경우, 호스트가 없는 웹 주소, 그리고 `javascript:`, `data:`, `blob:`, `file:`, `about:`, `asset:`입니다.

```go
if err := core.CheckURL(url); err == nil {
	cx.OpenURL(url, func(opened bool) {
		if !opened {
			status.Set("No app opens that link.")
		}
	})
}
```

`cx.OpenFile`은 앱이 가진 파일을 해당 형식에 연결된 앱에서 열고, `cx.Share`는 macOS, iOS, Android, Windows, Linux(데스크톱 포털 또는 파일 관리자를 통해), 웹(`navigator.share`)에서 파일, 텍스트 또는 둘 다를 위한 시스템 공유 시트를 표시합니다. `core.SharedFile`은 디스크상의 경로이며, 웹에서의 `Share`처럼 파일이 메모리에 있는 경우에는 `Data` 바이트이고, 다른 앱이 보게 될 이름과 형식을 함께 가집니다. 데스크톱에서 `desktop.RevealFile`은 Finder, 탐색기 또는 파일 관리자에서 파일이나 폴더를 선택된 상태로 보여 주고, `desktop.TrashFile`은 그것을 휴지통으로 옮깁니다. 영구 삭제는 절대 하지 않으며, 볼륨에 휴지통이 없거나 휴지통이 보관할 수 없는 경우(예: 휴지통보다 큰 파일)에는 `core.ErrNoTrash`로 응답하고 항목을 그 자리에 둡니다.

```go
cx.Share(core.ShareOptions{
	Files: []core.SharedFile{{Path: path, Name: "Notes.txt"}},
	Text:  "A note from NSGo",
}, func(shown bool, err error) {})
```

`mobile.OnShared(cx, func(items []mobile.SharedItem) { ... })`는 [`android.share_types`](building.md#android에서-공유-받기) 또는 [`apple.share_types`](building.md#ios에서-공유-받기)에 대한 텍스트와 파일을 받습니다. Android 공유는 앱을 시작할 수 있으며, iOS는 앱이 활성화되거나 사용자가 다음에 앱을 열 때까지 공유를 App Group에 대기시킵니다. 대기 중인 공유는 첫 번째 핸들러를 기다립니다. iOS는 콜백이 반환될 때까지 수신함 배치를 유지하므로, 프로세스가 종료되더라도 최소 한 번은 전달됩니다. 텍스트는 `Text`로 도착하고, 파일은 `Path`, `Name`, `MIME`을 가진 읽기 가능한 캐시 사본으로 도착합니다. 파일이 캐시보다 오래 남아야 한다면 영구 저장소로 옮기거나 복사하세요.

`desktop.LookUp`은 macOS에서 세 손가락 탭을 했을 때처럼 텍스트에 대한 시스템 사전 패널을 뷰의 특정 영역 위에 표시합니다. 선택 영역의 텍스트와 영역을 그 `Selectable` 뷰의 `Ref`와 함께 전달합니다. `desktop.CanLookUp`은 macOS의 창 안에서 이것이 가능한지 알려 줍니다. Windows, Linux, 웹에는 그런 패널이 없습니다. 테스트에서는 `nsgotest.Environment`가 `CanLookUp`을 설정한 경우 `nsgotest.App.LookUps`가 표시된 패널을 나열합니다.

```go
var area core.Ref
var selected core.TextSelection
core.View{Ref: &area, Selectable: true, OnTextSelection: func(s core.TextSelection) { selected = s }, Children: subtitles}
// From a menu item or a shortcut:
if desktop.CanLookUp(cx) {
	desktop.LookUp(cx, selected.Text, &area, selected.Bounds)
}
```

`desktop.QuickLook`은 특정 인덱스부터 시작하는 파일 미리보기를 요청하며, 패널이 열려 있는 동안 다시 호출하면 파일을 갱신합니다. `desktop.CanQuickLook`은 macOS와 Windows에서는 앱 자체 창에서의 사용 가능 여부를, Linux에서는 세션이 GNOME Sushi의 실행 중이거나 활성화 가능한 D-Bus 서비스를 제공하는지를 알려 줍니다. Windows는 파일 형식에 등록된 미리보기 처리기를 사용하므로, 지원되는 형식은 설치된 처리기나 미리보기 프로그램에 따라 다릅니다. true 결과는 플랫폼에 요청했다는 뜻일 뿐, 파일을 렌더링했다는 뜻은 아닙니다.

Space나 Escape를 누르면 패널이 닫힙니다. 화살표 키는 Windows에서, 그리고 X11에서 선택과 소유권을 보고하는 Sushi 버전에서 요청된 파일 사이를 선택합니다. Wayland에서는 현재 Linux가 부모 창이 없는 미리보기를 열기 때문에 소유 창과 함께 자동으로 닫히는 기능과 화살표 선택을 사용할 수 없습니다. KDE 전용 미리보기 프로그램과 웹은 지원되지 않습니다.

```go
if desktop.CanQuickLook(cx) {
	desktop.QuickLook(cx, []string{reportPath, sourcePath}, 0)
}
```

## 네이티브 메뉴 아이콘

`desktop.MenuItem.Icon`은 macOS, Windows, Linux의 애플리케이션 메뉴, Dock 메뉴, 트레이 메뉴, 팝업 메뉴에서 항목 옆에 아이콘을 표시합니다. `desktop.MenuIcon`은 파싱된 벡터 아이콘(`SVG`, 예: `lucide.Folder()`), 인코딩된 이미지 바이트(`Data`, PNG, JPEG, GIF, WebP 또는 SVG), 또는 local/asset/pixel `core.ImageSource`(`Source`)를 받습니다. `Template`을 설정하면 흑백 이미지를 메뉴의 텍스트 색으로 칠하며, 라이트 및 다크 모드(그리고 macOS에서는 강조 상태)를 따릅니다. 흑백 `SVG` 아이콘은 기본적으로 템플릿으로 취급됩니다.

```go
desktop.SetApplicationMenu(cx, []desktop.MenuItem{{
	Label: "File",
	Items: []desktop.MenuItem{
		{Label: "Open Folder…", Icon: desktop.MenuIcon{SVG: lucide.Folder()}, OnSelect: openFolder},
		{Label: "Export", Icon: desktop.MenuIcon{Data: exportPNG, Template: true}, OnSelect: exportFile},
	},
}})
```

아이콘은 로컬 소스에서 동기적으로 로드됩니다. 원격 URL이나 유효하지 않은 이미지는 항목을 아이콘 없이 남겨 둡니다. 메뉴 레이블, 단축키, 체크/라디오 표시는 네이티브 동작을 유지합니다.

테스트에서는 `nsgotest.MenuItem.Icon`이 `app.Menu`, `app.DockMenu`, `app.PopupMenu`에 각 항목의 아이콘을 기록합니다.

## 사진과 파일

`cx.PickImages`는 시스템의 사진 선택기를 표시하고 `core.PickedFile`로 응답하며, 앱은 그 바이트, 이름, 형식, 픽셀 크기를 읽습니다. `cx.TakePhoto`는 플랫폼에 카메라가 있으면 카메라를 표시하며, `cx.Env().CanTakePhoto`가 카메라가 있는지 알려 줍니다. macOS, Windows, Linux에서는 스캐너의 카메라 미리보기를 사진 찍기 버튼과 함께 표시합니다. `cx.ScanCode`는 카메라 미리보기를 열고 처음 읽은 코드로 응답합니다. `ScanOptions.Kinds`가 다른 종류를 요청하지 않으면 QR 코드이며, 사용자가 닫으면 nil로 응답합니다. `cx.Env().CanScanCode`는 플랫폼에 스캐너가 있는지 알려 주며, 카메라가 있는 macOS, iOS, Android 기기, 카메라가 있는 Windows 컴퓨터, Camera 포털과 GStreamer `pipewiresrc`가 있는 Linux 데스크톱, `getUserMedia`와 `BarcodeDetector`가 있는 브라우저가 해당됩니다. 요청한 창이 보이면 macOS는 시트를, Windows와 Linux는 창 위의 모달을, 웹은 페이지 위의 오버레이를 표시합니다. macOS와 iOS에서는 `apple.info`에 `NSCameraUsageDescription`을, Android에서는 `android.permissions`에 `CAMERA`를 넣으세요. Android, Windows, Linux는 PDF417을 제외한 모든 종류를 읽습니다. 코드의 텍스트는 신뢰할 수 없는 입력으로 취급하세요.

tvOS에서 `PickImages`는 이미지 없이 응답하고 `TakePhoto`는 `core.ErrNoCamera`를 반환합니다. NSGo는 Continuity Camera 선택기를 제공하지 않습니다.

테스트에서는 `nsgotest.App.PickImages`, `nsgotest.App.TakePhoto`, `nsgotest.App.ScanCode`가 선택과 촬영에 대해서는 `nsgotest.ImagePick` 레코드를, 스캔에 대해서는 `nsgotest.CodeScan` 레코드를 받고, 시스템 UI 대신 응답합니다. 이 훅은 헤드리스에서도, 지원되는 `nsgo test -target` 호스트에서도 동작합니다. `nsgotest.Environment(core.Environment{Camera: true, ScanCode: true})`는 카메라가 없는 타깃에 카메라를 제공합니다.

```go
cx.PickImages(core.PickOptions{Limit: 1, MaxDimension: 2048}, func(files []core.PickedFile) {
	if len(files) == 1 {
		avatar.Set(files[0].Data)
	}
})
```

`cx.PickFiles`는 모든 형식의 파일을 선택하고, `cx.PickDirectories`는 폴더를 `fs.FS`로 선택하며, `cx.SaveFile`은 시스템의 저장 대화상자를 통해 파일이나 바이트를 저장합니다. 두 선택기 모두 `Title`, `Button` 레이블, `Start` 폴더를 받고, `PickFilesOptions.TypesName`은 형식 메뉴의 이름을 지정하므로 앱은 자신의 언어로 선택기를 표시할 수 있습니다. 이는 힌트이며, macOS, Windows, Linux는 이를 따르고, iOS는 `Start`만 따르며, Android와 웹은 전혀 따르지 않습니다. `cx.PickScreenColor`는 `cx.Env().CanPickScreenColor()`가 true인 곳, 즉 macOS, Windows, 데스크톱 Screenshot 포털이 있는 Linux, `EyeDropper`가 있는 브라우저에서 화면 어디에서든 색을 선택합니다.

macOS, Windows, Linux, iOS에서 저장 대화상자를 특정 위치에서 열려면 `core.SaveOptions.Start`를 존재하는 절대 경로 폴더로 설정하세요. 힌트가 비어 있거나 사용할 수 없는 경우, 또는 다른 타깃에서는 시스템이 폴더를 선택합니다.

쓰기 전에 선택된 저장 위치를 검증하려면 `core.SaveOptions.Accept`를 설정하세요. 이 함수는 엔진의 고루틴에서 실행됩니다. 오류를 반환하면 저장 위치는 그대로 유지됩니다. macOS, Windows, 그리고 Linux의 인프로세스 GTK 선택기에서는 대화상자가 그 이유를 표시하고 사용자가 다른 경로를 선택할 수 있게 합니다. 거부된 뒤 취소하면 `saved == false`, 거부된 경로, 오류가 보고됩니다. Android와 Linux 데스크톱 포털에서는 거부가 그 결과로 저장을 완료합니다. Android는 사용자가 폴더를 선택하게 하고, 요청한 파일 이름을 `name` 쿼리 매개변수로 가진 트리 URI를 제공합니다. 승인 후에 문서를 만들고, 성공하면 새 문서의 URI를 보고합니다. 검사가 5초 안에 응답하지 않으면 플랫폼은 쓰기를 포기합니다. iOS와 HarmonyOS는 검사가 필요한 저장에 대해 오류를 보고합니다. 웹에는 저장 위치 경로가 없으므로 일반적인 저장 흐름을 사용합니다.

헤드리스 테스트에서는 `App.PickFiles`, `App.PickDirectories`, `App.SaveFile`이 `nsgotest.FilePick`, `nsgotest.DirectoryPick`, `nsgotest.FileSave` 레코드를 받습니다. 같은 레코드가 `app.FilePicks`, `app.DirectoryPicks`, `app.Saves`에도 있습니다.

### 번들 에셋

앱의 `resources/` 디렉터리 아래에 파일을 두고 `cx.ReadAsset("asset:maps/level.bin", done)`으로 Go에서 읽으세요. 콜백은 읽기가 끝난 뒤 엔진의 고루틴에서 `([]byte, error)`를 받으며, 소유자가 사라지거나 다시 실행되면 버려집니다. 없는 파일을 식별하려면 `errors.Is(err, core.ErrAssetNotFound)`를 사용하세요. 읽기는 파일 전체를 메모리에 로드합니다.

## PCM 오디오 풀(pull) 재생

앱이 디코딩하거나 합성한 인터리브 Float32 PCM을 재생하려면 `audio.NewOutput(cx, audio.OutputConfig{SampleRate: 48000, Channels: 2, Render: render})`를 사용하세요. `Start`는 샘플 가져오기를 시작하고, `Pause`는 가져오기를 멈추며, 소유 스코프가 사라질 때 출력이 닫힙니다. macOS, iOS, Android, Windows, Linux, 웹이 이 스트림을 지원합니다. 다른 타깃은 `audio.ErrUnsupported`를 반환합니다.

`Render`는 채운 프레임 수를 반환합니다. 오디오 스레드에서 실행되므로 블로킹, 메모리 할당, 그래프 접근을 피해야 합니다. 덜 채우면 나머지 부분은 무음으로 재생되고 `Underruns`가 증가합니다. `Position`은 단조 증가하는 프레임 수와 그 프레임의 호스트 시간을 반환합니다. 대기 중인 프레임을 버리려면 `Flush`를, 버퍼 지연 시간을 읽으려면 `Latency`를 사용하세요. 경로 변경과 중단이 일어나면 출력이 일시 정지되고 엔진에서 `OnInterrupt`가 호출됩니다. 앱은 `Start`로 출력을 재개할 수 있습니다.

## 저장소

`cx.Storage()`는 앱의 키-값 저장소로, 기기에 보관되며 앱의 모든 그래프가 공유합니다. 읽기는 동기적이며, 계산(computation) 안에서 읽으면 그 계산이 변경 사항을 구독합니다. `SetItem`은 할당량(`core.StorageQuota`, 기본값 5 MiB)을 넘으면 `core.StorageFullError`를 반환합니다. 저장소는 사용자의 어떤 프로그램이든 읽을 수 있는 일반 파일이므로, 토큰과 비밀번호는 대신 [비밀 정보](#비밀-정보)에 보관하세요.

`cx.Store()`는 기반 저장소를 반환합니다. 자체 고루틴에서 플러그인을 실행하는 경우처럼 `core.RootOptions.Storage`로 다른 루트를 만드는 호스트를 위한 것으로, 이렇게 하면 두 루트가 앱의 항목과 기기에 보관된 내용을 공유합니다. 이는 저장소 전체이며, `cx.ProvideStorage`가 앱이 작성하지 않은 코드에 제공하는 파티션이 아닙니다.

자체 영속성을 제공하는 호스트는 `core.StorageBackend`와 함께 `core.NewStore(backend)`를 호출합니다. `Load() (map[string]string, error)`는 한 번 로드하고, `Write(core.StorageChange)`는 변경 사항을 순서대로 받습니다. `change.Op`(`core.StorageSet`, `core.StorageRemove`, `core.StorageClear`)로 분기하고 `change.Key`와 `change.Value`를 사용하세요. 메모리에만 보관하는 저장소라면 `nil`을 전달하세요.

```go
store := cx.Storage()
if tab, ok := store.GetItem("tab"); ok {
	selected.Set(tab)
}
store.SetItem("tab", "library")
```

`store.Keys()`는 키를 나열하고, `RemoveItem`은 하나를 삭제하며, `Clear`는 저장소를 비웁니다. `cx.DataDir()`와 `cx.CacheDir()`는 키-값 저장소에 담기에는 너무 큰 파일을 위한 앱의 폴더를 제공합니다. `cx.LogsDir()`는 로그 파일을 위한 플랫폼의 디렉터리를 제공하고, `Scope.ProvideStorage`는 하위 트리에 자체 파티션을 제공합니다.

iOS App Clip과 그 전체 앱은 `app_clip`과 명시적인 `apple.app_group`이 구성되어 있으면 이 데이터 및 저장소 위치를 공유하며, 캐시는 분리된 상태로 유지됩니다. 그룹을 활성화해도 기존 샌드박스 데이터가 이전되지는 않습니다. 구성과 서명은 [App Clip 저장소](building.md#app-clip)에서 다룹니다.

HarmonyOS에서는 `os.ReadFile`, `os.WriteFile`, `os.Rename` 같은 Go 파일 작업이 이 디렉터리들과 `os.TempDir()` 아래에서 동작합니다. 상대 경로는 files 디렉터리에서 시작합니다. 권고(advisory) 파일 잠금과 심볼릭 링크는 사용할 수 없습니다.

### 사용자 폴더

`desktop.UserDir(cx, desktop.DownloadsDir)`는 사용자 폴더 중 하나의 경로를 반환합니다: `DocumentsDir`, `DownloadsDir`, `DesktopDir`, `PicturesDir`, `MusicDir`, `VideosDir`. 매번 시스템에 물어보므로 사용자가 옮긴 폴더도 따라갑니다. macOS에서는 홈 디렉터리의 폴더(동영상은 `Movies`), Windows에서는 OneDrive 폴더를 포함한 알려진 폴더(known folders), Linux에서는 XDG 사용자 디렉터리입니다. 폴더가 아직 존재하지 않을 수도 있습니다. 휴대폰, 웹, HarmonyOS에는 앱이 경로로 접근하는 이런 폴더가 없으며, Linux에서 `user-dirs.dirs`에 빠진 폴더도 마찬가지입니다. 이 경우 `UserDir`는 `core.ErrUserDirUnavailable`을 반환하며, 앱은 대신 `cx.PickDirectories`나 `cx.SaveFile`로 사용자에게 묻습니다.

```go
dir, err := desktop.UserDir(cx, desktop.DownloadsDir)
if err != nil {
	dir = cx.DataDir()
}
```

테스트는 사용자 폴더를 받지 않습니다. 각 폴더는 테스트 임시 디렉터리 안의 빈 디렉터리이며, `nsgotest.UserDirs(map[core.StandardDir]string{…})`로 고정할 수 있고 지정하지 않은 폴더는 사용할 수 없습니다.

### 크래시 보고서

네이티브 릴리스 빌드는 Go 크래시 트레이스를 `cx.LogsDir()/crashes` 아래에 보관합니다. Apple은 잡히지 않은 Objective-C 예외를, Android는 잡히지 않은 Java 예외를, Windows는 최상위 필터에 도달한 SEH 예외를 기록합니다. 다음 실행 시 `cx.CrashReports()`로 대기 중인 보고서를 나열하세요. 각 보고서에는 그것을 생성한 버전, 빌드, 플랫폼이 표시됩니다. `Open()`을 호출해 진단 텍스트를 읽고 리더를 닫은 다음, 앱이 보고서를 처리했으면 `Acknowledge()`를 호출해 삭제하세요. 이 파일 작업들은 고루틴에서 실행하세요.

## 디스크 공간

`cx.DiskSpace(func(space core.DiskSpace, err error) { … })`는 `cx.DataDir()`가 있는 볼륨을 비동기로 조회하고 엔진 고루틴에서 콜백을 호출합니다. `Total`은 시스템이 보고한 용량, `Available`은 사용자가 요청한 작업에 앱이 사용할 수 있는 공간, `Opportunistic`은 캐시와 프리페치에 사용할 수 있는 공간입니다. 세 값 모두 바이트 단위입니다. 시스템이 해당 수치를 보고하는 경우 `Available`에는 시스템이 필요할 때 회수하는 공간이 포함됩니다. 여유 공간 수치가 하나뿐인 플랫폼은 두 필드 모두에 그 값을 반환합니다. 웹에서 `Total`은 해당 오리진에 대한 브라우저의 예상 저장소 할당량이고 `Available`은 사용되지 않은 부분입니다. 서비스를 사용할 수 없으면 `core.ErrDiskSpaceUnavailable`을 반환합니다.

Apple 빌드는 `nsgo.yaml`의 `apple.privacy.accessed_apis`에 `NSPrivacyAccessedAPICategoryDiskSpace`를 선언해야 합니다. 쓰기 전에 공간을 확인하거나 공간이 부족할 때 파일을 정리하고 그 변화를 사용자가 확인할 수 있다면 `E174.1`을 선언하세요. 사용 가능한 공간을 사용자에게 보여 준다면 `85F4.1`을 선언하세요. 앱은 측정값의 사용 및 공유에 관한 Apple의 제한을 따라야 합니다. 이 카테고리가 없는 빌드는 `core.ErrNotDeclared`를 반환합니다. [Apple이 승인한 사유](https://developer.apple.com/documentation/bundleresources/app-privacy-configuration/nsprivacyaccessedapitypes/nsprivacyaccessedapitype)

테스트의 응답은 `nsgotest.DiskSpace(space, err)`로 설정하세요.

## 클라우드 저장소

`cloud` 패키지는 작은 키-값 항목(`cloud.Get`, `cloud.Set`, `cloud.Delete`, `cloud.Keys`)과 더 큰 파일 또는 저장 스냅숏(`cloud.ReadFile`, `cloud.WriteFile`, `cloud.DeleteFile`, `cloud.ListFiles`)을 기기 간에 동기화합니다. macOS와 iOS에서는 iCloud Key-Value Store와 ubiquity 문서 컨테이너를, Android에서는 Google Play Games Saved Games를 사용합니다.

모든 작업은 엔진 고루틴에서 콜백을 전달하며, `cx`가 해제되었으면 콜백을 건너뜁니다. `cx.Env().CanUseCloudStorage()`는 클라우드 저장소를 사용할 수 있는지 알려 줍니다. 사용자가 로그아웃했거나, 인타이틀먼트가 없거나, 플랫폼에 네이티브 클라우드 백엔드가 없는 경우(Windows, Linux, 웹) 모든 호출은 `cloud.ErrUnavailable`을 반환하며 쓰기를 조용히 버리는 일은 없습니다.

```go
if cx.Env().CanUseCloudStorage() {
	cloud.WriteFile(cx, "saves/slot1.sav", saveBytes, func(err error) {
		if err != nil {
			status.Set(err.Error())
		}
	})
}

cloud.WatchChanges(cx, func(ev cloud.ChangeEvent) {
	reload(ev.Keys, ev.Files)
})

cloud.OnConflict(cx, func(c cloud.Conflict) {
	_ = c.Resolve(mergeSaves(c.Local, c.Remote))
})
```

`cloud.WatchChanges`와 `cloud.OnConflict`는 외부의 여러 기기에서 발생한 변경과 버전 충돌을 엔진 고루틴에서 전달하며, `cx`가 해제되면 자동으로 정리됩니다. 충돌한 파일을 읽으면 `*cloud.ConflictError`(`errors.Is(err, cloud.ErrConflict)`와 일치)가 반환됩니다.

`nsgo build`에서 Apple iCloud 인타이틀먼트를 활성화하려면 `nsgo.yaml`에 `cloud:`(또는 `icloud:`) 섹션을 추가하세요.

```yaml
cloud:
  kv_store: true
  containers:
    - iCloud.com.example.myapp
```

서명 프로필은 요청한 iCloud 기능을 허용해야 합니다. macOS에서는 Mac App Store 빌드(`sign.macos_profile`)만 iCloud를 포함합니다. macOS는 프로비저닝 프로필이 있어야만 iCloud를 허용하기 때문이며, Developer ID 빌드는 경고와 함께 iCloud를 제외합니다. Android에서 클라우드 저장소를 사용하려면 앱에 Play Games Services v2 SDK가 있어야 하고 플레이어가 로그인되어 있어야 합니다.

Steam 같은 데스크톱 스토어에서는 Steam Auto-Cloud가 추가 코드 없이 `cx.DataDir()`를 직접 동기화합니다. 로컬 저장 파일을 계속 그곳에 쓰고 Steam이 디렉터리를 동기화하게 하세요.

테스트에서 `nsgotest`는 각 앱에 격리된 메모리 내 클라우드 저장소를 제공합니다. `nsgotest.CloudAvailable` 또는 `app.SetCloudAvailable`로 사용 가능 여부를 전환하고, `app.SimulateCloudChange`로 외부의 여러 기기에서 일어나는 업데이트를 시뮬레이션하며, `app.SimulateCloudConflict`로 충돌 해결을 테스트하세요.

## 비밀 정보

`cx.Secrets()`는 리프레시 토큰, API 키, 비밀번호를 위한 앱의 보안 저장소입니다. 계정별로 문자열을 `Get`, `Set`, `Delete`하며, 플랫폼이 이를 보호합니다. 없는 계정은 `("", false, nil)`로 읽히고, 웹처럼 플랫폼에 보안 저장소가 없는 경우에는 모든 호출이 `core.ErrSecretsUnavailable`을 반환합니다. NSGo는 절대 일반 저장소로 대체하지 않습니다. 키체인이 사용자에게 묻는 것처럼 시스템이 호출을 대기시킬 수 있으므로, 이 호출들은 엔진의 고루틴 밖에서 실행하고 결과는 `cx.Post`로 넘겨주세요.

```go
secrets := cx.Secrets()
go func() {
	token, ok, err := secrets.Get("refresh-token")
	cx.Post(func() {
		if err == nil && ok {
			login(token)
		}
	})
}()
```

| 플랫폼 | 비밀 정보가 보관되는 곳 |
| --- | --- |
| macOS | 로그인 키체인의 일반 암호: `kSecAttrService`가 서비스, `kSecAttrAccount`가 계정 |
| iOS, tvOS, watchOS | 같은 방식의 키체인 일반 암호 |
| Windows | 대상이 `service/account`인 자격 증명 관리자의 일반 자격 증명, 최대 2560바이트 |
| Linux | `service`와 `account` 속성을 가진 Secret Service(GNOME Keyring, KWallet)의 항목. 세션에서 Secret Service가 실행되지 않으면 사용할 수 없음 |
| Android | Android Keystore의 AES 키로 암호화되어 앱의 비공개 파일에 저장되며 백업에서 제외됨. 읽기 시에는 `expo-secure-store`가 자신의 `SecureStore` `SharedPreferences`에 암호화해 둔 값으로도 대체함 |
| WeChat 미니 프로그램 | 플랫폼이 암호화하는 저장소(`encrypt: true`를 지정한 `wx.setStorage` / `wx.getStorage`, 기본 라이브러리 2.21.3 이상). 이전 기본 라이브러리에서는 사용할 수 없음 |
| 웹, 기타 미니 프로그램 | 사용할 수 없음(`core.ErrSecretsUnavailable`): 브라우저에는 OS가 보호하는 클라이언트 비밀 저장소가 없으며(웹 세션은 서버가 설정하는 `HttpOnly` 쿠키에 보관하세요), QQ, Alipay, Douyin 미니 프로그램에는 암호화 저장소가 없음 |

서비스는 기본적으로 앱의 ID(`core.App().ID`)입니다. 다른 프레임워크에서 옮겨 온 앱은 그 프레임워크가 쓴 항목을 지정해 사용자의 로그인 상태를 유지할 수 있습니다. `Service`, 그리고 Apple 플랫폼에서는 `AccessGroup`과 `Accessible`을 지정합니다. Android에서는 `expo-secure-store`가 `SharedPreferences`에 암호화한 값을 읽으려면 `Service`를 그 `keychainService`(기본값 `"key_v1"`)로 설정하세요. 쓰거나 삭제하면 이전된 항목이 지워집니다. iOS에서 `core.SecretAfterFirstUnlock`은 휴대폰이 잠겨 있는 동안에도 백그라운드 작업이 항목을 읽을 수 있게 하며, `ThisDeviceOnly` 접근성은 항목을 백업에서 제외합니다. macOS에서 `AccessGroup`은 항목을 데이터 보호 키체인으로 옮기며, 이를 위해서는 `keychain-access-groups` 인타이틀먼트에 해당 그룹을 나열한 팀이 서명한 빌드가 필요합니다. 이 인타이틀먼트는 Mac App Store 빌드와 Developer ID 빌드 모두에 대해 `nsgo.yaml`의 `macos.entitlements` 아래에 구성합니다.

```go
s := cx.Secrets()
s.Service = "app:no-auth" // the service an earlier build used
s.Accessible = core.SecretAfterFirstUnlockThisDeviceOnly
err := s.Set("refresh-token", token)
```

`nsgotest`는 앱의 비밀 정보를 메모리의 `App.Secrets`에 보관하고, `nsgotest.Secrets`로 일부 비밀 정보를 가진 채 테스트를 시작할 수 있으므로 테스트가 컴퓨터의 키체인에 손대는 일은 없습니다. TSX 패키지는 `secrets` 기능(capability)으로 자체 서비스 아래의 저장소에 접근합니다.

## 네트워킹

`cx.Env().Network()`는 시스템에 사용 가능한 경로가 있는지(`Online`), 그리고 시스템이 그 경로를 종량제이거나 비용이 많이 드는 것으로 간주하는지(`Expensive`)를 알려 줍니다. 바인딩은 다른 환경 값과 마찬가지로 이를 따릅니다. `cx.OnNetworkChange(func(core.Network))`는 스코프가 살아 있는 동안 변경이 있을 때마다 엔진의 고루틴에서 핸들러를 호출합니다. 시스템의 경로는 특정 서버가 응답하는지에 대해서는 아무것도 알려 주지 않습니다. 연결 상태를 보고할 수 없는 타깃은 기본적으로 온라인이며, 알 수 없는 비용은 기본적으로 false입니다.

오프라인 동작을 테스트하려면 `nsgotest.Environment` 또는 `App.SetEnvironment`를 통해 `core.Environment{Network: &core.Network{Online: false}}`를 설정하세요.

Go 자체의 `net/http`는 시스템의 TLS 및 프록시 설정을 사용해 모든 타깃에서 요청을 보냅니다. 시그널은 엔진의 고루틴에 속하므로, 다른 고루틴에서 받은 응답은 `cx.Post`를 통해 시그널에 전달합니다.

```go
go func() {
	resp, err := http.Get("https://api.example.com/items")
	if err != nil {
		cx.Post(func() { errText.Set(err.Error()) })
		return
	}
	defer resp.Body.Close()
	var items []Item
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		cx.Post(func() { errText.Set(err.Error()) })
		return
	}
	cx.Post(func() { list.Set(items) })
}()
```

`cx.Post`는 시그널을 설정할 수 있는 그래프의 고루틴에서 함수를 실행합니다. Android에서는 매니페스트가 인터넷 권한을 요청하며, 이미지 다운로드는 디스크에 자동으로 캐시됩니다.

`core.NewResource`는 소스에서 로드되는 값에 대해 같은 일을 합니다. 페처(fetcher)는 자체 고루틴에서 실행되며, 소스가 바뀌면 다시 로드합니다.

```go
items := core.NewResource(cx, func() (string, bool) { return query.Get(), query.Get() != "" },
	func(ctx context.Context, q string, _ core.ResourceInfo) ([]Item, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.example.com/items?q="+url.QueryEscape(q), nil)
		if err != nil {
			return nil, err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		var list []Item
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			return nil, err
		}
		return list, nil
	})
```

`items.Get()`, `items.Err()`, `items.Loading()`은 바인딩이 따르는 시그널입니다. 새 쿼리는 아직 실행 중인 요청을 `ctx`를 통해 취소하므로, 늦게 도착한 응답이 더 새로운 응답을 대체하는 일은 없습니다. `items.Refetch()`는 다시 시도하며, 로드하는 동안 마지막 목록을 유지합니다. `items.Mutate`는 낙관적 업데이트를 위해 값을 즉시 설정합니다.

`core.Suspense`는 콘텐츠가 읽는 리소스가 첫 값을 로드하는 동안 대체 화면을 표시하므로, 화면에 별도의 로딩 플래그가 필요 없습니다.

```go
core.Suspense{
	Content:  func(cx *core.Scope) core.Node { return itemList(cx, items) },
	Fallback: func(*core.Scope) core.Node { return ui.Spinner{Label: "Loading"} },
}
```

콘텐츠는 기다리는 동안 마운트된 채로 숨겨져 있으므로 값이 도착하면 즉시 표시됩니다. 다시 가져오는(refetch) 동안에는 마지막 값으로 계속 표시됩니다.

### 로컬 서비스 탐색

macOS, iOS, Android, Windows, Linux에서 `cx.AdvertiseService`는 수신 대기 포트와 TXT 속성을 게시합니다. `cx.BrowseServices`는 확인된(resolved) 서비스를 호스트, 포트, TXT 속성과 함께 보고합니다. `Host`는 DNS 호스트 이름이거나 IP 주소일 수 있습니다. 서비스가 사라지면 콜백은 `Removed`가 설정된 상태로 그 서비스를 다시 받습니다. 플랫폼 지원 여부는 `cx.Env().CanDiscoverServices()`로 확인하세요. 두 호출 모두 중지 함수를 반환하며, 스코프가 해제되면 종료됩니다. 플랫폼 오류는 엔진 고루틴에서 비동기로 보고됩니다. Linux는 Avahi를 사용하며, 데몬을 사용할 수 없으면 오류를 보고합니다.

Apple 플랫폼의 Bonjour를 사용하려면 `nsgo.yaml`에 서비스 타입을 선언하고 로컬 네트워크 접근에 대한 설명을 넣으세요. 시스템이 권한 요청 창을 관리하고 사용자의 선택을 기억합니다.

```yaml
apple:
  info:
    NSLocalNetworkUsageDescription: Finds nearby devices to sync with.
    NSBonjourServices:
      - _example._tcp
```

Android는 `NsdManager`를 사용합니다. Android 12 이하, 그리고 T Extensions 7 이전의 Android 13에서는 Wi-Fi 탐색에 `CHANGE_WIFI_MULTICAST_STATE`가 필요합니다. 독립 실행형 빌드는 이 권한을 선언합니다. NSGo를 Android 호스트에 임베드한다면 해당 기기를 위해 호스트 매니페스트에 이 권한을 추가하세요. Android의 원시 TXT API(API 37 또는 T Extensions 22)가 없는 기기에서는 게시하는 TXT 값이 유효한 UTF-8이어야 합니다. 탐색 시에는 원시 TXT 값이 보존됩니다. Android 호스트가 API 37 이상을 대상으로 한다면 Android 17 이상에서 `ACCESS_LOCAL_NETWORK`를 선언하고 요청해야 합니다.

헤드리스 테스트에서 탐색 결과를 제공하려면 `nsgotest.LocalServices`를 사용하세요.

## 지금 재생 중

macOS의 제어 센터와 메뉴 막대에 앱의 현재 미디어를 표시하려면 `cx.SetNowPlaying(core.NowPlaying{...})`을 사용하세요. `Title`, `Artist`, `Album`과 선택 사항인 `Artwork`를 `*core.ImageSource`로 제공합니다. `Duration`이 0이면 라이브 스트림을 뜻합니다. 현재 `Position`과 재생 `Rate`(일시 정지 중에는 0, 일반 속도에서는 1)를 제공하세요. 트랙, 위치 또는 속도가 바뀌면 다시 게시하세요. 호출 사이에는 시스템이 위치를 진행시킵니다.

지원하는 전송 컨트롤을 `Commands`에 나열하세요. 예를 들어 `core.MediaPlay`, `core.MediaPause`, `core.MediaTogglePlayPause`, `core.MediaSeek` 등이 있습니다. 요청은 `cx.OnMediaCommand`로 처리합니다. 콜백은 명령의 `Kind`와, 탐색(seek)인 경우 요청된 `Position`을 받습니다. 살아 있는 가장 최근의 핸들러가 명령을 받습니다. 앱의 재생이 끝나면 `cx.ClearNowPlaying()`을 호출하세요. 앱이 수동으로 게시하는 동안 macOS의 제어되는 비디오는 자체 게시를 중단합니다. 플랫폼 지원 여부는 `cx.Env().CanShowNowPlaying()`으로 확인하세요.

## 백그라운드 다운로드

독립 실행형 iOS 및 macOS 앱과 Android에서는 `cx.Download`와 `cx.OnDownloadUpdate`를 사용하세요. 임베드된 Apple 뷰에는 전송 호스트가 없습니다. Android 앱은 `nsgo.yaml`에서 `android.background_downloads: true`를 활성화합니다. 다운로드는 앱이 보이는 동안 사용자 동작으로 시작하세요. 앱이 쓸 수 있고 부모 디렉터리가 존재하는 절대 경로를 저장 위치로 사용하세요.

```go
cx.OnDownloadUpdate(func(p core.DownloadProgress) {
    progress.Set(p)
})
task, err := cx.Download(core.DownloadRequest{URL: url}, path, core.DownloadOptions{
    CertificatePEM: serverCertificate,
})
```

전송은 GET을 사용하며 선택적인 요청 헤더를 받습니다. `CertificatePEM`이 서버의 리프 인증서를 고정(pin)하지 않는 한 일반 HTTPS 신뢰 저장소를 사용합니다. 고정을 사용할 때도 호스트 이름이 일치하고 인증서가 유효해야 합니다. 서비스는 해당 URL로부터의 성공 응답을 요구하므로 최종 URL을 사용하세요.

Apple에서 공개 호스트 이름의 자체 서명 인증서를 사용하려면 `apple.info`에 해당 호스트에 대한 App Transport Security 예외가 필요합니다. 전송은 여전히 정확한 인증서 고정, 호스트 이름, 유효성을 확인합니다. 로컬 NAS 연결에는 접근 이유를 설명하는 `NSLocalNetworkUsageDescription`도 필요하며, 정규화되지 않은 호스트나 `.local` 호스트는 `NSAllowsLocalNetworking`을 사용합니다. [Apple의 ATS 설정](https://developer.apple.com/documentation/security/preventing-insecure-network-connections)

```yaml
apple:
  info:
    NSAppTransportSecurity:
      NSExceptionDomains:
        files.example.com:
          NSExceptionAllowsInsecureHTTPLoads: true
```

Android 포그라운드 서비스는 액티비티가 화면을 벗어난 뒤에도 계속됩니다. Android의 알림 권한에 따라 진행률 알림을 게시합니다. 진행률은 바이트 수를 사용하며, 전체 크기를 알 때까지 `Total`을 -1로 설정합니다. 성공하면 저장 위치를 원자적으로 대체합니다. `task.Pause()`는 부분 파일을 유지하고, `task.Resume()`은 서버가 검증자(validator)를 제공하면 HTTP 범위 요청을 사용하며, `task.Cancel()`은 부분 파일을 제거합니다.

iOS는 앱이 일시 중단된 동안에도 전송을 계속하는 시스템 백그라운드 URLSession을 사용합니다. 앱은 실행 시 세션에 다시 연결하고, 백그라운드 완료를 확인하기 전에 파일을 게시합니다. macOS 전송은 프로세스 안에서 실행됩니다. Apple 플랫폼은 일시 정지된 다운로드에 네이티브 재개 데이터를 사용하며, 최종 결과가 저장될 때까지 요청 메타데이터를 앱의 비공개 저장소에 보관합니다. 재개할 수 없는 서버라면 전송을 처음부터 다시 시작합니다.

앱이 시작될 때 `OnDownloadUpdate`를 다시 등록하면 활성 작업과 저장된 결과를 볼 수 있습니다. 최종 결과를 처리한 뒤에는 `p.Task.Forget()`을 사용하세요. Android는 예를 들어 포그라운드 전송 시간이 만료되면 서비스를 중지할 수 있습니다. 그러면 앱은 `DownloadInterrupted`를 받습니다. Windows, Linux, tvOS, watchOS, 웹에서는 `Download`가 `ErrDownloadUnavailable`을 반환합니다.

## 음성 인식과 수명 주기

`cx.StartSpeech`는 `cx.Env().CanRecognizeSpeech()`가 인식기가 있다고 알려 주는 곳(macOS, iOS, Android, Windows, 그리고 `SpeechRecognition` 또는 `webkitSpeechRecognition`을 제공하는 브라우저)에서 시스템 인식기로 음성을 인식하고, 변경될 때마다 받아쓴 텍스트로 응답합니다. Apple 플랫폼에서는 `apple.info`에 `NSMicrophoneUsageDescription`과 `NSSpeechRecognitionUsageDescription`을 선언하고, Android에서는 `android.permissions`에 `RECORD_AUDIO`를 추가하세요. 헤드리스 테스트에서는 인식이 활성화되어 있는 동안 `app.Speech`가 `nsgotest.SpeechSession`을 가집니다. 결과를 전달하려면 `app.HearSpeech`와 `app.EndSpeech`를 사용하세요.

```go
session, err := cx.StartSpeech(core.SpeechOptions{
	Lang:    "en-US",
	Interim: true,
	OnResult: func(text string, final bool) {
		transcript.Set(text)
	},
})
if err == nil {
	defer session.Stop()
}
```

`cx.Env().Lifecycle()`는 앱이 활성 상태인지, 비활성 상태인지, 백그라운드에 있는지 알려 줍니다. 데스크톱에서는 `cx`가 속한 창의 상태이므로 다른 창이 보이는 동안 최소화된 창의 콘텐츠를 일시 정지할 수 있고, 앱 자체 스코프에서는 앱의 모든 창을 합친 상태입니다. `desktop.Minimized(cx)`는 그 창이 최소화되어 있는지 알려 주고, `desktop.FrontmostApp(cx)`는 앱 자신을 포함해 데스크톱에서 어떤 애플리케이션이 맨 앞에 있는지와 그 창이 전체 화면인지 알려 주므로, 데스크톱 펫이나 시간 추적기처럼 이를 따라갈 수 있습니다. `desktop.CanReadFrontmostApp(cx)`는 macOS, Windows, X11, 지원되는 Wayland 컴포지터에서 이를 따라갈 수 있는지 알려 줍니다. `cx.OnQuit`은 앱이 어떤 방식으로 종료되든 종료할 때 정리 작업을 하며, 그 작업에 200 ms가 주어집니다. `desktop.OnQuitRequest`는 데스크톱의 종료 메뉴 항목이나 `desktop.RequestQuit(cx)`를 취소하거나 연기할 수 있습니다. Windows와 Linux에서는 창을 닫으면 앱이 종료되는 경우 `OnClosing` 다음에도 실행됩니다. `desktop.RequestRestart(cx)`는 재시작 전에 같은 핸들러에 묻고, `desktop.Restart(cx)`는 여전히 즉시 재시작합니다. Linux와 Windows에서 업데이터에 의한 재실행도 핸들러에 묻기 때문에 저장 작업이 이를 취소하거나 연기할 수 있습니다. 시스템이 로그아웃, 재시작 또는 시스템 종료를 보고하면 `q.Forced()`가 true이며 앱은 `OnQuit`으로 진행합니다. 사용자에게 결정을 요청하기 전에 `Forced()`를 확인하세요. `cx.SetAppearance`는 라이트 또는 다크 모드를 강제하고, `cx.SetLocale`은 언어를 선택합니다. `cx.OnKey`는 키보드 단축키를 바인딩합니다. 이 바인딩은 `core.KeyOptions{OverModals: true}`로 유지하지 않는 한 소유자가 표시하는 것이 숨겨져 있거나 대화상자 아래에 있는 동안 쉬고, `YieldsToMenu`가 포커스가 있는 곳에서 같은 단축키를 가진 활성화된 메뉴 항목에 키를 양보하지 않는 한 애플리케이션 메뉴보다 먼저 키를 가져갑니다(텍스트 필드나 웹 뷰에 포커스가 있는 동안에는 `MenuNotEditing` 항목이 바인딩에 키를 남겨 두고 같은 단축키를 가진 이후 항목들이 그 키를 가져가지 못하게 하며, Linux의 메뉴는 Control, Alt 또는 로고 키 없이 눌린 키를 가져가지 않으므로 그곳에서는 바인딩이 그런 키를 유지합니다). 또한 `AllSurfaces`가 앱 전체 단축키를 위해 나중에 열리는 것을 포함한 앱의 모든 창과 패널에 바인딩하지 않는 한 `cx`가 속한 창에서 동작하며(이 경우 바인딩된 창에서 소유자가 표시하는 것이 숨겨져 있는 동안에는 모든 창에서 쉽니다), `core.KeyAlways` 또는 `core.KeyNotEditing`으로 바인딩하지 않는 한 포커스가 있는 컨트롤이 사용하는 키, 즉 버튼의 Return과 Space, 그리고 슬라이더, 탭 바, 라디오 그룹의 화살표 키는 그 컨트롤에 남겨 둡니다. `cx.OnOpenFiles`는 macOS, Windows, Linux, iOS에서 열린 문서를 받습니다. iOS는 일반 파일을 앱 소유의 임시 저장소로 가져오므로, 보관하려면 `cx.DataDir()`로 옮기세요. macOS에서 애플리케이션 메뉴를 제공한 창을 닫으면 남아 있는 가장 최근 창의 메뉴가 복원되고, 남은 창이 없으면 기본 메뉴가 복원됩니다.

Wayland에서 맨 앞 앱 추적에는 컴포지터의 wlr foreign-toplevel 프로토콜 버전 2 이상이 필요하며, 다른 컴포지터는 `CanReadFrontmostApp`을 false로 보고합니다. ID는 Wayland 애플리케이션 ID이고, 이름은 가능하면 데스크톱 엔트리에서 가져오며, PID는 0입니다. 패키지된 Windows 앱은 프레임 호스트 경로 대신 AUMID를 사용합니다. macOS는 창 크기로 추측하지 않고 AppKit의 전체 화면 표시 상태를 따릅니다.

애플리케이션 메뉴의 편집 역할은 `OnSelect` 대체 동작보다 포커스가 있는 편집기를 먼저 사용합니다. macOS에서 실행 취소와 다시 실행은 그 편집기의 기록을 따르고, 잘라내기는 앱이 비활성 상태일 때를 포함해 그 편집기의 선택 영역을 따릅니다. 편집기가 없으면 대체 동작이 활성화된 상태로 유지됩니다.

iOS에서는 `cx.OnKeyWith`에 `core.KeyOptions.Title` 함수를 제공하면 바인딩이 발동할 수 있는 동안 시스템의 키보드 메뉴에 단축키를 표시합니다. `Group`은 iPadOS 26 이상에서 메뉴 이름을 지정합니다. 이 함수들은 앱의 카탈로그를 읽을 수 있으므로 단축키 제목이 언어를 따릅니다. 테스트에서 현재 목록을 확인하려면 `app.KeyCommands()`를 사용하세요.

```go
cx.OnKey("mod+k", func() { palette.Set(true) })
cx.SetLocale("de")
cx.OnQuit(func(ctx context.Context) { saveDraft(ctx) })
```

`cx.Env().DeviceName()`은 사용자가 기기에 붙인 이름이고, `cx.Env().DeviceModel()`은 `Mac15,7`, `iPhone15,2`, `Google Pixel 8` 같은 하드웨어 모델이므로, 앱은 페어링된 두 기기를 구별할 수 있습니다. macOS, Android, Windows, Linux는 둘 다 제공합니다. iOS는 모델을 제공하며, 이름은 Apple의 사용자 지정 기기 이름 인타이틀먼트를 가진 앱에만 제공합니다. watchOS는 하드웨어 식별자를, HarmonyOS는 마케팅 이름 또는 제품 모델을, 미니 프로그램은 플랫폼의 브랜드와 모델을 제공하며, 이 타깃들은 사용자가 지정한 이름을 비워 둡니다. 웹은 둘 다 제공하지 않습니다. 테스트에서는 `nsgotest.Environment`로 설정합니다.

`cx.Env().ThermalState()`는 Apple 플랫폼과 Android API 29 이상에서 `core.ThermalNominal`, `ThermalFair`, `ThermalSerious` 또는 `ThermalCritical`을 보고합니다. `cx.Env().LowPowerMode()`는 Apple 플랫폼, Android, Windows, 그리고 power-profiles-daemon이 있는 Linux 세션에서 `core.LowPowerOn` 또는 `LowPowerOff`를 보고합니다. 시스템이 값을 제공하지 않으면 `ThermalUnknown`과 `LowPowerUnknown`을 반환합니다. 각 읽기는 독립적으로 변경을 따르므로, 앱은 실행 중에 작업량을 조절할 수 있습니다. 테스트에서는 `nsgotest.Environment` 또는 `app.SetEnvironment`를 통해 `core.Environment`의 `ThermalState`와 `LowPowerMode`를 설정하세요.

`desktop.CursorPoint(cx, func(p core.Point, available bool) { ... })`는 macOS, Windows, X11에서 앱의 창 밖에서도 포인터를 읽습니다. 호출마다 한 번 샘플링합니다. 좌표는 `desktop.Monitors(cx)` 및 `Window.ShowAt`과 같은 화면 좌표입니다. Wayland와 기타 지원되지 않는 타깃에서, 또는 스코프에 `core.AccessCursorPoint`가 없으면 `available`은 false입니다.

`desktop.SetWindowConstraints`는 콘텐츠 크기를 제한하고 크기 조정, 최소화, 최대화를 제어합니다. 창을 열 때 `WindowOptions.Constraints`를 설정하거나, 메인 창에는 `nsgo.WindowConstraints`를 사용하세요. Linux는 X11에서 고정되지 않은 최댓값을 적용하며, X11과 Wayland에서 GTK 네이티브 제목 표시줄의 금지된 최소화/최대화 버튼을 제거합니다. Wayland는 현재 상한에 대해서만 고정 크기를 지원합니다.

`desktop.SetWindowLevel`은 창을 다른 창들 위에 띄우고, `desktop.SetWindowWorkspaces(cx, desktop.WindowWorkspacesAll)` 또는 `WindowOptions.Workspaces`는 데스크톱 펫, HUD, 타이머처럼 창을 모든 작업 공간에 유지합니다. macOS에서는 모든 Space와 다른 앱의 전체 화면 공간 위에 표시되며 Cmd-` 순환에서 제외되고, Linux에서는 X11 창 관리자의 모든 가상 데스크톱에 표시됩니다. Windows와 Wayland에서는 창이 열린 작업 공간에 남으며, `desktop.CanSetWindowWorkspaces`는 이 설정이 적용되는지 알려 줍니다. macOS와 `_NET_WM_DESKTOP`을 지원하는 X11 창 관리자에서 true입니다. 메인 창은 그 콘텐츠에서 `desktop.SetWindowWorkspaces`를 호출하면 모든 작업 공간에 표시됩니다. 테스트에서는 `nsgotest.App.Window`가 요청을 나열하고, `nsgotest.Environment`가 `CanSetWindowWorkspaces`를 설정합니다.

`desktop.RegisterHotKey`는 macOS, Linux, 그리고 `nsgo.Run(App)`으로 실행한 Windows 앱에서 어떤 앱이 맨 앞에 있든 동작하는 전역 단축키를 바인딩합니다. 핸들의 `State`는 시스템이 이를 받아들였는지 알려 줍니다. 시스템이 응답할 때까지는 `HotKeyPending`이고, 그 뒤에는 `HotKeyRegistered`, 다른 등록이 그 조합을 가지고 있으면 `HotKeyTaken`, 전역 단축키를 지원하지 않는 키나 플랫폼이면 `HotKeyUnsupported`가 되므로, 단축키 녹화기는 다른 조합을 요청할 수 있습니다. macOS에서는 앱이 이미 그 조합을 가지고 있거나 다른 앱이 그것을 독점적으로 등록한 경우에만 사용 중인 것으로 간주합니다. Wayland에서는 컴포지터가 사용자에게 단축키를 바인딩하도록 요청하며, 사용자가 바인딩하면 등록되고, 사용자나 컴포지터가 제외하면 사용 중이 됩니다. 테스트에서는 `nsgotest.HotKeyTaken`과 `nsgotest.HotKeyUnsupported`가 조합이 그렇게 응답하도록 만듭니다.

`desktop.TitleBarHidden`을 사용하면 Windows와 Linux는 Alt만 눌렀다 떼거나 F10을 누를 때 애플리케이션 메뉴를 팝업으로 제공합니다. Alt와 최상위 레이블의 첫 글자나 숫자를 함께 누르면 레이블의 문자 체계에서 해당 하위 메뉴가 열리거나 해당 명령이 선택됩니다. 앱 바인딩은 자신의 키를 유지합니다. Shift+F10은 여전히 컨텍스트 메뉴 키이며, 다른 Alt 조합은 시스템에 남습니다. Escape는 팝업을 닫고 키보드 포커스를 복원합니다.

```go
shortcut := desktop.RegisterHotKey(cx, "alt+space", launcher.Toggle)
warning := core.Text{ContentFunc: func() string {
	if shortcut.State() == desktop.HotKeyTaken {
		return "Another app uses Alt+Space"
	}
	return ""
}}
```

macOS, Windows, Linux의 데스크톱 크기 조정은 그리기 전에 새 레이아웃을 최대 16 ms까지 기다립니다. 레이아웃이 이 기한을 맞출 수 있도록 반응형 계산과 이벤트 핸들러를 빠르게 유지하세요. 느린 I/O는 고루틴에서 수행하고 결과는 `cx.Post`로 게시하세요. 바쁜 앱은 크기 조정을 계속하다가 따라잡으면 레이아웃을 표시합니다.

`desktop.SetWindowFrame`은 `WindowFrame.Move`가 true이면 메인 화면의 왼쪽 위를 기준으로 한 포인트 단위의 `X`와 `Y`를 사용해 창을 이동합니다. 설정하지 않은 치수는 현재 크기를 유지합니다. `desktop.CanPlaceWindows`를 확인하세요. macOS, Windows, X11의 Linux는 위치 지정을 지원하며, 일반 Wayland 최상위 창은 컴포지터가 정한 위치를 유지합니다. 이 데스크톱들에서 `desktop.Frame`은 네이티브 창의 위치를 따릅니다.

macOS와 Windows에서 `WindowFrame{Animated: true}`는 보이는 패널을 새 프레임으로 부드럽게 전환하며, 도킹된 패널의 크기도 포함됩니다. 숨겨진 패널과 동작 줄이기 설정에서는 새 프레임이 즉시 적용됩니다. 일반 macOS 창은 AppKit의 애니메이션을 사용합니다.

`desktop.WindowOptions.Parent`를 앱의 열린 창으로 설정하면 부모 창이 닫힐 때 둘 다 닫힙니다. 데스크톱에서 자식 창은 부모 위에 머물고, 부모가 숨겨지거나 최소화되면 따라갑니다. 명시적으로 숨긴 자식 창은 부모가 돌아와도 숨겨진 상태로 유지됩니다. macOS에서는 `Frame`이 설정되지 않은 경우 부모 위 가운데에 배치되며, 부모를 따라 전체 화면으로 전환됩니다.

`WindowOptions.NoShadow`와 `desktop.SetWindowShadow(cx, false)`는 제목 표시줄이 숨겨져 자체 크롬을 그리는 Windows 창의 그림자도 제거합니다. 시스템 제목 표시줄이 있는 창은 테두리와 그림자를 유지하며, 창 관리자가 장식하는 Linux 창도 마찬가지입니다. Linux에서 `NoShadow`는 GTK 자체 장식의 그림자만 제거합니다.

`desktop.SetWindowPointerEvents`는 포인터가 창을 통과해 그 아래에 있는 어떤 앱의 창에든 전달되게 합니다. `WindowPointerNone`은 헤드업 디스플레이처럼 모든 곳에서, `WindowPointerPainted`는 뷰가 아무것도 그리지 않거나 포인터를 받지 않는 곳에서 통과시키므로, 자기 자신에 대한 누르기, 드래그, 오른쪽 클릭은 계속 받는 데스크톱 펫의 주변 같은 경우에 사용합니다. 시스템이 그리는 제목 표시줄은 아무것도 그려지지 않은 것으로 간주됩니다. `WindowOptions.PointerEvents`와 `PanelOptions.PointerEvents`는 창을 그런 상태로 엽니다. `Desktop.WindowDrag` 영역은 누를 수 있는 조상 안에서도 여전히 드래그되며, 더 가까이에 있는 누를 수 있는 컨트롤이나 편집 컨트롤은 창을 드래그하는 대신 자체 입력을 유지합니다. 포인터를 받지 않는 창은 누르기를 받지 못하므로, 앱은 단축키, 트레이 아이콘 또는 다른 창에서 포인터를 되돌려 줍니다. `Access`가 보유한 코드에는 `core.AccessOverlay`가 필요합니다.

```go
desktop.OpenPanel(cx, desktop.PanelOptions{Width: 600, Height: 400, PointerEvents: desktop.WindowPointerPainted}, Pet)
```

macOS와 Windows에서 painted 창 안의 이미지는 `Image.HitTestAlpha`(TSX `hitTestAlpha`)를 0보다 크고 1 이하인 값으로 설정할 수 있습니다. 예를 들어 0.5는 불투명도가 절반 이상인 그림 픽셀에서 포인터를 받습니다. 이 임계값은 이미지의 `OnPress`에도 적용됩니다. 뷰의 배경이나 테두리는 여전히 그려진 것으로 간주됩니다. 0은 이미지 뷰 전체를 사용하며, Linux의 이미지도 마찬가지입니다.

painted macOS 창에서 `Video.HitTestAlpha`(TSX `hitTestAlpha`)는 디코딩된 비디오 프레임에 같은 임계값을 적용합니다. `Controls`가 꺼져 있고 변환되지 않은 비디오 트랙, 예를 들어 데스크톱 펫을 위한 알파 채널이 있는 HEVC에서 동작합니다. 0, 사용할 수 없는 프레임, 그리고 다른 데스크톱 호스트에서는 비디오 뷰의 경계를 사용합니다.

`desktop.NewTray`는 macOS와 Windows에서 `TrayOptions.View`로 실시간 아이콘을 그릴 수 있습니다. Windows는 뷰의 투명 렌더링을 알림 영역의 아이콘 크기에 맞추고, 뷰가 바뀌거나 이미지 로드가 끝나면 갱신합니다. 동작에는 `TrayOptions.OnClick`, `Panel`, `Menu`를 사용하세요. Linux는 현재 PNG 아이콘을 표시하며, 트레이 뷰를 그리는 것은 아직 목표로 남아 있습니다.

`desktop.OpenPanel`은 그런 단축키나 트레이 아이콘이 표시하는 패널, 즉 다른 창들 위에 있는 테두리 없는 창을 엽니다. `desktop.OpenWindow`, `desktop.NewTray`, `mobile.OpenOverlay`, `audio.NewPlayer`, `audio.LoadSound`, `core.NewHolder`와 마찬가지로, 이것은 실행 중인 핸들러의 소유자가 아니라 전달한 `cx`에 직접 속하므로, 버튼의 `OnPress`에서 앱의 스코프를 전달하면 그 버튼의 스코프가 사라진 뒤에도 창, 패널 또는 플레이어가 살아 있습니다. 반면 반응형 기본 요소(`Effect`, `Cleanup`, `AfterFunc`, `OnKey`)는 실행 중인 소유자에 속하며, 이 소유자는 `cx.Run`이 전환합니다. 팝업 카드는 `Transparent`를 지정한 패널 위에 스스로를 그립니다. 이 패널에는 자체 배경, 테두리, 그림자가 없으므로 카드의 둥근 모서리와 `BoxShadow`만 보이며, 패널 안에 그림자를 위한 공간이 있어야 합니다. `FitContent`는 패널이 처음 표시되기 전과 콘텐츠가 바뀔 때 패널 크기를 `Width`와 `Height`까지 콘텐츠에 맞춥니다. `desktop.Transparent`는 투명 창과 마찬가지로 패널이 지금 자체적으로 아무것도 그리지 않는지 알려 주며, `desktop.CanMakeWindowsTransparent`는 창이 투명해질 수 있는지만 알려 줍니다. 이 값이 false인 경우, 예를 들어 컴포지터가 없는 Linux에서는 콘텐츠에 배경을 지정하세요.

macOS에서 `Transparent`가 없는 패널은 어느 한 치수가 24포인트보다 작으면 각진 모서리를 사용하고, 일반 크기에서는 둥근 모서리를 복원합니다.

```go
lookup := desktop.OpenPanel(cx, desktop.PanelOptions{Width: 380, Height: 520, Transparent: true, FitContent: true, HideOnBlur: true}, Card)
desktop.RegisterHotKey(cx, "alt+space", lookup.Toggle)
```

`desktop.NewTray`는 macOS에서 이미지나 그려진 뷰 옆에 `TrayOptions.Title`을 표시합니다. 사용자 지정 팝업의 경우 `OnClick` 또는 `Menu` 안에서 `tray.ClickBounds()`를 읽고, 사용 가능한 사각형을 `panel.ShowAt(bounds, core.PlaceBottom)`에 전달하세요. 이 값은 `Env.Displays`와 같은 화면 포인트를 사용하며 제목을 포함합니다. 첫 클릭 전, 제거된 후, Linux에서, 그리고 Windows 셸이 제공할 수 없는 경우에는 경계를 사용할 수 없습니다. 클릭할 때마다 이전 경계가 대체되므로, 사용할 수 없는 결과가 이전 사각형을 재사용하는 일은 없습니다. 대신 `TrayOptions.Panel`을 설정하면 내장된 클릭 시 토글 동작이 유지됩니다.

`PanelOptions.Dock`은 `Show`, `Focus`, `Toggle`, 콘텐츠 크기 조정 중에도 패널을 디스플레이의 사용 가능한 가장자리를 따라 가운데에 유지합니다. `Edge`를 `core.PlaceTop`, `PlaceBottom`, `PlaceLeft` 또는 `PlaceRight`로, `Display`를 `Env.Displays` ID(기본 디스플레이는 빈 값)로, `Margin`을 해당 가장자리로부터의 포인트 단위 거리로 설정하세요. 도킹된 패널은 `ShowAt`이나 `MoveTo`를 사용할 수 없으며, `SetWindowFrame`은 크기만 변경합니다. 트레이 아이콘은 패널을 아이콘 아래가 아니라 도킹 위치에 표시합니다.

```go
panel := desktop.OpenPanel(cx, desktop.PanelOptions{
	Width: 320, Height: 80, NoActivate: true,
	Dock: desktop.PanelDock{Edge: core.PlaceBottom, Margin: 12},
}, Candidates)
panel.OnPlacementError(cx, func(err error) { /* offer another presentation */ })
panel.Show()
```

`desktop.CanDockPanels(cx)`는 임의의 창 배치와 별도로 도킹 지원 여부를 보고합니다. macOS, Windows, X11은 이를 지원하며, Wayland에는 선택적 런타임 라이브러리 `libgtk4-layer-shell.so.0`과 `zwlr_layer_shell_v1`을 구현한 컴포지터가 필요합니다. NSGo는 GTK보다 먼저 이 라이브러리를 로드합니다. 이미 Wayland를 로드한 임베딩 호스트는 호스트를 시작하기 전에 gtk4-layer-shell을 미리 로드해야 합니다. 라이브러리, 프로토콜 또는 선택한 디스플레이가 없으면 패널은 숨겨진 상태로 남고 `OnPlacementError`가 호출됩니다. Wayland 도킹은 작업 공간을 예약하지 않고 top 레이어를 사용합니다. `NoActivate`는 키보드를 현재 창에 남겨 두며, 이것이 없는 패널은 숨겨질 때까지 독점적인 키보드 포커스를 가져갑니다. 일반 Wayland 창은 평소의 컴포지터 배치를 유지합니다.

단축키 녹화기는 사용자가 그런 키를 선택할 수 있게 합니다. `Focusable` 뷰의 `OnKeyDown`은 포커스를 가진 동안 눌리는 각 키를 앱의 바인딩보다 먼저, 누르고 있는 수정자 키와 함께 받으며, 수정자 키만 단독으로 누른 경우도 포함합니다. `true`를 반환하면 키가 바인딩에 전달되지 않으므로 Command+S를 녹화해도 저장되지 않습니다. `KeyEvent.Shortcut`은 누른 키를 `OnKey`와 `RegisterHotKey`가 받는 형식으로 각 수정자 키를 이름으로 써서 나타내고("ctrl+shift+k", "alt+space", Command+Shift+K는 "shift+meta+k"), 수정자 키 단독이면 ""입니다. `OnKeyUp`은 `OnKeyDown`이 키를 소비했거나 포커스가 이동한 뒤에도, 수정자 키를 여전히 누른 상태로, 그 키들이 올라오는 것을 각각 받습니다. 둘 다 macOS, Windows, Linux, 웹(브라우저는 페이지에 넘기지 않는 단축키를 유지합니다), Apple TV, 그리고 `nsgotest`에서 동작합니다. Android는 하드웨어 키보드 이벤트를 포커스된 뷰에 비동기로 전달하므로 `OnKeyDown`의 반환값으로 키를 소비할 수 없습니다. iPhone과 iPad는 하드웨어 키 폴링을 지원합니다. 앱이 전역으로 등록한 단축키는 뷰보다 먼저 키를 가져가므로, 녹화기는 녹화하는 동안 그 단축키를 등록 해제합니다.

```go
core.View{
	Focusable: func() bool { return true },
	OnKeyDown: func(e core.KeyEvent) bool {
		if s := e.Shortcut(); s != "" {
			shortcut.Set(s)
		}
		return true
	},
	OnKeyUp: func(core.KeyEvent) { recording.Set(false) },
	Children: []core.Node{core.Text{ContentFunc: shortcut.Get}},
}
```

게임이나 캔버스는 대신 키를 폴링합니다. `core.KeysDown(cx)`는 창에서 눌려 있는 키를 나열하고, `core.KeyHeld(cx, "left")`는 특정 키가 눌려 있는지 알려 줍니다. 이들은 추적되는 읽기이며, `RequestAnimationFrame` 루프에서도 읽을 수 있습니다. 첫 번째 읽기부터 플랫폼이 모든 키를 보고하게 되므로, 이미 눌려 있던 키는 다시 누를 때까지 알 수 없습니다. 필드나 웹 뷰에 입력된 키, 또는 입력기가 조합 중일 때의 키는 절대 눌린 것으로 간주되지 않으며, 창이 포커스를 잃을 때 여전히 눌려 있는 키는 그 시점에 올라온 것으로 처리됩니다. 이들은 `OnKeyUp`이 동작하는 곳과 하드웨어 키보드가 연결된 iPhone 및 iPad에서 동작합니다.

마우스 버튼의 경우 `View.OnMouseDown`과 `View.OnMouseUp`은 macOS, Windows, Linux, 웹, `nsgotest`에서 모든 버튼(`core.MouseButtonLeft`, `core.MouseButtonRight`, `core.MouseButtonMiddle`, `core.MouseButtonBack`, `core.MouseButtonForward`)에 대해 `core.MouseButtonEvent`를 전달하며, 뷰를 키보드 포커스 정지 지점으로 만들지 않습니다. `core.MouseButtonsDown(cx)`와 `core.MouseButtonHeld(cx, core.MouseButtonRight)`는 창에서 눌려 있는 버튼에 대한 추적되는 읽기이며, 창이 포커스를 잃으면 자동으로 지워집니다.

물리적 위치에 연결된 컨트롤에는 `KeyEvent.Code`, `core.KeyCodesDown(cx)` 또는 `core.KeyCodeHeld(cx, "KeyW")`를 읽으세요. 코드는 `KeyboardEvent.code` 이름을 사용합니다. AZERTY에서 QWERTY의 W 위치는 `Code: "KeyW"`, `Key: "z"`입니다. `core.KeyCodeLabel(cx, "KeyW")`는 안내 문구에 쓸 활성 레이아웃의 표시 레이블을 제공합니다. iOS, Android, HarmonyOS, 그리고 레이아웃 맵이 없는 브라우저에서는 그 위치에서 마지막으로 보고된 키 이름을 사용하며, 키가 관찰될 때까지 빈 문자열을 반환합니다. HarmonyOS는 ArkUI가 스캔 코드를 보고하지 않기 때문에 위치와 키 모두 InputKit의 `KeyCode`에서 이름을 가져오므로, 그곳에서는 키 이름이 아직 QWERTY가 아닌 레이아웃을 따르지 않습니다.

```go
var loop func(time.Time)
loop = func(time.Time) {
	if core.KeyHeld(cx, "left") {
		x.Update(func(x float32) float32 { return x - 4 })
	}
	if core.KeyHeld(cx, "right") {
		x.Update(func(x float32) float32 { return x + 4 })
	}
	cx.RequestAnimationFrame(loop)
}
cx.RequestAnimationFrame(loop)
```

## 화면 켜짐 유지

`cx.KeepAwake("Playing video", core.KeepAwakeOptions{Display: true})`는 반환된 해제 함수가 실행되거나 현재 스코프가 해제될 때까지 화면을 켜진 상태로 유지합니다. Android에서는 앱이 보이는 동안 유지가 적용됩니다. 웹에서는 Screen Wake Lock을 지원하는 보안 컨텍스트에서 동작하며, 페이지가 다시 보이게 되면 잠금을 다시 요청합니다. Safari는 첫 요청을 허용하기 전에 누르기나 키 입력을 요구할 수 있습니다. 컴퓨터만 깨어 있게 하는 유지는 이 두 플랫폼에서 효과가 없습니다.

## 포인터 잠금

게임 카메라나 화면 가장자리에서도 계속되어야 하는 드래그에서는 ref를 가진 뷰를 누를 때 `ref.LockPointer()`를 호출하세요. 포인터는 제자리에 머물고 숨겨지며, 뷰는 `OnPointerDelta(dx, dy)`로 상대 이동을 받습니다. 양수 값은 오른쪽과 아래쪽으로의 이동입니다.

잠금을 해제하려면 반환된 함수를 호출하세요. Escape, 창의 포커스 상실, 뷰를 숨기거나 제거하는 것으로도 해제됩니다. 서피스가 이를 지원하는지는 `cx.Env().CanLockPointer()`로 확인하세요. 웹에서는 누르기 같은 사용자 제스처가 필요합니다. Wayland의 Linux에서는 포인터 제약과 상대 이동을 지원하는 컴포지터가 필요합니다.

## 사운드 제한

엔진의 제한을 변경하려면 사운드를 로드하기 전에 `audio.SetSoundLimits(cx, audio.SoundLimits{MaxSounds: 200, MaxDuration: 20 * time.Second})`를 사용하세요. `MaxEncodedBytes`, `MaxDecodedBytes`, `MaxPlaybacks`도 설정할 수 있습니다. 0인 필드는 기본값을 사용합니다. 기본값은 사운드당 인코딩 8 MiB와 5초, 소스 64개, 전체 사운드 합계 디코딩 16 MiB, 출력당 겹쳐 재생 32개입니다. 기존 사운드는 로드된 상태로 남으며 새 제한에 포함됩니다. 제한된 스코프는 엔진 예산 내에서 기본 제한을 유지합니다. 서명된 TSX 패키지의 `nsgo/audio` 사운드는 Go 앱이 무엇을 설정했든 기본 제한에 걸리는 반면, 앱 자체의 TSX는 Go 앱의 제한을 따릅니다. Android의 SoundPool은 제한과 관계없이 한 번에 최대 32개의 사운드를 재생하며 각 사운드에서 최대 1 MiB를 디코딩합니다.

## 전체 화면

`desktop.SetFullScreen(cx, true, displayID)`는 macOS, Windows, Linux에서 `cx.Env().Displays()`가 지정한 디스플레이를 채웁니다. ID를 생략하면 창의 현재 디스플레이를 사용합니다. 알 수 없는 ID는 창을 그 자리에 둡니다. 같은 요청을 반복해도 효과가 없습니다. `desktop.SetFullScreen(cx, false)`는 전체 화면을 종료하고 창의 이전 프레임을 복원합니다. 웹에서는 누르기 핸들러에서 진입하세요. 디스플레이는 브라우저가 선택하며, `cx.Env().FullScreen()`은 Esc를 포함한 진입과 종료를 따릅니다.

## 디스플레이

`cx.Env().Display()`는 창의 현재 `core.DisplayInfo`를 반환하며, 창이 다른 디스플레이로 이동하면 바인딩을 갱신합니다. `cx.Env().Displays()`는 플랫폼이 보고하는 디스플레이를 나열합니다. `RefreshRate`와 `MaxRefreshRate`는 Hz 단위이며, 알 수 없는 주사율은 0입니다. iOS는 패널의 최대 주사율을 보고합니다. HarmonyOS는 창의 디스플레이와 그 현재 주사율, 지원되는 최고 주사율을 보고합니다. 웹은 현재 화면을 보고하며, 프레임 콜백으로 측정한 값에는 `RefreshRateEstimated`를 표시합니다. 테스트에서는 `nsgotest.Environment`를 통해 `Displays`와 `DisplayID`를 설정하세요.

## 텍스트 읽어 주기

`cx.Speak`는 `cx.Env().CanSpeak()`가 플랫폼이 말할 수 있다고 알려 주는 곳, 즉 macOS, iOS, tvOS, Android, Windows, 웹, 그리고 speech-dispatcher가 있는 Linux에서 해당 언어의 시스템 음성으로 텍스트를 소리 내어 읽습니다. 한 번에 하나의 발화만 말합니다. 다른 `Speak`는 말하고 있던 발화를 끊으며, `cx.StopSpeaking`과 그 발화를 말한 스코프의 종료도 마찬가지입니다. `done`은 발화가 어떻게 끝났는지 한 번 받습니다. 완료, 중지, 또는 `core.SpeakError`로 인한 실패이며, 예를 들어 해당 언어를 말하는 음성이 설치되어 있지 않으면 `core.SpeakLanguageUnavailable`입니다.

```go
cx.Speak("Guten Morgen", core.Voice{Lang: "de-DE", Rate: 0.8}, func(stopped bool, err error) {
	speaking.Set(false)
})
```

`Rate`와 `Pitch`는 0.5부터 2까지이며, 1은 음성 고유의 값입니다. `cx.Voices("ja", ...)`는 언어에 대해 설치된 음성을 나열하므로, 앱은 사용자에게 없는 음성을 설치하도록 요청하거나 음성을 고르게 할 수 있으며, 고른 음성의 `ID`를 `core.Voice`에 넣습니다. 브라우저는 사용자가 페이지에서 무언가를 누른 뒤에만 말하며, 그 전에는 `core.SpeakNotAllowed`로 실패합니다. TSX 패키지의 Go 컴포넌트는 `text_to_speech` 기능(capability)이 있어야만 말합니다.

## 오디오 반복 재생

`audio.Voice.Loop`를 설정하면 `Stop` 또는 `Close`까지 사운드 재생을 반복합니다. 스트리밍 음악의 경우 `audio.PlayerOptions.LoopStart`를 설정하고 `SetLoop(true)`를 호출하세요. 첫 재생에는 인트로가 포함됩니다. 이후 재생은 오프셋에서 시작합니다. 오프셋이 0이면 트랙 전체를 반복합니다.

macOS 14 및 iOS 17 이상에서 길이가 유한한 트랙은 네이티브 큐를 사용해 현재 재생이 끝나기 전에 다음 반복을 준비합니다. 이전 Apple 버전은 현재 재생이 끝날 때 루프 시작 지점으로 탐색하므로 짧은 공백이 생깁니다. Linux와 웹에서 반복 재생 플레이어는 최대 32 MiB, 5분 길이의 트랙을 메모리로 디코딩하고(분당 약 23 MB) 공백 없이 반복합니다. Linux에서는 더 크거나 긴 트랙도 자체 디코더를 통해 스트리밍되며 마찬가지로 공백 없이 반복합니다. 웹에서는 끝에서 다시 탐색하므로 짧은 공백이 생깁니다. Windows와 Android에서 시작 지점이 있는 루프는 끝에서 그 지점으로 다시 탐색하므로 짧은 공백이 생깁니다. 웹에서 재생 속도가 1이 아닌 반복 재생 플레이어도 끝에서 다시 탐색합니다. 브라우저의 공백 없는 재생은 속도에 따라 음높이를 바꾸기 때문입니다. 웹에서 반복 재생 플레이어는 사운드와 마찬가지로 사용자가 페이지에서 무언가를 누른 뒤에만 재생되며, 그 전에는 `NotAllowed`로 실패합니다.

## 생체 인증

`cx.BiometricAvailability`는 Face ID, Touch ID 또는 기기 지문이 등록되어 있는지 보고합니다. Android는 어떤 종류가 등록되어 있는지 알려 주지 않으므로, 그곳에서는 지문이 등록되어 있으면 지문을, 그렇지 않고 기기에 얼굴 인식 하드웨어가 있으면 얼굴을 보고합니다. `cx.AuthenticateBiometric`은 비어 있으면 안 되는 사유와 함께 시스템에 사용자 확인을 요청하고, 성공, 실패, 취소 또는 사용 불가를 보고합니다. 앱을 위해 암호 대체 수단을 선택하지는 않습니다.

```go
cx.BiometricAvailability(func(kind core.BiometricKind) {
	if kind != core.BiometricNone {
		cx.AuthenticateBiometric("Confirm deleting this account", func(result core.BiometricResult) {
			if result == core.BiometricSuccess {
				deleteAccount()
			}
		})
	}
})
```

Face ID 앱은 `apple.info` 아래에 `NSFaceIDUsageDescription`을 선언하고, Android 앱은 `android.permissions` 아래에 `USE_BIOMETRIC`을 나열합니다.

## 로그인

`cx.AuthSession`은 웹 뷰가 아닌 시스템의 인증 세션에서 OAuth 제공자로 사용자를 로그인시킵니다. macOS, iOS, watchOS에서는 `ASWebAuthenticationSession`, Android에서는 Custom Tab 또는 브라우저, Windows, Linux, 그리고 스킴이 없는 Mac에서는 기본 브라우저, 웹에서는 팝업을 사용합니다. 웹에서는 리디렉션 페이지가 같은 오리진의 `BroadcastChannel`로도 콜백을 보내므로, 제공자의 `Cross-Origin-Opener-Policy`가 팝업과 페이지의 연결을 끊어도 로그인이 완료됩니다. 그런 팝업은 페이지에서 닫힌 것으로 보이기 때문에, 제공자의 페이지를 표시한 뒤 팝업을 닫으면 5분이 지나야 세션이 취소로 끝납니다. 세션에 취소 버튼을 제공하세요. 프로토콜은 앱이 담당합니다. 앱은 PKCE 검증자(verifier)와 state를 만들고, 전달받은 리디렉션에 맞는 인증 URL을 구성하며, 코드를 교환하기 전에 콜백을 확인합니다. 취소는 실패가 아닙니다. `errors.Is(err, core.AuthCancelled)`는 사용자가 세션을 닫았거나 반환된 `cancel`이 세션을 끝냈음을 알려 줍니다.

```go
verifier, challenge, state := newPKCE() // the app's own
cx.AuthSession(func(redirect string) string {
	q := url.Values{"client_id": {clientID(redirect)}, "redirect_uri": {redirect}, "response_type": {"code"}, "scope": {"openid email"},
		"state": {state}, "code_challenge": {challenge}, "code_challenge_method": {"S256"}}
	return "https://accounts.example.com/authorize?" + q.Encode()
}, core.AuthSessionOptions{Scheme: "com.example.app", Path: "/oauth2redirect", Timeout: 5 * time.Minute}, func(callback string, err error) {
	if errors.Is(err, core.AuthCancelled) {
		return
	}
	if err != nil {
		showError(err)
		return
	}
	u, _ := url.Parse(callback)
	if u.Query().Get("state") == state {
		exchangeCode(u.Query().Get("code"), verifier)
	}
})
```

리디렉션은 스킴이 설정된 경우 `com.example.app:/oauth2redirect`이고(Windows, Linux, Android에서는 OS가 콜백을 실행 중인 앱에 전달하도록 `nsgo.yaml`의 `url_schemes`에 스킴이 있어야 합니다. macOS, iOS, watchOS의 시스템 세션은 그 선언 없이도 콜백을 받습니다), 데스크톱의 루프백에서는 `http://127.0.0.1:<port>/oauth2redirect`이므로, 둘 다 제공자에 등록하세요. 종종 각각 별도의 클라이언트로 등록합니다. 루프백만 사용하려면 `Scheme`을 비워 두고, 앱의 연결된 도메인(associated domain)을 사용하려면 `Host`와 함께 `https`로 설정하세요. 이는 iOS 17.4, macOS 14.4, watchOS 10.4가 받습니다. watchOS는 스킴이나 연결된 HTTPS 도메인이 필요합니다. 루프백 대체 수단이 없으며, NSGo의 기존 watchOS 26 최소 버전이 여전히 적용됩니다. 닫힌 브라우저 탭은 루프백 세션에 아무것도 알려 주지 않으므로 `Timeout`이나 취소 버튼을 제공하세요. `Ephemeral`은 macOS, iOS, watchOS에 브라우저의 쿠키를 공유하지 말도록 요청하고, Android에서는 브라우저가 지원하는 경우 비공개 Custom Tab을 요청합니다. Android에서 쿠키 격리를 가정하지 마세요. tvOS와 미니 프로그램은 현재 `core.AuthUnavailable`로 응답합니다. TSX에서 `AuthSession.start(url, options)`는 `auth_session` 기능(capability)이 필요하며, `result`가 콜백으로 resolve되거나 취소되면 null로 resolve되는 세션을 제공합니다.

Android는 일치하는 콜백을 일반 `OnOpenLinks`보다 먼저 소비하며, 콜백 없이 돌아오면 세션이 취소로 끝납니다. 콜백은 앱이 소유한 비공개 스킴을 사용해야 합니다. 빈 스킴, HTTPS 콜백 또는 선언되지 않은 스킴은 `core.AuthUnavailable`을 반환하며, 로그인 세션 이전에 빌드된 Android 런타임도 마찬가지입니다.

브라우저 세션은 일치하는 콜백 링크만 소비하며, 관련 없는 링크나 늦게 도착한 링크는 여전히 `OnOpenLinks`에 도달합니다. 같은 콜백 URI로 다른 브라우저 세션을 시작하면 이전 세션이 취소됩니다. 서로 다른 URI는 다른 창에서 대기할 수 있습니다.

인증 URL 함수에 전달된 리디렉션을 그대로 사용하세요. 웹과 데스크톱 루프백 리디렉션은 함수가 받기 전에 유니코드 경로를 인코딩하고 점(dot) 세그먼트를 정리합니다. 루프백 `Path`는 디코딩된 텍스트이므로 리터럴 `%`도 이스케이프됩니다.

미니 프로그램은 대신 `cx.SignInMiniProgram(core.MiniProgramSignInOptions{}, done)`을 사용합니다. 콜백은 `Platform`(`MiniProgramWeChat`, `MiniProgramQQ`, `MiniProgramDouyin` 또는 `MiniProgramAlipay`)과 비어 있지 않고 수명이 짧은 `Code`를 가진 `core.MiniProgramCredential`을 받습니다. 호스트의 코드 교환을 위해 이를 서버로 보내고, 앱 시크릿은 서버에 보관하세요. 이는 프로필 정보를 요청하지 않는 `wx.login`/`qq.login`/`tt.login`, 또는 Alipay의 기본 `auth_base` 인증입니다. URL을 열거나 OAuth URL 빌더를 실행하지 않습니다.

`cx.Env().CanSignInMiniProgram()`은 호스트 API의 사용 가능 여부를 보고합니다. 다른 타깃은 `core.MiniProgramSignInUnavailable`로 응답하고, `core.AccessMiniProgramSignIn`이 없는 코드는 `AccessDenied`를 받습니다. Douyin은 기본적으로 조용히 요청합니다. 로그인하지 않은 호스트는 `MiniProgramSignInNotSignedIn`을 반환하며, `MiniProgramSignInOptions{Prompt: true}`는 호스트 계정 로그인 창을 허용합니다. 그 창을 닫으면 `MiniProgramSignInCancelled`가 반환됩니다. 익명 기기 코드는 절대 로그인한 사용자로 간주되지 않습니다. 테스트는 `nsgotest.Environment(core.Environment{MiniProgramSignIn: true})`와 `app.MiniProgramSignIn`을 설정하며, `app.MiniProgramSignIns`가 요청을 기록합니다. TSX는 미니 프로그램에서 실행되지 않습니다.

사용자가 이미 다른 곳(예: 자신의 브라우저)에서 로그인한 상태로 웹 뷰를 로그인시켜야 하는 앱은 `cx.SetWebCookies`로 그 로그인의 쿠키를 쓴 다음 페이지를 로드합니다. 여기에는 페이지 스크립트가 설정할 수 없는 HttpOnly 쿠키도 포함되며, done은 저장소가 쿠키를 보유하게 되면 실행됩니다. 저장소는 앱의 모든 웹 뷰가 공유하는 영구 저장소이므로 `Expires`가 있는 쿠키는 앱보다 오래 남습니다. 시크릿(Incognito) 뷰는 이 쿠키를 보지 못하지만, 앱마다 저장소를 하나만 유지하는 Android는 예외입니다. HarmonyOS, 웹, tvOS, watchOS는 `Unavailable`로 응답합니다.

```go
cx.SetWebCookies([]core.WebCookie{
	{Name: "SID", Value: sid, Domain: ".example.com", Expires: expires, Secure: true, HTTPOnly: true},
}, func(err error) {
	if err == nil {
		page.Set(core.WebSrc("https://www.example.com/"))
	}
})
```

쿠키 하나를 제거하려면 같은 이름, 도메인, 경로와 과거의 `Expires`로 다시 쓰세요. 만료 시간이 0이면 세션 쿠키가 됩니다. 제거를 반영해야 하는 페이지를 로드하기 전에 콜백을 기다리세요.

반대로 `cx.WebCookies`는 사이트의 페이지가 표시되어 있는지와 관계없이 저장소의 쿠키를 나열하며, `document.cookie`가 읽을 수 없는 HttpOnly 쿠키와 그 도메인, 경로, 만료 시간, 플래그도 포함합니다. `core.WebCookieFilter`는 `URL`로의 요청이 보낼 쿠키, 또는 `Domain`과 그 아래 모든 호스트의 쿠키를 고르며, 빈 필터는 모든 쿠키를 고릅니다. 사용자의 로그인으로 Go에서 사이트를 호출하거나 사용자가 어디에 로그인되어 있는지 보여 주는 앱은 이렇게 쿠키를 읽습니다. `cx.ClearWebsiteData`는 모든 사이트에서 사용자를 로그아웃시킵니다. 저장소의 쿠키, 로컬 및 세션 저장소, IndexedDB, 서비스 워커, 캐시를 제거하며, done은 이들이 모두 사라지면 실행됩니다. Android의 `CookieManager`는 URL이 보낼 이름과 값만 응답하므로, 그곳에서는 `URL` 필터만 목록을 반환하며 `Domain`에 호스트를 채우고 다른 필드는 비워 둡니다. `Domain` 필터나 빈 필터는 `Unavailable`로 응답하며, 그곳에서의 지우기는 HTTP 캐시를 남겨 둡니다. HarmonyOS, 웹, tvOS, watchOS는 둘 다 `Unavailable`로 응답합니다.

```go
cx.WebCookies(core.WebCookieFilter{Domain: "example.com"}, func(cookies []core.WebCookie, err error) {
	signedIn.Set(err == nil && slices.ContainsFunc(cookies, func(c core.WebCookie) bool { return c.Name == "SID" }))
})
```

## Apple로 로그인

`cx.SignInWithApple`은 `cx.Env().CanSignInWithApple()`이 true인 macOS, iOS, tvOS에서 앱 자체 버튼으로부터 시스템의 Apple로 로그인 시트를 표시합니다. 자격 증명으로 한 번 응답하거나 오류로 응답합니다. 사용자가 시트를 닫으면 실패가 아닌 `core.AppleSignInCancelled`이고, 그렇지 않으면 `AppleSignInFailed`, 다른 플랫폼에서는 `AppleSignInUnavailable`입니다. Apple은 사용자가 앱에 처음 로그인할 때만 이메일과 이름을 제공하므로 그때 보관하세요. `Nonce`는 소문자 16진수로 된 SHA-256 값으로 요청에 들어가며, 이 값은 아이덴티티 토큰의 `nonce` 클레임에 담깁니다. 서버는 앱에 준 nonce를 해시해 비교합니다. `cx.AppleCredentialState`는 사용자가 그 이후에 자격 증명을 취소했는지 알려 줍니다. 헤드리스 테스트에서는 `app.AppleSignIn`이 `nsgotest.AppleSignInRequest`를 받고 자격 증명이나 오류를 제공합니다.

```go
nonce := rand.Text() // also sent to the server, which checks the token's claim against its SHA-256
cx.SignInWithApple(core.AppleSignInOptions{FullName: true, Email: true, Nonce: nonce}, func(c core.AppleCredential, err error) {
	switch {
	case errors.Is(err, core.AppleSignInCancelled):
	case err != nil:
		showError(err)
	default:
		signInOnServer(c.IdentityToken, c.AuthorizationCode, nonce, c.GivenName, c.FamilyName, c.Email)
	}
})
cx.AppleCredentialState(savedUser, func(s core.AppleCredentialState, err error) {
	if err == nil && s == core.AppleCredentialRevoked {
		signOut()
	}
})
```

앱은 Apple로 로그인 인타이틀먼트로 서명되어야 하며, `nsgo.yaml`이 요청하면 `build`가 이를 추가합니다. 인타이틀먼트가 없으면 시트가 실패합니다.

```yaml
apple:
  sign_in_with_apple: true  # com.apple.developer.applesignin, in iOS and tvOS packages and Mac App Store builds
```

Xcode는 `nsgo dev`의 기기 빌드에 대해 이 기능을 등록합니다. `sign.profile` 또는 `sign.macos_profile`의 프로비저닝 프로필은 Apple로 로그인이 켜진 App ID에서 나온 것이어야 합니다. macOS는 프로필이 있어야만 이를 허용하므로, 디스크 이미지나 Mac에서의 `nsgo dev` 실행은 로그인할 수 없으며, 빌드가 개발자 인타이틀먼트를 요청하지 않는 시뮬레이터도 마찬가지입니다.

## 인앱 구매

`cx.Products`, `cx.Purchase` 및 관련 호출은 `cx.Env().CanPurchase()`가 true인 macOS, iOS, tvOS에서, `nsgo.yaml`이 StoreKit을 요청하는 빌드로 App Store를 통해 판매합니다.

```yaml
apple:
  purchases: true  # StoreKit's library, in macOS, iOS and tvOS packages; no entitlement is needed
```

앱 자체의 가격이 아니라 스토어가 제공하는 가격 `DisplayPrice`를 표시하세요. 스토어는 각 스토어프론트의 가격을 직접 정합니다. 구매는 트랜잭션 또는 오류로 한 번 응답합니다. 사용자가 시트를 닫으면 `core.PurchaseCancelled`, 구매 요청(Ask to Buy)처럼 승인을 기다리는 동안에는 `core.PurchasePending`, 그 밖에는 스토어의 메시지와 함께 `core.PurchaseFailed`, 다른 플랫폼에서는 `PurchasesUnavailable`입니다. 트랜잭션의 `Signed`, 즉 스토어의 JWS를 앱의 서버로 보내 검증하고, 구매한 것을 제공한 다음 트랜잭션을 완료하세요. 스토어는 완료되지 않은 트랜잭션을 실행할 때마다 앱에 다시 전달합니다.

`cx.ManageSubscriptions`는 Apple의 구독 관리 UI(iOS에서는 시스템 시트, macOS와 tvOS에서는 App Store 구독 페이지)를 엽니다. `cx.RequestRefund`는 트랜잭션에 대한 환불 요청을 시작하고 `core.RefundSuccess` 또는 `core.RefundCancelled`로 응답합니다.

```go
cx.OnPurchaseUpdate(func(t core.Transaction) { // renewals, approved purchases, other devices', refunds, and unfinished ones at launch
	verifyAndDeliver(t.Signed, func() { cx.FinishTransaction(t.ID, nil) })
})
cx.Products([]string{"com.example.pro.monthly"}, func(products []core.Product, err error) {
	if err == nil && len(products) > 0 {
		price.Set(products[0].DisplayPrice)
	}
})
cx.Purchase("com.example.pro.monthly", core.PurchaseOptions{AccountToken: userUUID}, func(t core.Transaction, err error) {
	switch {
	case errors.Is(err, core.PurchaseCancelled):
	case errors.Is(err, core.PurchasePending):
		showWaiting() // its transaction comes through OnPurchaseUpdate
	case err != nil:
		showError(err)
	default:
		verifyAndDeliver(t.Signed, func() { cx.FinishTransaction(t.ID, nil) })
	}
})
```

`FinishTransaction`은 이미 완료된 트랜잭션에 대해 `PurchaseNotFound`를 반환하며, `CurrentEntitlements`가 반환한 트랜잭션도 마찬가지입니다. 완료된 비소모성 항목은 계속 소유한 상태로 남습니다.

앱이 시작될 때 `OnPurchaseUpdate`를 등록하세요. 핸들러가 등록되기 전에 온 업데이트는 첫 번째 핸들러를 기다립니다. `cx.CurrentEntitlements`는 실행 시 잠금을 해제할 수 있도록 사용자가 현재 소유한 것을 응답하며, 구매 복원 버튼은 `cx.SyncPurchases`를 호출하고, 이 호출은 사용자에게 로그인을 요청할 수 있습니다. `AccountToken`은 트랜잭션과 App Store의 서버 알림이 전달하는 앱 자체의 UUID입니다. macOS는 Mac App Store 빌드에서만 판매하며, App Clip은 아무것도 판매하지 않습니다. Android, watchOS, Windows, Linux, 웹, 미니 프로그램은 현재 `PurchasesUnavailable`로 응답합니다. 구매는 `nsgotest`로 테스트하세요. `Products`, `PurchaseAnswer`, `UpdatePurchase`, `UnfinishedTransactions`가 스토어를 대신합니다. `app.PurchaseRequests`는 스토어 작업을 `nsgotest.PurchaseRequest` 값으로 기록합니다. TSX에서 `Purchases` 모듈은 `purchases` 기능(capability)이 필요합니다. `purchase`는 `{ status: "purchased", transaction }`, `"pending"` 또는 `"cancelled"`로 resolve되며, `on("update", listener)`가 업데이트를 받습니다.

## 화면 방향

전체 화면 플레이어의 스코프에서 `cx.RequestOrientation(core.OrientationLandscape)`를 호출해 iOS, Android, HarmonyOS 또는 브라우저에 가로 방향을 요청하세요. 스코프가 살아 있는 가장 최근 요청이 적용됩니다. 그 스코프가 끝나면 이전 요청이 다시 적용되거나 앱이 평소의 방향 정책으로 돌아갑니다.

iOS 16 이상에서는 요청이 인터페이스를 즉시 회전시킬 수 있습니다. 이전 iOS 버전에서는 다음 시스템 회전을 제한합니다. 표시된 모달의 `IOS.SupportedOrientations`가 우선합니다. Android와 HarmonyOS에서는 시스템이 표현할 수 없는 세로 및 가로 방향 조합이 허용되는 하나의 방향으로 줄어들며, 이미 표시된 방향이 우선됩니다. Android는 대화면이나 다중 창 모드에서 방향 요청을 무시할 수 있습니다.

웹 앱에서는 플레이어의 스코프를 여는 누르기 핸들러에서 `desktop.ToggleFullScreen(cx)`를 호출하세요. 브라우저가 누르기로부터 전체 화면에 진입한 다음 NSGo가 플레이어의 방향 요청을 적용합니다. 전체 화면을 종료하면 잠금이 해제됩니다. 지원되지 않는 작업과 거부는 브라우저 콘솔에 보고됩니다.

`nsgotest.App.Orientation`은 가장 최근에 요청된 마스크를 기록하며, 평소의 방향 정책이면 0입니다.

## 스토어 리뷰

`cx.RequestReview()`는 macOS와 iOS에서는 App Store에, Android에서는 Google Play에 앱 내 평가 요청 창을 요청합니다. `cx.Env().CanRequestReview()`는 서비스를 사용할 수 있는지 보고합니다. 요청 창을 표시할지와 언제 표시할지는 스토어가 결정하며, 결과는 보고되지 않습니다. Apple의 요청 창은 개발 빌드로 테스트하세요. TestFlight 빌드에서는 표시되지 않습니다.

독립 실행형 Android 패키지에는 Play Review SDK, 그 대화상자 액티비티, Play 스토어 가시성 쿼리가 포함되며, 빌드에 Android SDK나 JDK가 필요하지 않습니다. 활성화된 Play 스토어가 없는 기기는 서비스를 사용할 수 없다고 보고합니다. 출시된 패키지는 [내부 앱 공유](https://developer.android.com/guide/playcore/in-app-review/test)로 테스트하세요.

NSGo를 임베드할 때는 Gradle 호스트에 `com.google.android.play:review:2.0.2`를 추가하세요. 앱이 Play 스토어를 볼 수 있도록 호스트의 매니페스트에서 `<queries>` 아래에 `<package android:name="com.android.vending" />`를 추가하세요. 호스트가 코드를 축소한다면, NSGo가 이름으로 로드하므로 아래 규칙으로 Play Review와 Tasks API를 보존하세요.

```proguard
-keep class com.google.android.play.core.review.ReviewManagerFactory { *; }
-keep interface com.google.android.play.core.review.ReviewManager { *; }
-keep class com.google.android.play.core.review.ReviewInfo { *; }
-keep class com.google.android.gms.tasks.Task { *; }
-keep interface com.google.android.gms.tasks.OnCompleteListener { *; }
```

테스트에서는 `nsgotest.Environment`에 `CanRequestReview`를 설정하고 `nsgotest.App.ReviewRequests`를 확인하세요.

## 시스템 가장자리 제스처

화면 가장자리에 터치 컨트롤이 있는 게임은 컨트롤이 활성화된 동안 `mobile.DeferSystemGestures`를 호출할 수 있습니다. 반환된 함수를 사용하면 요청을 일찍 해제할 수 있습니다. 요청은 스코프가 정리될 때도 끝납니다. 같은 창에서 여러 스코프의 요청은 합쳐집니다.

```go
release := mobile.DeferSystemGestures(cx, mobile.SystemGestureOptions{
	IOSEdges: mobile.GestureEdgeBottom,
	AndroidRects: []core.Bounds{{
		Origin: core.Point{Y: 120},
		Size:   core.Size{Width: 24, Height: 96},
	}},
})
```

iOS는 물리적인 화면 가장자리를 사용합니다. Android 10 이상은 왼쪽 위를 기준으로 측정한 서피스 포인트 단위의 사각형을 사용합니다. 사각형은 그것이 필요한 컨트롤 주변으로만 유지하세요. Android는 홈 제스처를 예약하며, 앱이 뒤로 가기에서 제외할 수 있는 영역을 제한합니다. `nsgotest.App.SystemGestures`는 테스트에 메인 서피스의 현재 합쳐진 요청을 제공합니다. 다른 창에는 `Surface.SystemGestures()`를 사용하세요.

## Android의 오버레이

`mobile.OpenOverlay`는 Android에서 앱의 뷰를 다른 앱 위에 표시합니다. 이 뷰는 앱의 시그널을 공유하는 자체 트리이며, 표시되는 동안 포그라운드 서비스가 앱을 계속 실행합니다. 앱은 `android.permissions` 아래에 `SYSTEM_ALERT_WINDOW`를 나열하고, 사용자는 `mobile.RequestOverlayPermission`이 여는 시스템 설정에서 이를 허용합니다. 권한이 없으면 오버레이는 즉시 끝나고 `OnEnd`가 그 이유를 받습니다. 오버레이는 터치를 받지만, `Focusable`이나 `Passive`가 아니면 키 입력과 주변의 터치는 맨 앞에 있는 앱에 맡기며, 콘텐츠에서 `desktop.SetWindowFrame`으로 이동합니다. 다른 플랫폼은 오버레이를 열지 않습니다.

```go
mobile.RequestOverlayPermission(cx, func(p core.OverlayPermission) {
	if p == mobile.OverlayGranted {
		mobile.OpenOverlay(cx, mobile.OverlayOptions{Width: 200, Height: 80}, Score)
	}
})
```

## 위치와 모션

`cx.Locate`는 위치를 한 번 가져오고, `cx.WatchLocation`은 위치를 스트리밍하며, `cx.WatchMotion`은 가속도계, 자이로스코프, 자력계, 융합된 기기 모션 샘플을 스트리밍합니다. 각각은 `*core.SensorStream`을 반환하며, 그 `Stop`은 스트림을 취소하고, 스트림을 시작한 스코프가 끝날 때도 마찬가지입니다. 콜백은 엔진 고루틴에서 실행됩니다. 권한은 `core.PermissionLocation`, `PermissionBackgroundLocation`, `PermissionMotion`입니다([권한](permissions.md)). 스트림을 시작해도 권한을 묻지 않으며, 권한이 없는 스트림은 `core.SensorNotAllowed`로 실패합니다.

```go
cx.RequestPermission(core.PermissionLocation, func(s core.PermissionStatus) {
	if !s.Allowed() {
		return
	}
	cx.Locate(core.LocationOptions{}, func(f core.LocationFix, err error) {
		if err == nil {
			showPosition(f.Latitude, f.Longitude, f.Approximate)
		}
	})
})
```

`Locate`는 새 위치를 기다리며, 기본값은 30초(`Timeout`)입니다. `MaxAge`가 있어야만 캐시된 위치를 사용할 수 있으며, 그 경우 `Cached`와 `Age`로 이를 알려 줍니다. `Approximate`는 사용자가 대략적인 위치만 허용한 위치임을 알려 줍니다. Android는 그런 위치를 최대 10분마다 한 번만 갱신하므로, 그곳에서는 `Locate`에 `MaxAge`를 제공하세요. 좌표는 WGS 84이며, 제공자가 갖고 있지 않은 측정값은 `Valid`가 false이고, `Altitude`는 타원체 고도인 반면 `MSLAltitude`는 평균 해수면 기준 고도입니다. 모션은 기기의 자연 축을 사용하며, 가속도는 m/s²(화면이 위를 향하게 놓으면 z축이 +9.81), 회전은 rad/s, 자기장은 µT 단위이고, 기본적으로 20 ms마다이며 10 ms–1 s 범위로 제한됩니다. 부하가 걸리면 스트림은 센서별로 가장 최신 샘플을 유지하고 대체된 샘플 수를 `Dropped`에 셉니다.

스트림은 앱이 비활성 상태인 동안에도 계속되고 백그라운드에서는 일시 정지하며, 그 이후의 첫 샘플에 `Gap`을 표시합니다. 대기 중인 `Locate`는 남은 타임아웃을 유지합니다. `LocationOptions.Background`는 `PermissionBackgroundLocation` 하에서 위치 스트림을 백그라운드에서도 계속 유지합니다. Apple 앱은 `location` 백그라운드 모드를 추가하고, Android에서는 포그라운드 서비스가 `NotificationTitle`과 `NotificationBody`로 알림을 표시하며 액티비티가 끝난 뒤에도 앱을 계속 실행합니다. 시스템이 종료한 프로세스를 다시 시작하는 것은 없습니다.

```yaml
apple:
  info:
    NSLocationWhenInUseUsageDescription: Shows where you are on the map.
    NSLocationUsageDescription: Shows where you are on the map.  # macOS
    NSMotionUsageDescription: Counts your reps.
    UIBackgroundModes: [location]                                # background streams
android:
  permissions: [ACCESS_COARSE_LOCATION, ACCESS_FINE_LOCATION, FOREGROUND_SERVICE, FOREGROUND_SERVICE_LOCATION]
```

iOS와 watchOS는 위치와 모션을, macOS는 위치를, tvOS는 한 번에 하나의 위치를, Android는 프레임워크 자체 제공자를 통해 둘 다 제공합니다. Linux는 시스템 버스의 GeoClue 2에서 위치와 스트림을 가져오며, 그 에이전트는 앱의 첫 요청이 시작될 때 사용자에게 묻습니다. GeoClue는 요청 창 없이는 물어볼 수 없으므로, 그 실행에서 앱이 요청하기 전까지 권한은 아직 묻지 않은 상태로 읽히며, 백그라운드 위치는 사용할 수 없습니다. Windows는 WinRT의 `Geolocator`에서 가져옵니다. 앱에 대해 위치가 꺼져 있는 동안 권한은 거부로 읽히고, 그 실행에서 앱이 요청해 허용되면 허용으로, 그 밖에는 아직 묻지 않은 상태로 읽히며, 그곳에서도 백그라운드 위치는 사용할 수 없습니다. 보안 브라우저는 지오로케이션 위치와, 모바일에서는 가속도계, 자이로스코프, 융합 모션 측정값을 제공합니다. Safari에서는 누르기 핸들러에서 `cx.RequestPermission(core.PermissionMotion, done)`을 호출하세요. 브라우저는 `navigator.geolocation`이 없으면 위치를, 데스크톱에서와 자력계 스트림에 대해서는 모션을 사용할 수 없다고 보고합니다. TSX에서 `Sensors`는 패키지 기능 `location`, `backgroundLocation`, `motion` 하에서 `permission`, `requestPermission`, `locate`, `watchLocation`, `watchMotion`을 가지며, 밀리초 단위를 사용합니다.

## 오디오

`audio` 패키지는 `cx.Env().CanPlayAudio()`가 true인 곳에서 메모리로 한 번 디코딩되는 짧은 사운드(`audio.LoadSound`)와 스트리밍되는 긴 오디오(`audio.NewPlayer`)를 재생합니다. 그 밖의 곳에서는 호출이 `audio.Unavailable`로 실패합니다.

```go
audio.LoadSound(cx, audio.Source{URI: "asset:sounds/click.wav"}, func(click *audio.Sound, err error) {
	if err != nil {
		return
	}
	click.Play(audio.Voice{})                                  // center, full volume, normal rate
	click.Play(audio.Voice{Volume: 0.5, Rate: 1.2, Pan: -0.5}) // overlaps, panned left
})
```

`audio.Voice`는 재생별 `Volume`(0에서 1, 0은 1로 취급), `Rate`(0.5에서 2, 0은 1로 취급), `Pan`(-1은 완전히 왼쪽, 1은 완전히 오른쪽, 0은 가운데)을 설정합니다. 이 범위를 벗어난 값은 범위 안으로 고정되고 NaN은 기본값을 사용합니다. `Sound.Stop`은 사운드의 모든 재생을 중지하고, `Close`는 사운드를 해제하며, 소유 스코프가 해제될 때도 마찬가지입니다. `Player`는 `Play`, `Preload`, `Pause`, `Seek`, `SetVolume`, `SetRate`, `SetLoop`, `Close`를 지원합니다. 현재 macOS, iOS, Windows, Linux, Android, 웹이 오디오를 재생합니다. Apple, Windows, 브라우저 백엔드는 WAV, AAC, MP3를 지원합니다. Linux는 GStreamer 코덱을 사용하며, 스트리밍 플레이어는 1× 속도로 재생합니다. Android는 뷰가 화면을 벗어나면 플레이어를 일시 정지하고, 디코딩된 각 사운드를 1 MiB로 제한합니다. tvOS, watchOS, HarmonyOS, 미니 프로그램은 여전히 `CanPlayAudio`를 false로 보고합니다.

브라우저는 소리가 나는 재생에 사용자 제스처를 요구합니다. 짧은 사운드는 미리 로드한 다음, 클릭, 탭 또는 키 입력 후에 `Sound.Play`를 호출하세요. 제스처 전에 재생한 사운드는 소리가 나지 않습니다. 브라우저가 재생을 막으면 플레이어의 `Play` 콜백은 `audio.NotAllowed`를 받습니다. 사용자 제스처에서 다시 시도하세요.

`Sound.Play`는 `audio.Playback`을 반환하며, macOS, iOS, Windows, Linux, 웹에서는 이것으로 자체 볼륨과 속도를 설정하거나 독립적으로 중지할 수 있습니다. `Ended()`는 반응형이며, 그 재생이 끝나거나, 중지되거나, 음성 수 제한으로 밀려나거나, 시작할 수 없으면 true가 됩니다. 개별 제어가 없는 백엔드는 이미 끝난 핸들을 반환합니다.

iOS에서 `AudioQueue`처럼 `audio`와 `Video` 밖에서 재생하는 라이브러리 자체의 오디오 엔진은 재생하는 동안 `audio.HoldSession(core.AudioSessionPlayback)`으로 앱의 단일 오디오 세션을 유지하고, 재생을 멈추면 반환된 해제 함수를 호출합니다. 이는 앱의 비디오, 플레이어, 녹음과 함께 플레이어의 `AudioSession`이 사용하는 수준을 받으므로, `AudioSessionPlayback`과 `AudioSessionMixWithOthers`는 벨/무음 스위치가 무음일 때도 계속 재생합니다. 다른 곳에서는 아무것도 유지하지 않습니다.

효과음을 소리 없이 시작하려면 `audio.Voice{Muted: true}`를, 재생 중인 효과음을 무음으로 만들려면 `playback.SetVolume(0)`을 사용하세요. 긴 오디오의 경우 `audio.NewPlayer`가 소스를 스트리밍하고, `player.SetMuted(true)`는 재생을 계속하면서 출력을 무음으로 만듭니다. 음소거를 해제하면 플레이어에 설정된 볼륨이 복원됩니다. `Voice.Volume`과 `Player.SetVolume`에서 0은 기본 최대 볼륨을 뜻합니다.

TSX에서 `nsgo/audio`는 `audio` 패키지 기능으로 제한되는 `supported()`와 `loadSound(uri)`를 제공합니다. `loadSound`는 사운드로 resolve되며, 그 `play({ volume, rate, pan, loop, muted })`는 `setVolume`, `setRate`, `stop`과 반응형 `ended`를 가진 재생을 반환합니다. 그곳에서는 볼륨 0이 무음입니다. 거부된 로드는 `Error`로 reject되며, 그 `reason`은 `quota-exceeded` 같은 오디오 오류의 이름이고, `code`는 기능이 없으면 `ERR_ACCESS_DENIED`, 제한을 넘으면 `ERR_NSH_QUOTA_EXCEEDED`, 오디오를 재생할 수 없는 곳에서는 `ERR_FEATURE_UNAVAILABLE_ON_PLATFORM`입니다. 사운드는 `close()`로 또는 그 뷰가 사라질 때만 해제됩니다. 샌드박스에서는 각 로드가 실패하거나 그 사운드가 닫힐 때까지 `Limits.Sounds`(기본값 256)에 포함됩니다.

## 게임패드

`gamepad` 패키지는 W3C Standard Gamepad 레이아웃(`ButtonSouth`/`ButtonA`부터 `ButtonHome`까지, `AxisLeftX`/`AxisLeftY`/`AxisRightX`/`AxisRightY`)으로 연결된 게임 컨트롤러를 읽습니다. `cx.Env().SupportsGamepads()`는 타깃이 컨트롤러를 지원하는지 보고합니다(macOS, iOS, Windows, Linux, Android, 그리고 Gamepad API가 있는 브라우저).

`gamepad.Pads(cx)`는 추적되는 반응형 읽기로, 컨트롤러가 연결되거나 해제되면 스코프를 다시 렌더링하지만 버튼이나 스틱 변화에는 다시 렌더링하지 않습니다. 각 `gamepad.Pad`는 `Family`(`FamilyXbox`, `FamilyPlayStation`, `FamilyNintendo` 또는 `FamilyGeneric`, `gamepad.DetectFamily`로도 분류 가능), 플랫폼이 보고하는 경우 USB `VendorID`와 `ProductID`, 그리고 햅틱 모터 지원 여부인 `CanRumble`을 제공합니다. `RequestAnimationFrame` 루프 안에서 `gamepad.State(cx, id)`는 디스플레이 프레임마다 한 번 스냅숏된 컨트롤러의 버튼(`[0, 1]`, `>= 0.5`에서 `Pad.Pressed`가 true)과 축(`[-1, 1]`, 음수는 왼쪽/위, 양수는 오른쪽/아래)을 반환합니다.

`gamepad.Rumble(cx, id, lowFreq, highFreq, duration)`은 `[0, 1]`로 고정된 강도로 저주파(강한/왼쪽) 및 고주파(약한/오른쪽) 진동 모터를 구동합니다. 해당 컨트롤러에서 실행 중인 진동을 대체하며, `duration`이 지나거나, 앱이 백그라운드로 가거나 포커스를 잃거나(`cx.Env().Lifecycle() != core.LifecycleActive`), `cx`가 해제되거나, 컨트롤러가 연결 해제되면 자동으로 멈춥니다. `gamepad.StopRumble(cx, id)`는 진동을 일찍 멈춥니다. `CanRumble == false`인 컨트롤러는 진동 호출을 깔끔하게 무시합니다.

```go
var loop func(time.Time)
loop = func(time.Time) {
	if pad, ok := gamepad.State(cx, 0); ok {
		move(pad.Axes[gamepad.AxisLeftX], pad.Axes[gamepad.AxisLeftY], pad.Pressed(gamepad.ButtonA))
		if hit {
			gamepad.Rumble(cx, pad.ID, 0.8, 0.3, 180*time.Millisecond)
		}
	}
	cx.RequestAnimationFrame(loop)
}
cx.RequestAnimationFrame(loop)
```

컨트롤러로 네이티브 포커스 정지 지점을 탐색하도록 하려면 `gamepad.EnableFocusNavigation(cx)`를 호출하세요(D-패드와 왼쪽 스틱이 포커스를 이동하고, `ButtonSouth`는 포커스된 컨트롤을 활성화하며, `ButtonEast`는 뒤로 가기를 실행합니다). 게임 플레이 중인 뷰는 마운트되어 있는 동안 포커스 탐색을 일시 정지하도록 `gamepad.CaptureInput(cx)`를 호출합니다. TSX에서 `nsgo/gamepad`는 `gamepad` 패키지 기능으로 제한되는 `supported()`, `pads()`, `state(id)`, `Button`, `Axis`를 제공합니다.

## 오디오 타이밍

시간이 정해진 재생에는 `audio.Now(cx)`를 시계로 사용하세요. `audio.CanSchedule(cx)`가 true이면 `audio.Voice.StartAt`을 미래 시각으로 설정하세요. 활성 출력 경로에 대해서는 `audio.OutputLatency(cx, func(latency time.Duration, known bool) { ... })`를 호출하세요. macOS, iOS, Linux, 웹은 시간 지정 시작을 지원합니다. Windows와 Android에서는 `CanSchedule`이 false이며 `StartAt`이 즉시 재생됩니다. 콜백의 `known` 값은 경로가 지연 시간을 보고했는지 알려 주며, Windows와 Android는 보고하지 않습니다.

## 게임 서비스

`gameservices` 패키지는 `cx.Env().CanUseGameServices()`가 true인 곳에서 플레이어 로그인, 업적, 리더보드, 네이티브 Game Center / Play Games 오버레이를 위한 제공자 중립적인 API를 제공합니다. 모든 콜백은 엔진 고루틴에서 실행되므로 프레임을 막지 않고 시그널을 직접 설정할 수 있습니다.

```go
if cx.Env().CanUseGameServices() {
	gameservices.Authenticate(cx, func(p gameservices.Player, err error) {
		if err == nil && p.Authenticated {
			playerName.Set(p.DisplayName)
			gameservices.ReportAchievement(cx, "first-run", 100, nil)
			gameservices.SubmitScore(cx, "high-scores", 1250, nil)
		}
	})
}
```

`gameservices.ReportAchievement`는 진행률을 `0..100`으로 고정합니다(`100`이면 업적이 잠금 해제됩니다). `gameservices.LoadScores`는 `AllTime`, `Week` 또는 `Today` 기간에 대해 `Global` 또는 `Friends`의 순위가 매겨진 `gameservices.ScoreEntry` 행을 읽고, `gameservices.ShowAchievements`와 `gameservices.ShowLeaderboard`는 플랫폼의 네이티브 오버레이를 엽니다. macOS, iOS, tvOS는 purego를 통해 GameKit에 접근합니다. Android는 아직 런타임에 없는 Java 런타임 훅을 통해 Play Games Services에 접근할 예정이므로 현재는 `CanUseGameServices() == false`를 보고합니다. Windows, Linux, 웹, watchOS, 미니 프로그램은 `CanUseGameServices() == false`를 보고하고 `gameservices.ErrUnsupported`로 응답합니다.

테스트에서는 `nsgotest.Environment(core.Environment{CanUseGameServices: true})`로 게임 서비스를 활성화하고, `App.SetGameServicesPlayer`, `App.SetGameServicesFriends`, `App.SeedLeaderboardScores`, `App.SetGameServicesError`, `App.GameServicesAchievements`, `App.GameServicesScores`, `App.GameServicesShownAchievements`, `App.GameServicesShownLeaderboards`로 각 앱의 격리된 가짜 구현을 제어하세요.

## 다음 단계

- [알림](notifications.md): 권한, 배지, 누르기.
- [폼](forms.md): 텍스트 필드와 키보드.
- [임베딩](embedding.md): 호스트 앱이 임베드된 앱에 제공하는 서비스.
- [빌드와 배포](building.md): 패키지에 필요한 권한과 파일.

### 피트니스, 지역, 제한된 모션

`cx.WatchSteps`, `cx.WatchActivity`, `cx.WatchPressure`는 스코프에 묶인 스트림을 반환합니다. 걸음 수/활동 전에, 그리고 Apple에서는 기압 전에 `core.PermissionActivityRecognition`을 요청하세요. Apple은 Core Motion을 통해 세 가지 모두를 제공합니다. Android는 걸음 수 카운터와 기압을 제공하며, 활동 분류는 사용할 수 없다고 보고합니다. 카운트는 샘플의 `Since` 구간에 속하며, 재개된 구간에는 `Gap`이 표시됩니다. 기압은 파스칼 단위이며, 선택적으로 미터 단위의 상대 고도를 포함합니다. 없는 거리, 층수, 고도는 `Valid`가 false입니다.

`cx.WatchGeofences`는 1개에서 20개까지의 서로 다른 `GeofenceRegion` 원(WGS 84 좌표와 미터 단위 반경)을 가진 `core.GeofenceOptions`를 받습니다. 지역별로 `GeofenceInside`, `GeofenceOutside` 또는 `GeofenceUnknown`을 보고합니다. `Background`에는 백그라운드 위치 접근 권한/선언과 Apple의 '항상' 권한이 필요하며, Android에는 알림 제목도 필요합니다. 중지하거나 해제하면 등록이 제거되며, 프로세스가 종료된 뒤에 살아남는 구독은 없습니다.

`cx.RequestPreciseLocation("route", done)`은 정확한 위치 권한을 명시적으로 요청하고 위치가 아닌 boolean으로 응답합니다. `apple.info`에 `NSLocationTemporaryUsageDescriptionDictionary: {route: Explains why this feature needs precision.}`를 선언하세요. Android에는 coarse와 fine 권한이 모두 필요합니다. false 응답이어도 대략적인 위치 권한은 허용된 상태로 남을 수 있습니다.

`cx.WatchBackgroundMotion(core.BackgroundMotionOptions{Motion: core.MotionOptions{}, Duration: 20 * time.Second, NotificationTitle: "Recording motion"}, sample, failed)`은 기간이 한정된 실시간 스트림을 요청합니다. `AccessMotion`과 `AccessBackgroundMotion`이 필요하며, TSX 패키지는 `motion`과 `backgroundMotion`을 선언합니다. iOS는 실행 임대(lease)를 더 일찍 만료시킬 수 있습니다. Android 앱은 `android.background_motion: true`를 설정하고 `FOREGROUND_SERVICE`를 선언합니다. 비고정(nonsticky) 서비스는 요청한 기간이 더 길더라도 3분 안에 중지됩니다. 기간 만료나 OS에 의한 만료는 `SensorTimedOut`을 보고합니다. 중지/해제는 제공자와 임대를 종료하며, 종료 후에는 아무것도 기록하지 않습니다. 다른 타깃은 사용할 수 없다고 보고합니다.

TSX `Sensors`는 이 여섯 메서드를 그대로 제공하며 밀리초 단위를 사용합니다. 헤드리스 테스트는 `FeedSteps`, `FeedActivity`, `FeedPressure`, `FeedGeofence`로 데이터를 공급하고, 정확도 요청에 대해 `PreciseLocationAnswer`를 설정합니다. 실제 피트니스, 지역 기반 깨우기, 일시 중단된 백그라운드 동작은 여전히 기기에서의 검수가 필요합니다.
