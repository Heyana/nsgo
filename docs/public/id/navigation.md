---
title: Navigasi
description: Stack navigasi native, header sistem, modal, menu, dan bilah tab.
group: Konsep inti
order: 56
---

## Stack

`base.NewNav` menyimpan sebuah stack berisi route dengan satu tipe: `Push` menambahkan halaman, `Pop` menghapus halaman teratas, `PopToRoot` kembali ke halaman pertama, dan `Replace` menukar halaman teratas. `Stack` miliknya merender setiap route sebagai halaman, menjaga halaman-halaman di bawahnya tetap terpasang beserta state-nya sementara halaman teratas ditampilkan. `ui.NavPage` memberi setiap halaman latar belakang dari `ui` dan sebuah `ui.NavHeader` dengan judul, tombol kembali, dan tombol-tombolnya.

watchOS tidak memiliki bilah navigasi, bilah tab, atau sakelar sistem, sehingga di sana `ui.NavPage` menggambar `ui.NavHeader` miliknya di atas halaman, termasuk tombol kembali, `ui.TabShell` menggambar bilah tabnya sendiri, dan `ui.SwitchSystem` adalah sakelar milik `ui` sendiri.

```go
nav := base.NewNav(cx, "inbox")

return nav.Stack(core.Style{FlexGrow: 1}, func(cx *core.Scope, route string, depth func() int) base.NavPage {
	if route == "inbox" {
		return ui.NavPage(cx, base.NavPage{Title: "Inbox", Children: []core.Node{
			ui.Button{Text: "Open a message", OnPress: func() { nav.Push("message") }},
		}}, ui.NavHeader{})
	}
	return ui.NavPage(cx, base.NavPage{Title: "Message"}, ui.NavHeader{
		OnBack: func() { nav.Pop(1) }, BackLabel: "Inbox",
		Trailing: []core.Node{ui.IconButton{Icon: lucide.Archive, Label: "Archive", Variant: ui.IconButtonGhost, OnPress: archive}},
	})
})
```

Di iOS dan Android platform menggambar header sistem dari judul halaman dan tombol-tombol header, dengan tombol kembali dan transisinya; di macOS, Windows, Linux, dan web, `ui` yang menggambarnya.

Android menggunakan top app bar Material 3. Bilah biasa setinggi 64 dp dengan judul 22 sp. `HeaderLargeTitle` memberikan bilah besar setinggi 152 dp dengan judul 28 sp yang menciut menjadi bilah biasa saat tampilan gulir di bagian atas halaman menggulir kontennya ke bawah bilah tersebut, dan meluas kembali saat konten kembali ke atas.

Atur `HeaderTitleStyle` dan `HeaderLargeTitleStyle` untuk memberi gaya pada font judul ringkas dan diperluas yang native di iOS dan Android: `FontSize`, `FontWeight`, `FontFamily`, dan `FontStyle`. Field yang tidak diatur mempertahankan default masing-masing platform; `HeaderTitleStyleFunc` dan `HeaderLargeTitleStyleFunc` mengikuti perubahan. `HeaderStyle` tetap menyediakan latar belakang, warna judul, dan tint tombol. Di TSX, ini adalah objek atau array gaya `headerTitleStyle` dan `headerLargeTitleStyle`.

`HeaderTitle` kustom hanya menggantikan judul ringkas di Android; `Title` tetap menjadi judul besar yang diperluas. Atur `Android: core.AndroidScreenProps{HeaderRightWithLargeTitle: true}` untuk menaruh `HeaderRight` di samping judul besar tersebut. Kontrol-kontrolnya berpindah ke toolbar yang tersemat saat judul menciut dan tetap dapat ditekan sepanjang waktu. TSX menggunakan `android={{ headerRightWithLargeTitle: true }}`. Pita besar akan membesar ketika fontnya memerlukan ruang lebih. Build Android mandiri mengaktifkan tata letak RTL native; host tertanam harus mengatur `android:supportsRtl="true"` pada aplikasinya agar header native mengikuti arah aplikasi.

Beri sebuah halaman `Header` miliknya sendiri untuk menggantikan `ui.NavHeader`. Di iOS dan Android, header ini menjadi tampilan judul pada header sistem, menggantikan `HeaderTitle`, di samping `HeaderLeft`, `HeaderRight`, dan tombol kembali milik halaman; header ini mengisi ruang di antara kedua ujung tersebut ketika tampilannya membesar atau mengambil lebar dalam persentase, seperti kolom pencarian, dan jika tidak, mengikuti ukuran kontennya. Di platform lain, header ini digambar di atas konten. `ui.NavHeader.Scroll` tetap menghubungkan pengguliran halaman iOS atau Android dengan efek blur header-nya.

`NoPopOnBack` mempertahankan halaman saat tombol kembali ditekan, `OnBackPress` menerima penekanan tersebut, dan `NoPopOnSwipe` menonaktifkan usapan kembali di iOS. `cx.OnBackPress(func() bool)` memberi aplikasi sebuah handler sebelum stack, untuk perubahan yang belum disimpan. `OnDismiss` melaporkan bahwa pengguna telah mengeluarkan halaman, dan stack menghapusnya.

## Layar penuh imersif

Panggil `mobile.Immersive(cx, true)` untuk meminta layar penuh bagi jendela dan `mobile.Immersive(cx, false)` untuk kembali ke pengaturan bilah sistem regulernya. Untuk sebuah halaman atau dialog, gunakan `StatusBar: core.StatusBarSetting{Immersive: core.ToggleOn}` untuk memintanya selama lapisan tersebut ditampilkan. Di TSX, gunakan `statusBar={{ immersive: true }}` pada halaman atau dialog, atau `status_bar: { immersive: true }` di `nsgo.yaml` untuk default jendela.

Di Android, usapan dari tepi akan menampilkan bilah status dan bilah navigasi sejenak. Di HarmonyOS, jendela meminta sistem menyembunyikan bilah status, bilah navigasi tiga tombol, dan indikator navigasi. Di iOS, bilah status disembunyikan dan UIKit memutuskan kapan menyembunyikan atau menampilkan indikator home. `cx.Env().SafeArea()` tetap melaporkan inset yang disediakan sistem, termasuk potongan layar (display cutout). Di HarmonyOS dengan navigasi tiga tombol dalam orientasi lanskap, sistem melaporkan area tombol di sepanjang tepi bawah sementara tombol-tombol itu digambar di sepanjang sisi samping, sehingga area aman dapat menyisakan ruang di bagian bawah sementara tombol-tombol menutupi sisi samping.

## Modal

`ui.Dialog` menampilkan sebuah permukaan di atas aplikasi yang diredupkan, berupa sheet dari bawah di layar sentuh, dan `ui.Sheet` menampilkan panel dari bawah. Masing-masing tertutup dengan Escape dan dengan menekan latar belakangnya kecuali diperintahkan sebaliknya.

```go
ui.Dialog{
	Open:         dialog.Get,
	OnOpenChange: func(open bool, _ base.Reason) { dialog.Set(open) },
	Title:        "Rename",
	Description:  "The new name shows everywhere.",
	Children:     []core.Node{ui.Input{Value: name.Get, OnChange: name.Set}},
	Footer: []core.Node{
		ui.Button{Text: "Cancel", Variant: ui.ButtonSecondary, OnPress: func() { dialog.Set(false) }},
		ui.Button{Text: "Save", OnPress: save},
	},
}
```

`ui.AlertDialog` meminta pengguna untuk mengonfirmasi, dan `cx.ShowAlert` menampilkan alert atau action sheet milik sistem sendiri ([Dialog](ui/dialog.md)).

## Menu

`ui.Menu` membuka `ui.MenuItem` dari sebuah tombol: di sampingnya, atau sebagai sheet dari bawah ketika `Presentation`-nya memintanya, di perangkat apa pun. `ui.ContextMenu` melakukan hal yang sama untuk klik sekunder atau tekan lama, dan `ui.Popover`, `ui.HoverCard`, serta `ui.Tooltip` menampilkan konten berjangkar lainnya.

```go
ui.Menu{Trigger: ui.Button{Text: "Actions", Variant: ui.ButtonSecondary}, Children: []core.Node{
	ui.MenuItem{Text: "Rename", OnSelect: rename},
	ui.MenuItem{Text: "Duplicate", OnSelect: duplicate},
	ui.MenuItem{Text: "Delete", Variant: ui.MenuItemDanger, OnSelect: remove},
}}
```

## Tab

`ui.Tabs[T]` adalah kontrol tersegmentasi atau daftar tab, dengan indikator yang bergeser ke opsi yang dipilih; `TabStyle` digabungkan di atas gaya setiap tab, `Selected` miliknya di atas gaya tab yang dipilih, dan `IndicatorStyle` di atas gaya indikator. Tab bergaris bawah dapat mengatur `IndicatorColor` dan menyesuaikan garis dengan kontennya menggunakan `IndicatorFit: TabsIndicatorContent`. `ui.Sidebar` menampung bagian dan baris untuk sidebar desktop, dan `ui.TabShell` menjaga bagian-bagian tingkat atas di samping bilah tab (di bawah secara default, atau vertikal di tepi awal atau akhir dengan `Edge` / `EdgeFunc`, termasuk `mobile.TabBarAutomatic` untuk mengikuti `cx.Env().VerticalBarEdge()`) pada setiap lebar jendela. Ia menggunakan bilah milik sistem ketika setiap tab memiliki `SystemIcon` atau `Icon` SVG, di tepi mana pun di iOS dan di tepi bawah di Android, dengan menampilkan SVG di mana pun `SystemIcon` tidak menamai SF Symbol di iOS atau drawable di Android; jika tidak, ia menggambar bilahnya dengan ikon SVG. Atur `BarHidden` selama halaman detail yang di-push terlihat untuk menyembunyikan bilah mana pun dan memulihkan area aman bagian tersebut; `TabBarInset` menjadi 0 selama bilah disembunyikan atau ditempatkan secara vertikal.

`mobile.TabBar` adalah bilah tab milik sistem di iOS dan Android, dengan tampilan Liquid Glass atau Material; ia menerima `Items` berupa `mobile.TabBarItem{Title, Icon, SVG}`, indeks yang dipilih, `OnChange`, dan `Edge` opsional (`mobile.TabBarBottom`, `mobile.TabBarLeading`, `mobile.TabBarTrailing`, atau `mobile.TabBarAutomatic`, yang mengikuti `cx.Env().VerticalBarEdge()` di iOS 27.1 dan kembali ke tepi bawah di tempat lain) atau `EdgeFunc` yang reaktif. Peran palet Android `tabBarBackground`, `tabBarIdle`, `tabBarSelected`, dan `tabBarIndicator` mengatur latar belakang bilah, ikon dan label yang tidak aktif, ikon dan label yang dipilih, serta pil yang dipilih. Palet kustom kembali ke `Surface`, `TextMuted`, `Accent`, dan `accentContainer` secara berurutan, dengan pil aksen 12% ketika `accentContainer` tidak diatur. Palet default mempertahankan warna latar belakang dan warna tidak aktif Material, dengan tab yang dipilih diberi tint warna aksen. Di tempat lain ia tidak mengambil ruang, sehingga aplikasi menampilkan `ui.Tabs` di sana. `mobile.IsAvailable(cx)` menyatakan apakah bilah tersebut tampil. `Icon` menamai ikon platform; `SVG`, seperti `lucide.House()`, tampil di iOS dan Android di mana pun `Icon` tidak menamai SF Symbol atau drawable, diberi tint oleh bilah seperti ikon sistem.

Atur `TabBarItem.Badge` ke `mobile.TabBadge{Text: "3"}` untuk teks atau `mobile.TabBadge{Dot: true}` untuk titik. Perbarui melalui `ItemsFunc`, dan kembalikan nilai nol untuk menghapusnya. `ui.ShellTab.Badge` menerima `func() mobile.TabBadge` untuk bilah sistem maupun bilah yang digambar. Teks lebih diutamakan ketika kedua field diatur. UIKit menampilkan badge native kosong untuk titik. Teknologi asistif membacakan teks badge atau kata yang dilokalkan untuk konten baru. TSX menerima `badge: { text: "3" }` atau `badge: { dot: true }` pada item native, dan fungsi yang mengembalikan nilai tersebut pada item `TabShell`.

```go
mobile.TabBar{
	Items: []mobile.TabBarItem{{Title: "Home", Icon: "house"}, {Title: "Search", Icon: "magnifyingglass"}},
	Selected: func() int { return tab.Get() },
	OnChange: func(i int) { tab.Set(i) },
}
```

## Berikutnya

- [Formulir](forms.md): apa yang ditampung oleh halaman dalam stack.
- [Animasi](animation.md): bagaimana halaman dan modal bergerak.
- [Tata letak](layout.md): area aman di sekitar header.
