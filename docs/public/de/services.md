---
title: Plattformdienste
description: "Hinweisdialoge, die Zwischenablage, Dateien, Fotos, Teilen, Speicher, Geheimnisse, Spracherkennung, Sprachausgabe, Netzwerk, Anmeldung, In-App-Käufe und der Lebenszyklus der App."
group: Plattformfunktionen
order: 44
---

## Hinweisdialoge und die Zwischenablage

`cx.ShowAlert` zeigt den Hinweisdialog oder das Aktionsblatt der Plattform. Seine `Actions` kommen über ihren Index zurück, und eine Rolle zeichnet die Abbrechen- und destruktiven Buttons so, wie es die Plattform tut. Unter macOS verwenden Hinweisdialoge und Dateiauswahldialoge ein Sheet, solange ihr Fenster sichtbar ist. Ein ausgeblendetes, minimiertes oder außerhalb des Bildschirms liegendes Fenster oder eine Tray-App ohne Fenster öffnet den Dialog stattdessen separat. Das Schließen des anfordernden Fensters bricht seine Dialoge ab.

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

`SetClipboard` schreibt Text und `SetClipboardImage` ein Bild. `ReadClipboard`, `ReadClipboardImage` und `ReadClipboardFiles` antworten asynchron, und `HasClipboardText` und `HasClipboardImage` lesen, was die Zwischenablage enthält, ohne einen Einfügehinweis. `core.FileFromURL` öffnet die URLs, die `ReadClipboardFiles` und `View.OnFileDrop` liefern, als `core.File`: `file:`-URLs und im Web `blob:`-URLs.

tvOS hat keine öffentliche Zwischenablage. Schreibvorgänge in die Zwischenablage werden ignoriert, Lesevorgänge liefern keinen Inhalt, und Prüfungen auf vorhandene Inhalte ergeben false.

```go
cx.ReadClipboard(func(text string) { pasted.Set(text) })
cx.SetClipboard("Hello")
```

Unter macOS und Windows liefert `ReadClipboardItems` eine Momentaufnahme aller verfügbaren Byte-Darstellungen bis zu 48 MiB. Jedes Element enthält geordnete `ClipboardRepresentation`-Werte mit einem nativen `Type` und seinen Rohdaten `Data`, einschließlich Formaten, die nur ihr Vorhandensein anzeigen und leere Daten haben. `ClipboardFormats` listet native Formatnamen auf, ohne ihre Nutzdaten zu kopieren. Um eine Momentaufnahme wiederherzustellen, übergeben Sie sie an `WriteClipboardItems`, zusammen mit einem optionalen Callback, der den Schreibfehler erhält. Ein leerer Slice leert die Zwischenablage. Vergleichen Sie Werte von `ClipboardChangeCount` auf Gleichheit, um einen neuen Kopiervorgang zu erkennen, auch wenn der kopierte Text identisch ist.

macOS behält die Elementgrenzen und die Typnamen der Zwischenablage bei. Windows verwendet ein einziges Element und bewahrt bytebasierte Win32-Namen wie `CF_HDROP` und registrierte Formate wie `Rich Text Format`. Beim Schreiben fasst es Elemente mit unterschiedlichen Formaten zusammen und verwendet für leere Nutzdaten ein Null-Byte. GDI-Handles, Owner-Display-Formate und private numerische Format-IDs werden ausgelassen. Auf anderen Plattformen erhalten die Lese- und Schreib-Callbacks `CodeUnavailable` und der Zähler-Callback `-1`.

## Links und Dateien öffnen

`cx.OpenURL` öffnet eine URL im Browser oder in der App des Systems, und `core.CheckURL` sagt, was es ablehnt: kein Schema, mehr als 16 KiB, Leerzeichen, eine Webadresse ohne Host sowie `javascript:`, `data:`, `blob:`, `file:`, `about:` und `asset:`.

```go
if err := core.CheckURL(url); err == nil {
	cx.OpenURL(url, func(opened bool) {
		if !opened {
			status.Set("No app opens that link.")
		}
	})
}
```

`cx.OpenFile` öffnet eine Datei, die die App besitzt, in der App, die ihrem Typ zugeordnet ist, und `cx.Share` zeigt das Teilen-Menü des Systems für Dateien, Text oder beides unter macOS, iOS, Android, Windows, Linux (über das Desktop-Portal oder den Dateimanager) und im Web (`navigator.share`). Eine `core.SharedFile` ist ein Pfad auf der Festplatte – oder, für `Share` im Web und überall dort, wo eine Datei im Speicher gehalten wird, ihre `Data`-Bytes – mit dem Namen und Typ, den die andere App sieht. Auf Desktops zeigt `desktop.RevealFile` eine Datei oder einen Ordner ausgewählt im Finder, im Explorer oder im Dateimanager, und `desktop.TrashFile` verschiebt sie in den Papierkorb; es löscht niemals endgültig, sondern antwortet mit `core.ErrNoTrash` und lässt das Element an seinem Platz, wenn das Volume keinen Papierkorb hat oder der Papierkorb es nicht aufnehmen würde, etwa eine Datei, die größer ist als der Papierkorb.

```go
cx.Share(core.ShareOptions{
	Files: []core.SharedFile{{Path: path, Name: "Notes.txt"}},
	Text:  "A note from NSGo",
}, func(shown bool, err error) {})
```

`mobile.OnShared(cx, func(items []mobile.SharedItem) { ... })` empfängt Text und Dateien für [`android.share_types`](building.md#incoming-shares-on-android) oder [`apple.share_types`](building.md#incoming-shares-on-ios). Geteilte Inhalte unter Android können die App starten; iOS stellt sie in einer App Group in eine Warteschlange, bis die App aktiv ist oder der Benutzer sie das nächste Mal öffnet. Ausstehende geteilte Inhalte warten auf den ersten Handler. iOS behält den Stapel im Posteingang, bis der Callback zurückkehrt; die Zustellung erfolgt über ein Prozessende hinweg mindestens einmal. Text kommt in `Text` an, und Dateien kommen als lesbare Cache-Kopien mit `Path`, `Name` und `MIME` an. Verschieben oder kopieren Sie eine Datei in dauerhaften Speicher, wenn sie den Cache überdauern soll.

`desktop.LookUp` zeigt das Wörterbuch-Panel des Systems für einen Text, wie es ein Tippen mit drei Fingern unter macOS tut, gezeichnet über einem Rechteck in einer View: den Text und das Rechteck einer Auswahl mit der `Ref` ihrer `Selectable`-View. `desktop.CanLookUp` sagt, ob das möglich ist, in einem Fenster unter macOS; Windows, Linux und das Web haben kein solches Panel. In Tests listet `nsgotest.App.LookUps` die angezeigten Panels auf, wenn `nsgotest.Environment` `CanLookUp` setzt.

```go
var area core.Ref
var selected core.TextSelection
core.View{Ref: &area, Selectable: true, OnTextSelection: func(s core.TextSelection) { selected = s }, Children: subtitles}
// From a menu item or a shortcut:
if desktop.CanLookUp(cx) {
	desktop.LookUp(cx, selected.Text, &area, selected.Bounds)
}
```

`desktop.QuickLook` fordert Dateivorschauen an, beginnend bei einem Index, und aktualisiert die Dateien, wenn es erneut aufgerufen wird, während das Panel geöffnet ist. `desktop.CanQuickLook` meldet die Verfügbarkeit in einem eigenen Fenster der App unter macOS und Windows sowie unter Linux, wenn die Sitzung den laufenden oder aktivierbaren D-Bus-Dienst von GNOME Sushi bereitstellt. Windows verwendet den registrierten Vorschau-Handler des Dateityps; welche Formate unterstützt werden, hängt von den installierten Handlern bzw. der Vorschau ab. Ein Ergebnis true bedeutet, dass die Plattform angefragt wurde, nicht dass sie die Datei dargestellt hat.

Leertaste oder Escape schließt das Panel. Pfeiltasten wählen Dateien der Anfrage unter Windows aus sowie in Sushi-Versionen, die Auswahl und Besitzer unter X11 melden. Unter Wayland öffnet Linux derzeit eine Vorschau ohne übergeordnetes Fenster; automatisches Schließen mit dem Besitzer und Auswahl per Pfeiltasten sind nicht verfügbar. KDE-spezifische Vorschauprogramme und das Web werden nicht unterstützt.

```go
if desktop.CanQuickLook(cx) {
	desktop.QuickLook(cx, []string{reportPath, sourcePath}, 0)
}
```

## Native Menüsymbole

`desktop.MenuItem.Icon` zeigt ein Symbol neben einem Eintrag in Anwendungs-, Dock-, Tray- und Popup-Menüs unter macOS, Windows und Linux. Ein `desktop.MenuIcon` nimmt ein geparstes Vektorsymbol (`SVG`, etwa `lucide.Folder()`), kodierte Bildbytes (`Data`, PNG, JPEG, GIF, WebP oder SVG) oder eine lokale/Asset-/Pixel-`core.ImageSource` (`Source`). Das Setzen von `Template` färbt ein einfarbiges Bild mit der Textfarbe des Menüs ein und folgt dabei der hellen und dunklen Darstellung (und unter macOS dem Hervorhebungszustand); einfarbige `SVG`-Symbole werden standardmäßig als Templates behandelt.

```go
desktop.SetApplicationMenu(cx, []desktop.MenuItem{{
	Label: "File",
	Items: []desktop.MenuItem{
		{Label: "Open Folder…", Icon: desktop.MenuIcon{SVG: lucide.Folder()}, OnSelect: openFolder},
		{Label: "Export", Icon: desktop.MenuIcon{Data: exportPNG, Template: true}, OnSelect: exportFile},
	},
}})
```

Symbole werden synchron aus lokalen Quellen geladen; entfernte URLs und ungültige Bilder lassen den Eintrag ohne Symbol. Menübeschriftungen, Tastenkürzel und Häkchen-/Optionsindikatoren behalten ihr natives Verhalten.

In Tests zeichnet `nsgotest.MenuItem.Icon` das Symbol jedes Eintrags in `app.Menu`, `app.DockMenu` und `app.PopupMenu` auf.

## Fotos und Dateien

`cx.PickImages` zeigt die Fotoauswahl des Systems und antwortet mit `core.PickedFile`s, deren Bytes, Namen, Typ und Pixelgröße die App liest. `cx.TakePhoto` zeigt die Kamera, wo die Plattform eine hat; `cx.Env().CanTakePhoto` sagt, ob das der Fall ist. Unter macOS, Windows und Linux zeigt es die Kameravorschau des Scanners mit einem Button „Foto aufnehmen“. `cx.ScanCode` öffnet eine Kameravorschau und antwortet mit dem ersten Code, den es liest, einem QR-Code, sofern `ScanOptions.Kinds` nicht nach anderen fragt, oder mit nil, wenn der Benutzer sie schließt; `cx.Env().CanScanCode` sagt, ob die Plattform einen Scanner hat: auf macOS-, iOS- und Android-Geräten mit Kamera, auf Windows-Rechnern mit einer Kamera, auf Linux-Desktops mit dem Camera-Portal und GStreamer `pipewiresrc` sowie in Browsern mit `getUserMedia` und `BarcodeDetector`. macOS zeigt ein Sheet, wenn das anfordernde Fenster sichtbar ist, Windows und Linux einen modalen Dialog über dem Fenster und das Web ein Overlay über der Seite. Tragen Sie `NSCameraUsageDescription` für macOS und iOS in `apple.info` ein und `CAMERA` für Android in `android.permissions`. Android, Windows und Linux lesen jede Art außer PDF417. Behandeln Sie den Text des Codes als nicht vertrauenswürdige Eingabe.

Unter tvOS antwortet `PickImages` ohne Bilder und `TakePhoto` gibt `core.ErrNoCamera` zurück; NSGo stellt die Continuity-Kamera-Auswahl nicht bereit.

In Tests erhalten `nsgotest.App.PickImages`, `nsgotest.App.TakePhoto` und `nsgotest.App.ScanCode` `nsgotest.ImagePick`-Datensätze für Auswahlen und Aufnahmen sowie `nsgotest.CodeScan`-Datensätze für Scans und antworten anstelle der System-UI. Die Hooks funktionieren headless und unter unterstützten `nsgo test -target`-Hosts. `nsgotest.Environment(core.Environment{Camera: true, ScanCode: true})` gibt einem Ziel ohne Kamera eine.

```go
cx.PickImages(core.PickOptions{Limit: 1, MaxDimension: 2048}, func(files []core.PickedFile) {
	if len(files) == 1 {
		avatar.Set(files[0].Data)
	}
})
```

`cx.PickFiles` wählt Dateien beliebigen Typs, `cx.PickDirectories` wählt Ordner als `fs.FS`, und `cx.SaveFile` speichert eine Datei oder Bytes über den Speichern-Dialog des Systems. Beide Auswahldialoge nehmen einen `Title`, eine `Button`-Beschriftung und einen `Start`-Ordner, und `PickFilesOptions.TypesName` benennt das Typmenü, sodass eine App sie in ihrer eigenen Sprache anzeigen kann; das sind Hinweise, denen macOS, Windows und Linux folgen, iOS nur für `Start` und Android und das Web überhaupt nicht. `cx.PickScreenColor` wählt eine Farbe an beliebiger Stelle auf dem Bildschirm, wo `cx.Env().CanPickScreenColor()` true ist: unter macOS, Windows, Linux mit dem Screenshot-Portal des Desktops und in Browsern mit `EyeDropper`.

Setzen Sie `core.SaveOptions.Start` auf einen vorhandenen absoluten Ordner, um den Speichern-Dialog unter macOS, Windows, Linux und iOS dort zu öffnen. Bei einem leeren oder unbrauchbaren Hinweis oder auf anderen Zielen wählt das System den Ordner.

Setzen Sie `core.SaveOptions.Accept`, um das gewählte Ziel vor dem Schreiben zu prüfen. Es läuft auf der Goroutine der Engine. Gibt es einen Fehler zurück, bleibt das Ziel unberührt. Unter macOS, Windows und im prozessinternen GTK-Auswahldialog unter Linux zeigt der Dialog den Grund an und lässt den Benutzer einen anderen Pfad wählen. Ein Abbruch nach einer Ablehnung meldet `saved == false`, den abgelehnten Pfad und den Fehler. Unter Android und im Linux-Desktop-Portal schließt eine Ablehnung das Speichern mit diesem Ergebnis ab. Android lässt den Benutzer einen Ordner wählen und liefert eine Baum-URI mit dem angeforderten Dateinamen als Abfrageparameter `name`. Es erstellt das Dokument nach der Zustimmung und meldet bei Erfolg die URI des neuen Dokuments. Die Plattform bricht das Schreiben ab, wenn die Prüfung nicht innerhalb von fünf Sekunden antwortet. iOS und HarmonyOS melden einen Fehler für Speichervorgänge, die eine Prüfung erfordern. Das Web hat keinen Zielpfad und verwendet seinen gewöhnlichen Speicherablauf.

In Headless-Tests erhalten `App.PickFiles`, `App.PickDirectories` und `App.SaveFile` `nsgotest.FilePick`-, `nsgotest.DirectoryPick`- und `nsgotest.FileSave`-Datensätze. Dieselben Datensätze stehen in `app.FilePicks`, `app.DirectoryPicks` und `app.Saves`.

### Gebündelte Assets

Legen Sie Dateien im Verzeichnis `resources/` der App ab und verwenden Sie `cx.ReadAsset("asset:maps/level.bin", done)`, um sie aus Go zu lesen. Der Callback erhält `([]byte, error)` nach dem Lesen auf der Goroutine der Engine und wird verworfen, wenn sein Besitzer verschwindet oder erneut ausgeführt wird. Verwenden Sie `errors.Is(err, core.ErrAssetNotFound)`, um fehlende Dateien zu erkennen. Das Lesen lädt die gesamte Datei in den Speicher.

## PCM-Audio abrufen

Verwenden Sie `audio.NewOutput(cx, audio.OutputConfig{SampleRate: 48000, Channels: 2, Render: render})`, um verschachteltes Float32-PCM abzuspielen, das Ihre App dekodiert oder synthetisiert. `Start` beginnt, Samples abzurufen, `Pause` stoppt das Abrufen, und der besitzende Scope schließt die Ausgabe, wenn er endet. macOS, iOS, Android, Windows, Linux und das Web unterstützen diesen Stream. Andere Ziele geben `audio.ErrUnsupported` zurück.

`Render` gibt die Anzahl der gefüllten Frames zurück. Es läuft auf dem Audio-Thread und muss Blockieren, Allokationen und Zugriffe auf den Graphen vermeiden. Eine unvollständige Füllung spielt für den Rest Stille und erhöht `Underruns`. `Position` gibt die monotone Frame-Anzahl und die Host-Zeit bei diesem Frame zurück. Verwenden Sie `Flush`, um ausstehende Frames zu verwerfen, und `Latency`, um die Pufferlatenz zu lesen. Routenwechsel und Unterbrechungen pausieren die Ausgabe und rufen `OnInterrupt` in der Engine auf. Die App kann sie mit `Start` fortsetzen.

## Speicher

`cx.Storage()` ist der Schlüssel-Wert-Speicher der App, der auf dem Gerät gehalten und von jedem Graphen der App geteilt wird. Lesevorgänge sind synchron, und ein Lesevorgang innerhalb einer Berechnung abonniert diese für Änderungen; `SetItem` gibt jenseits des Kontingents (`core.StorageQuota`, standardmäßig 5 MiB) einen `core.StorageFullError` zurück. Es ist eine einfache Datei, die jedes Programm des Benutzers lesen kann, also bewahren Sie Tokens und Passwörter stattdessen in [Geheimnissen](#secrets) auf.

`cx.Store()` gibt den zugrunde liegenden Speicher zurück, für einen Host, der mit `core.RootOptions.Storage` eine weitere Root erstellt, etwa einen, der ein Plugin auf einer eigenen Goroutine ausführt: Beide Roots teilen dann die Einträge der App und das, was auf dem Gerät gehalten wird. Es ist der gesamte Speicher, nicht die Partition, die `cx.ProvideStorage` Code gibt, den die App nicht geschrieben hat.

Ein Host, der seine eigene Persistenz bereitstellt, ruft `core.NewStore(backend)` mit einem `core.StorageBackend` auf: `Load() (map[string]string, error)` lädt einmal, und `Write(core.StorageChange)` erhält Änderungen der Reihe nach. Unterscheiden Sie nach `change.Op` (`core.StorageSet`, `core.StorageRemove`, `core.StorageClear`) und verwenden Sie `change.Key` und `change.Value`. Übergeben Sie `nil` für einen Speicher, der nur im Arbeitsspeicher gehalten wird.

```go
store := cx.Storage()
if tab, ok := store.GetItem("tab"); ok {
	selected.Set(tab)
}
store.SetItem("tab", "library")
```

`store.Keys()` listet die Schlüssel auf, `RemoveItem` löscht einen, und `Clear` leert den Speicher. `cx.DataDir()` und `cx.CacheDir()` liefern die Ordner der App für Dateien, für die der Schlüssel-Wert-Speicher zu klein ist; `cx.LogsDir()` liefert das Verzeichnis der Plattform für Logdateien, und ein `Scope.ProvideStorage` gibt einem Teilbaum eine eigene Partition.

Ein iOS App Clip und seine vollständige App teilen sich diese Daten- und Speicherorte, wenn `app_clip` und eine explizite `apple.app_group` konfiguriert sind; ihre Caches bleiben getrennt. Das Aktivieren der Gruppe migriert keine vorhandenen Sandbox-Daten. [App-Clip-Speicher](building.md#app-clips) behandelt Konfiguration und Signierung.

Unter HarmonyOS funktionieren Go-Dateioperationen wie `os.ReadFile`, `os.WriteFile` und `os.Rename` unter diesen Verzeichnissen und `os.TempDir()`. Relative Pfade beginnen im Dateiverzeichnis. Advisory-Dateisperren und symbolische Links sind nicht verfügbar.

### Absturzberichte

Native Release-Builds bewahren Go-Absturz-Traces unter `cx.LogsDir()/crashes` auf. Apple zeichnet nicht abgefangene Objective-C-Ausnahmen auf, Android nicht abgefangene Java-Ausnahmen und Windows SEH-Ausnahmen, die seinen Top-Level-Filter erreichen. Listen Sie ausstehende Berichte beim nächsten Start mit `cx.CrashReports()` auf. Jeder Bericht gibt die Version, den Build und die Plattform an, die ihn erzeugt haben. Rufen Sie `Open()` auf, um den Diagnosetext zu lesen, schließen Sie den Reader und rufen Sie dann `Acknowledge()` auf, um den Bericht zu löschen, sobald die App ihn verarbeitet hat. Führen Sie diese Dateioperationen auf einer Goroutine aus.

## Speicherplatz

`cx.DiskSpace(func(space core.DiskSpace, err error) { … })` fragt das Volume, das `cx.DataDir()` enthält, asynchron ab und ruft auf der Goroutine der Engine zurück. `Total` ist die vom System gemeldete Kapazität, `Available` ist das, was die App für eine vom Benutzer angeforderte Operation nutzen kann, und `Opportunistic` ist das, was sie für Caches und Vorabrufe nutzen kann. Alle drei sind in Bytes angegeben. `Available` schließt Speicher ein, den das System bei Bedarf freigibt, wo es diesen Wert meldet. Plattformen mit nur einem Wert für freien Speicher geben ihn in beiden Feldern zurück. Im Web ist `Total` das geschätzte Speicherkontingent des Browsers für den Origin und `Available` der ungenutzte Anteil. Ist der Dienst nicht verfügbar, gibt es `core.ErrDiskSpaceUnavailable` zurück.

Apple-Builds verlangen, dass `apple.privacy.accessed_apis` in `nsgo.yaml` `NSPrivacyAccessedAPICategoryDiskSpace` deklariert. Deklarieren Sie `E174.1`, um vor dem Schreiben den Speicherplatz zu prüfen oder bei knappem Speicher Dateien freizugeben, mit einer Änderung, die der Benutzer wahrnehmen kann. Deklarieren Sie `85F4.1`, um dem Benutzer den verfügbaren Speicher anzuzeigen. Die App muss Apples Einschränkungen für die Nutzung und Weitergabe der Messwerte einhalten. Ein Build ohne die Kategorie gibt `core.ErrNotDeclared` zurück. [Apples zugelassene Gründe](https://developer.apple.com/documentation/bundleresources/app-privacy-configuration/nsprivacyaccessedapitypes/nsprivacyaccessedapitype)

Legen Sie die Antwort eines Tests mit `nsgotest.DiskSpace(space, err)` fest.

## Cloud-Speicher

Das Paket `cloud` synchronisiert kleine Schlüssel-Wert-Einträge (`cloud.Get`, `cloud.Set`, `cloud.Delete`, `cloud.Keys`) und größere Dateien oder Spielstand-Snapshots (`cloud.ReadFile`, `cloud.WriteFile`, `cloud.DeleteFile`, `cloud.ListFiles`) geräteübergreifend: iCloud Key-Value Store und Ubiquity-Dokumentcontainer unter macOS und iOS sowie Google Play Games Saved Games unter Android.

Jede Operation liefert ihren Callback auf der Goroutine der Engine und überspringt ihn, wenn `cx` verworfen wurde. `cx.Env().CanUseCloudStorage()` meldet, ob Cloud-Speicher verfügbar ist; wenn der Benutzer abgemeldet ist, Entitlements fehlen oder die Plattform kein natives Cloud-Backend hat (Windows, Linux und das Web), gibt jeder Aufruf `cloud.ErrUnavailable` zurück und verwirft Schreibvorgänge niemals stillschweigend.

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

`cloud.WatchChanges` und `cloud.OnConflict` liefern externe Änderungen von mehreren Geräten und Versionskonflikte auf der Goroutine der Engine und räumen sich automatisch auf, wenn `cx` verworfen wird. Das Lesen einer Datei mit Konflikt gibt einen `*cloud.ConflictError` zurück (der zu `errors.Is(err, cloud.ErrConflict)` passt).

Um Apples iCloud-Entitlements in `nsgo build` zu aktivieren, fügen Sie `nsgo.yaml` einen Abschnitt `cloud:` (oder `icloud:`) hinzu:

```yaml
cloud:
  kv_store: true
  containers:
    - iCloud.com.example.myapp
```

Das Signierprofil muss die angeforderten iCloud-Fähigkeiten gewähren. Unter macOS enthalten nur Mac-App-Store-Builds (`sign.macos_profile`) iCloud, da macOS es nur mit einem Provisioning-Profil gewährt; ein Developer-ID-Build lässt es mit einer Warnung weg. Unter Android benötigt Cloud-Speicher das Play Games Services v2 SDK in der App und einen angemeldeten Spieler.

In Desktop-Stores wie Steam synchronisiert Steam Auto-Cloud `cx.DataDir()` direkt, ohne zusätzlichen Code – schreiben Sie lokale Spielstanddateien einfach weiter dorthin und lassen Sie Steam das Verzeichnis synchronisieren.

In Tests gibt `nsgotest` jeder App einen isolierten Cloud-Speicher im Arbeitsspeicher: Schalten Sie die Verfügbarkeit mit `nsgotest.CloudAvailable` oder `app.SetCloudAvailable` um, simulieren Sie externe Aktualisierungen von mehreren Geräten mit `app.SimulateCloudChange` und testen Sie die Konfliktauflösung mit `app.SimulateCloudConflict`.

## Geheimnisse

`cx.Secrets()` ist der sichere Speicher der App für Refresh-Tokens, API-Schlüssel und Passwörter: `Get`, `Set` und `Delete` einer Zeichenkette pro Konto, die die Plattform schützt. Ein fehlendes Konto wird als `("", false, nil)` gelesen, und wo die Plattform keinen sicheren Speicher hat, wie im Web, gibt jeder Aufruf `core.ErrSecretsUnavailable` zurück; NSGo weicht niemals auf einfachen Speicher aus. Das System kann einen Aufruf warten lassen, wie es ein Schlüsselbund tut, der den Benutzer fragt, also rufen Sie sie abseits der Goroutine der Engine auf und reichen Sie das Ergebnis mit `cx.Post` zurück.

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

| Plattform | Wo ein Geheimnis aufbewahrt wird |
| --- | --- |
| macOS | Ein generisches Passwort im Anmelde-Schlüsselbund: `kSecAttrService` ist der Dienst, `kSecAttrAccount` das Konto |
| iOS, tvOS, watchOS | Ein generisches Passwort im Schlüsselbund, auf dieselbe Weise |
| Windows | Eine generische Anmeldeinformation der Anmeldeinformationsverwaltung, deren Ziel `service/account` ist, höchstens 2560 Bytes |
| Linux | Ein Element des Secret Service (GNOME Keyring, KWallet) mit den Attributen `service` und `account`; nicht verfügbar, wo die Sitzung keinen ausführt |
| Android | Verschlüsselt mit einem AES-Schlüssel des Android Keystore, in den privaten Dateien der App, außerhalb von Backups; Lesevorgänge greifen außerdem auf Werte zurück, die `expo-secure-store` in seine `SecureStore`-`SharedPreferences` verschlüsselt hat |
| WeChat-Miniprogramme | Von der Plattform verschlüsselter Speicher (`wx.setStorage` / `wx.getStorage` mit `encrypt: true`, Basisbibliothek 2.21.3+); auf älteren Basisbibliotheken nicht verfügbar |
| Web, andere Miniprogramme | Nicht verfügbar (`core.ErrSecretsUnavailable`): Browser haben keinen vom Betriebssystem geschützten clientseitigen Geheimnisspeicher (bewahren Sie Websitzungen in vom Server gesetzten `HttpOnly`-Cookies auf), und QQ-, Alipay- und Douyin-Miniprogrammen fehlt verschlüsselter Speicher |

Der Dienst ist standardmäßig die ID der App (`core.App().ID`). Eine App, die von einem anderen Framework umsteigt, hält ihre Benutzer angemeldet, indem sie die Elemente benennt, die dieses Framework geschrieben hat: `Service` und auf Apple-Plattformen `AccessGroup` und `Accessible`. Unter Android setzen Sie `Service` auf den `keychainService` von `expo-secure-store` (standardmäßig `"key_v1"`), um Werte zu lesen, die es in `SharedPreferences` verschlüsselt hat; Schreiben oder Löschen entfernt den migrierten Eintrag. Unter iOS lässt `core.SecretAfterFirstUnlock` Hintergrundarbeit ein Element lesen, während das Telefon gesperrt ist, und eine `ThisDeviceOnly`-Zugänglichkeit hält es aus Backups heraus. Unter macOS verschiebt eine `AccessGroup` die Elemente in den Data-Protection-Schlüsselbund, der einen Build benötigt, der von einem Team signiert ist, dessen Entitlement `keychain-access-groups` die Gruppe aufführt, konfiguriert unter `macos.entitlements` in `nsgo.yaml` sowohl für Mac-App-Store- als auch für Developer-ID-Builds.

```go
s := cx.Secrets()
s.Service = "app:no-auth" // the service an earlier build used
s.Accessible = core.SecretAfterFirstUnlockThisDeviceOnly
err := s.Set("refresh-token", token)
```

`nsgotest` hält die Geheimnisse einer App im Arbeitsspeicher, in `App.Secrets`, und `nsgotest.Secrets` startet einen Test mit einigen davon, sodass ein Test niemals den Schlüsselbund des Rechners berührt. Ein TSX-Paket erreicht den Speicher mit der Fähigkeit `secrets`, unter einem eigenen Dienst.

## Netzwerk

`cx.Env().Network()` meldet, ob das System eine nutzbare Route hat (`Online`) und ob es diese Route als getaktet oder teuer betrachtet (`Expensive`). Bindings folgen ihm wie jedem anderen Umgebungswert. `cx.OnNetworkChange(func(core.Network))` ruft den Handler bei jeder Änderung auf der Goroutine der Engine auf, solange sein Scope lebt. Die Route des Systems sagt nichts darüber aus, ob ein bestimmter Server antwortet. Ein Ziel, das die Konnektivität nicht melden kann, gilt standardmäßig als online, und unbekannte Kosten gelten standardmäßig als false.

Setzen Sie `core.Environment{Network: &core.Network{Online: false}}` über `nsgotest.Environment` oder `App.SetEnvironment`, um das Offline-Verhalten zu testen.

Gos eigenes `net/http` stellt Anfragen auf jedem Ziel, mit den TLS- und Proxy-Einstellungen des Systems. Signals gehören zur Goroutine der Engine, also erreicht eine Antwort von einer anderen Goroutine sie über `cx.Post`.

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

`cx.Post` führt seine Funktion auf der Goroutine des Graphen aus, wo Signals gesetzt werden dürfen. Unter Android fordert das Manifest die Internetberechtigung an, und Bild-Downloads werden auf der Festplatte zwischengespeichert.

`core.NewResource` tut dasselbe für einen Wert, der aus einer Quelle geladen wird. Sein Fetcher läuft auf einer eigenen Goroutine und lädt erneut, wenn sich die Quelle ändert:

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

`items.Get()`, `items.Err()` und `items.Loading()` sind Signals, denen Bindings folgen. Eine neue Abfrage bricht die noch laufende Anfrage über `ctx` ab, sodass eine späte Antwort niemals eine neuere ersetzt. `items.Refetch()` versucht es erneut und behält die letzte Liste, während geladen wird, und `items.Mutate` setzt sofort einen Wert für eine optimistische Aktualisierung.

`core.Suspense` zeigt einen Platzhalter, während die Ressourcen, die sein Inhalt liest, ihren ersten Wert laden, sodass ein Bildschirm kein eigenes Lade-Flag braucht:

```go
core.Suspense{
	Content:  func(cx *core.Scope) core.Node { return itemList(cx, items) },
	Fallback: func(*core.Scope) core.Node { return ui.Spinner{Label: "Loading"} },
}
```

Der Inhalt bleibt eingehängt, aber verborgen, während er wartet, sodass er sofort erscheint, wenn die Werte eintreffen. Ein erneutes Laden lässt ihn mit dem letzten Wert sichtbar.

### Lokale Dienste entdecken

Unter macOS, iOS, Android, Windows und Linux veröffentlicht `cx.AdvertiseService` einen lauschenden Port und TXT-Attribute. `cx.BrowseServices` meldet aufgelöste Dienste mit ihrem Host, Port und ihren TXT-Attributen. `Host` kann ein DNS-Hostname oder eine IP-Adresse sein. Wenn ein Dienst verschwindet, erhält der Callback ihn erneut mit gesetztem `Removed`. Prüfen Sie `cx.Env().CanDiscoverServices()` auf Plattformunterstützung. Beide Aufrufe geben eine Stopp-Funktion zurück und enden, wenn ihr Scope verworfen wird. Ein Plattformfehler wird asynchron auf der Goroutine der Engine gemeldet. Linux verwendet Avahi und meldet einen Fehler, wenn der Daemon nicht verfügbar ist.

Für Bonjour auf Apple-Plattformen deklarieren Sie die Diensttypen und erklären den Zugriff auf das lokale Netzwerk in `nsgo.yaml`. Das System verwaltet die Berechtigungsabfrage und merkt sich die Entscheidung des Benutzers.

```yaml
apple:
  info:
    NSLocalNetworkUsageDescription: Finds nearby devices to sync with.
    NSBonjourServices:
      - _example._tcp
```

Android verwendet `NsdManager`. Android 12 und früher sowie Android 13 vor T Extensions 7 benötigen `CHANGE_WIFI_MULTICAST_STATE` für die WLAN-Erkennung. Eigenständige Builds deklarieren diese Berechtigung. Wenn Sie NSGo in einen Android-Host einbetten, fügen Sie sie für diese Geräte dem Manifest des Hosts hinzu. Veröffentlichte TXT-Werte müssen auf Geräten ohne Androids Raw-TXT-API (API 37 oder T Extensions 22) gültiges UTF-8 sein. Das Durchsuchen bewahrt rohe TXT-Werte. Wenn ein Android-Host API 37 oder höher anvisiert, muss er unter Android 17 oder höher `ACCESS_LOCAL_NETWORK` deklarieren und anfordern.

Verwenden Sie `nsgotest.LocalServices`, um in einem Headless-Test Suchergebnisse bereitzustellen.

## Now Playing

Verwenden Sie `cx.SetNowPlaying(core.NowPlaying{...})`, um die aktuellen Medien der App im Kontrollzentrum und in der Menüleiste von macOS anzuzeigen. Geben Sie `Title`, `Artist`, `Album` und optional `Artwork` als `*core.ImageSource` an. Eine `Duration` von null bedeutet einen Livestream. Geben Sie die aktuelle `Position` und die Wiedergabe-`Rate` an (null während der Pause, eins bei normaler Geschwindigkeit). Veröffentlichen Sie erneut, wenn sich Titel, Position oder Rate ändern. Das System lässt die Position zwischen den Aufrufen weiterlaufen.

Führen Sie die unterstützten Transportsteuerungen in `Commands` auf, etwa `core.MediaPlay`, `core.MediaPause`, `core.MediaTogglePlayPause` und `core.MediaSeek`. Behandeln Sie Anfragen mit `cx.OnMediaCommand`. Der Callback erhält den `Kind` des Befehls und, bei einem Sprung, die angeforderte `Position`. Der neueste lebende Handler nimmt Befehle entgegen. Rufen Sie `cx.ClearNowPlaying()` auf, wenn die App mit der Wiedergabe fertig ist. Während die App manuell veröffentlicht, setzen gesteuerte Videos unter macOS ihre eigene Veröffentlichung aus. Prüfen Sie `cx.Env().CanShowNowPlaying()` auf Plattformunterstützung.

## Hintergrund-Downloads

Verwenden Sie `cx.Download` und `cx.OnDownloadUpdate` in eigenständigen iOS- und macOS-Apps sowie unter Android. Eingebettete Apple-Views haben keinen Übertragungs-Host. Android-Apps aktivieren `android.background_downloads: true` in `nsgo.yaml`. Starten Sie einen Download aus einer Benutzeraktion heraus, während die App sichtbar ist. Verwenden Sie ein absolutes Ziel mit einem vorhandenen übergeordneten Verzeichnis, in das die App schreiben kann.

```go
cx.OnDownloadUpdate(func(p core.DownloadProgress) {
    progress.Set(p)
})
task, err := cx.Download(core.DownloadRequest{URL: url}, path, core.DownloadOptions{
    CertificatePEM: serverCertificate,
})
```

Die Übertragung verwendet GET und akzeptiert optionale Anfrage-Header. Sie verwendet den normalen HTTPS-Vertrauensspeicher, sofern `CertificatePEM` nicht das Leaf-Zertifikat des Servers pinnt. Das Pinning verlangt außerdem einen passenden Hostnamen und ein gültiges Zertifikat. Verwenden Sie die endgültige URL, da der Dienst eine erfolgreiche Antwort von dieser URL verlangt.

Unter Apple benötigt ein selbstsigniertes Zertifikat unter einem öffentlichen Hostnamen eine App-Transport-Security-Ausnahme für diesen Host in `apple.info`. Die Übertragung prüft dennoch das genaue Zertifikats-Pinning, den Hostnamen und die Gültigkeit. Eine Verbindung zu einem lokalen NAS benötigt außerdem eine `NSLocalNetworkUsageDescription`, die den Zugriff erklärt, und unqualifizierte oder `.local`-Hosts verwenden `NSAllowsLocalNetworking`. [Apples ATS-Einstellungen](https://developer.apple.com/documentation/security/preventing-insecure-network-connections)

```yaml
apple:
  info:
    NSAppTransportSecurity:
      NSExceptionDomains:
        files.example.com:
          NSExceptionAllowsInsecureHTTPLoads: true
```

Der Android-Vordergrunddienst läuft weiter, nachdem die Activity den Bildschirm verlassen hat. Er zeigt eine Fortschrittsbenachrichtigung an, vorbehaltlich der Benachrichtigungsberechtigung von Android. Der Fortschritt verwendet Byte-Zählungen und setzt `Total` auf -1, bis er bekannt ist. Bei Erfolg wird das Ziel atomar ersetzt. `task.Pause()` behält die Teildatei, `task.Resume()` verwendet einen HTTP-Range, wenn der Server einen Validator liefert, und `task.Cancel()` entfernt die Teildatei.

iOS verwendet eine Hintergrund-URLSession des Systems, die Übertragungen fortsetzt, während die App angehalten ist. Die App verbindet ihre Sitzung beim Start erneut und veröffentlicht Dateien, bevor sie den Abschluss im Hintergrund bestätigt. Übertragungen unter macOS laufen im Prozess. Apple-Plattformen verwenden native Fortsetzungsdaten für pausierte Downloads und halten Anfrage-Metadaten im privaten App-Speicher, bis ein Endergebnis gespeichert ist. Ein Server, der nicht fortsetzen kann, startet die Übertragung neu.

Registrieren Sie `OnDownloadUpdate` beim Start der App erneut, um aktive Aufgaben und gespeicherte Ergebnisse zu sehen. Verwenden Sie `p.Task.Forget()`, nachdem Sie ein Endergebnis verarbeitet haben. Android kann den Dienst stoppen, zum Beispiel wenn seine Übertragungszeit im Vordergrund abläuft. Die App erhält dann `DownloadInterrupted`. Unter Windows, Linux, tvOS, watchOS und im Web gibt `Download` `ErrDownloadUnavailable` zurück.

## Spracherkennung und der Lebenszyklus

`cx.StartSpeech` erkennt Sprache mit der Spracherkennung des Systems, wo `cx.Env().CanRecognizeSpeech()` sagt, dass es eine gibt (macOS, iOS, Android, Windows und Browser, die `SpeechRecognition` oder `webkitSpeechRecognition` bereitstellen), und antwortet mit dem Transkript, während es sich ändert. Auf Apple-Plattformen deklarieren Sie `NSMicrophoneUsageDescription` und `NSSpeechRecognitionUsageDescription` in `apple.info`; unter Android fügen Sie `RECORD_AUDIO` zu `android.permissions` hinzu. In einem Headless-Test hält `app.Speech` eine `nsgotest.SpeechSession`, solange die Erkennung aktiv ist. Verwenden Sie `app.HearSpeech` und `app.EndSpeech`, um Ergebnisse zu liefern.

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

`cx.Env().Lifecycle()` sagt, ob die App aktiv, inaktiv oder im Hintergrund ist: auf Desktops das Fenster, in dem sich `cx` befindet, sodass der Inhalt eines minimierten Fensters pausieren kann, während ein anderes Fenster angezeigt wird, und im eigenen Scope der App alle ihre Fenster zusammen. `desktop.Minimized(cx)` sagt, ob dieses Fenster minimiert ist, und `desktop.FrontmostApp(cx)`, welche Anwendung auf dem Desktop im Vordergrund ist, die App selbst eingeschlossen, und ob ihr Fenster im Vollbild ist, so wie ein Desktop-Haustier oder ein Zeiterfassungsprogramm ihr folgt; `desktop.CanReadFrontmostApp(cx)` sagt, ob ihr gefolgt wird, unter macOS, Windows, X11 und unterstützten Wayland-Compositoren. `cx.OnQuit` räumt beim Beenden auf, wie auch immer das geschieht, mit 200 ms für seine Arbeit. `desktop.OnQuitRequest` kann einen Menüeintrag „Beenden“ auf dem Desktop oder `desktop.RequestQuit(cx)` abbrechen oder aufschieben. Unter Windows und Linux läuft es außerdem nach `OnClosing`, wenn das Schließen eines Fensters die App beenden würde. `desktop.RequestRestart(cx)` fragt dieselben Handler vor einem Neustart; `desktop.Restart(cx)` bleibt sofortig. Neustarts durch den Updater unter Linux und Windows fragen ebenfalls, sodass das Speichern sie abbrechen oder aufschieben kann. Wenn das System Abmelden, Neustart oder Herunterfahren meldet, ist `q.Forced()` true, und die App fährt über `OnQuit` fort. Prüfen Sie `Forced()`, bevor Sie den Benutzer um eine Entscheidung bitten. `cx.SetAppearance` erzwingt hell oder dunkel, `cx.SetLocale` wählt die Sprache, `cx.OnKey` bindet ein Tastenkürzel, das ruht, solange das, was sein Besitzer zeigt, verborgen ist oder unter einem Dialog liegt, sofern `core.KeyOptions{OverModals: true}` es nicht aktiv hält, das seine Taste vor dem Anwendungsmenü erhält, sofern `YieldsToMenu` sie nicht einem aktivierten Menüeintrag überlässt, der dasselbe Kürzel dort nimmt, wo der Fokus liegt (während ein Textfeld oder eine Web-View den Fokus hat, überlässt ein `MenuNotEditing`-Eintrag der Bindung ihre Taste und hält die Taste von späteren Einträgen mit demselben Kürzel fern, und das Menü unter Linux nimmt keine Taste, die ohne Control, Alt oder eine Logo-Taste gedrückt wird, sodass die Bindung eine solche Taste dort behält), das in dem Fenster wirkt, in dem sich `cx` befindet, sofern `AllSurfaces` es nicht für ein app-weites Tastenkürzel in jedem Fenster und Panel der App bindet, auch in später geöffneten, das in all diesen ruht, solange das, was sein Besitzer zeigt, in dem Fenster verborgen ist, in dem es gebunden ist, und das einem fokussierten Steuerelement die Tasten lässt, die es verwendet, einem Button sein Return und seine Leertaste und einem Schieberegler, einer Tab-Leiste oder einer Optionsgruppe ihre Pfeiltasten, sofern es nicht mit `core.KeyAlways` oder `core.KeyNotEditing` gebunden ist; `cx.OnOpenFiles` hört Dokumente, die unter macOS, Windows, Linux und iOS geöffnet werden. iOS importiert reguläre Dateien in temporären Speicher, der der App gehört; verschieben Sie sie nach `cx.DataDir()`, um sie zu behalten. Unter macOS stellt das Schließen des Fensters, das das Anwendungsmenü geliefert hat, das Menü des zuletzt verbliebenen Fensters wieder her oder das Standardmenü, wenn keines mehr übrig ist.

Unter Wayland erfordert die Verfolgung der App im Vordergrund Version 2 oder höher des wlr-foreign-toplevel-Protokolls des Compositors; andere Compositoren melden `CanReadFrontmostApp` als false. Ihre ID ist die Wayland-Anwendungs-ID, ihr Name stammt, sofern verfügbar, aus dem Desktop-Eintrag, und ihre PID ist 0. Paketierte Windows-Apps verwenden ihre AUMID anstelle eines Frame-Host-Pfads. macOS folgt dem Vollbild-Darstellungszustand von AppKit, anstatt ihn aus der Fenstergröße abzuleiten.

Eine Bearbeitungsrolle im Anwendungsmenü nimmt den fokussierten Editor vor ihrem `OnSelect`-Fallback. Unter macOS folgen Rückgängig und Wiederholen dem Verlauf dieses Editors, und Ausschneiden folgt seiner Auswahl, auch während die App inaktiv ist; ohne Editor bleibt der Fallback aktiviert.

Unter iOS geben Sie `cx.OnKeyWith` eine `core.KeyOptions.Title`-Funktion, um das Tastenkürzel im Tastaturmenü des Systems anzubieten, solange die Bindung auslösen kann. `Group` benennt sein Menü unter iPadOS 26 und höher. Die Funktionen können den Katalog der App lesen, sodass die Titel der Tastenkürzel der Sprache folgen. Verwenden Sie `app.KeyCommands()` in Tests, um die aktuelle Liste zu prüfen.

```go
cx.OnKey("mod+k", func() { palette.Set(true) })
cx.SetLocale("de")
cx.OnQuit(func(ctx context.Context) { saveDraft(ctx) })
```

`cx.Env().DeviceName()` ist der Name, den der Benutzer dem Gerät gegeben hat, und `cx.Env().DeviceModel()` sein Hardwaremodell, etwa `Mac15,7`, `iPhone15,2` oder `Google Pixel 8`, sodass eine App zwei gekoppelte Geräte unterscheiden kann. macOS, Android, Windows und Linux liefern beides; iOS liefert das Modell und den Namen nur einer App mit Apples Entitlement für den vom Benutzer vergebenen Gerätenamen. watchOS liefert seine Hardwarekennung, HarmonyOS seinen Marktnamen oder sein Produktmodell und Miniprogramme die Marke und das Modell der Plattform; diese Ziele lassen den vom Benutzer vergebenen Namen leer. Das Web liefert keines von beiden. Tests setzen sie mit `nsgotest.Environment`.

`cx.Env().ThermalState()` meldet `core.ThermalNominal`, `ThermalFair`, `ThermalSerious` oder `ThermalCritical` auf Apple-Plattformen und unter Android ab API 29. `cx.Env().LowPowerMode()` meldet `core.LowPowerOn` oder `LowPowerOff` auf Apple-Plattformen, unter Android, Windows und in Linux-Sitzungen mit power-profiles-daemon. Wo das System keinen Wert liefert, geben die Lesevorgänge `ThermalUnknown` und `LowPowerUnknown` zurück. Jeder Lesevorgang folgt Änderungen unabhängig, sodass eine App ihre Arbeitslast anpassen kann, während sie läuft. In Tests setzen Sie `ThermalState` und `LowPowerMode` in `core.Environment` über `nsgotest.Environment` oder `app.SetEnvironment`.

`desktop.CursorPoint(cx, func(p core.Point, available bool) { ... })` liest den Zeiger unter macOS, Windows und X11 auch außerhalb der Fenster der App. Es nimmt pro Aufruf eine Stichprobe. Der Punkt liegt in denselben Bildschirmkoordinaten wie `desktop.Monitors(cx)` und `Window.ShowAt`. Unter Wayland und auf anderen nicht unterstützten Zielen oder wenn dem Scope `core.AccessCursorPoint` fehlt, ist `available` false.

`desktop.SetWindowConstraints` begrenzt die Inhaltsgröße und steuert Größenänderung, Minimieren und Maximieren; setzen Sie `WindowOptions.Constraints` beim Öffnen eines Fensters oder verwenden Sie `nsgo.WindowConstraints` für das Hauptfenster. Linux wendet nicht feste Maxima unter X11 an und entfernt verbotene Minimieren-/Maximieren-Buttons aus der nativen Titelleiste von GTK unter X11 und Wayland. Wayland unterstützt als Obergrenze derzeit nur feste Größen.

`desktop.SetWindowLevel` lässt ein Fenster über den anderen schweben, und `desktop.SetWindowWorkspaces(cx, desktop.WindowWorkspacesAll)` oder `WindowOptions.Workspaces` hält es auf jedem Arbeitsbereich, wie es bei einem Desktop-Haustier, einem HUD oder einem Timer der Fall ist: unter macOS auf jedem Space und über den Vollbild-Spaces anderer Apps, außerhalb von Cmd-`, und unter Linux auf jedem virtuellen Desktop eines X11-Fenstermanagers. Windows und Wayland lassen es auf dem Arbeitsbereich, auf dem es geöffnet wurde, und `desktop.CanSetWindowWorkspaces` sagt, ob es wirkt: true unter macOS und unter einem X11-Fenstermanager, der `_NET_WM_DESKTOP` unterstützt. Ein Hauptfenster kommt auf jeden Arbeitsbereich, indem `desktop.SetWindowWorkspaces` aus seinem Inhalt aufgerufen wird. In Tests listet `nsgotest.App.Window` die Anfragen auf, und `nsgotest.Environment` setzt `CanSetWindowWorkspaces`.

`desktop.RegisterHotKey` bindet ein globales Tastenkürzel, das wirkt, während eine beliebige App im Vordergrund ist, unter macOS, unter Linux und in einer Windows-App, die mit `nsgo.Run(App)` ausgeführt wird. Der `State` seines Handles sagt, ob das System es angenommen hat: `HotKeyPending`, bis es antwortet, dann `HotKeyRegistered`, `HotKeyTaken`, wenn eine andere Registrierung die Kombination hält, oder `HotKeyUnsupported` für eine Taste oder eine Plattform ohne globale Tastenkürzel, sodass ein Kürzel-Rekorder nach einer anderen Kombination fragen kann. Unter macOS gilt eine Kombination nur dann als belegt, wenn die App sie bereits hält oder eine andere App sie exklusiv registriert hat. Unter Wayland bittet der Compositor den Benutzer, das Tastenkürzel zu binden: Es ist registriert, sobald er das getan hat, und belegt, wenn er oder der Compositor es auslassen. In Tests lassen `nsgotest.HotKeyTaken` und `nsgotest.HotKeyUnsupported` Kombinationen entsprechend antworten.

Mit `desktop.TitleBarHidden` bieten Windows und Linux das Anwendungsmenü als Popup an, wenn Alt allein gedrückt und losgelassen wird, oder mit F10. Alt mit dem ersten Buchstaben oder der ersten Ziffer einer Beschriftung der obersten Ebene öffnet dieses Untermenü oder wählt diesen Befehl, in der Schrift der Beschriftung. App-Bindungen behalten ihre Tasten; Shift+F10 bleibt die Kontextmenütaste, und andere Alt-Kombinationen bleiben beim System. Escape schließt das Popup und stellt den Tastaturfokus wieder her.

```go
shortcut := desktop.RegisterHotKey(cx, "alt+space", launcher.Toggle)
warning := core.Text{ContentFunc: func() string {
	if shortcut.State() == desktop.HotKeyTaken {
		return "Another app uses Alt+Space"
	}
	return ""
}}
```

Größenänderungen auf dem Desktop unter macOS, Windows und Linux warten bis zu 16 ms auf das neue Layout, bevor gezeichnet wird. Halten Sie reaktive Berechnungen und Event-Handler kurz, damit das Layout diese Frist einhalten kann; führen Sie langsame I/O auf einer Goroutine aus und übermitteln Sie Ergebnisse über `cx.Post`. Eine ausgelastete App setzt die Größenänderung fort und zeigt ihr Layout, sobald sie aufholt.

`desktop.SetWindowFrame` verschiebt ein Fenster, wenn `WindowFrame.Move` true ist, und verwendet dabei `X` und `Y` in Punkten von der oberen linken Ecke des Hauptbildschirms. Nicht gesetzte Abmessungen behalten ihre aktuellen Größen. Prüfen Sie `desktop.CanPlaceWindows`: macOS, Windows und Linux unter X11 unterstützen die Positionierung; ein gewöhnliches Wayland-Toplevel behält die Position des Compositors. `desktop.Frame` folgt auf diesen Desktops der Position des nativen Fensters.

Unter macOS und Windows lässt `WindowFrame{Animated: true}` ein sichtbares Panel sanft in seinen neuen Rahmen übergehen, einschließlich der Größe eines angedockten Panels. Verborgene Panels und reduzierte Bewegung wenden den neuen Rahmen sofort an. Gewöhnliche macOS-Fenster verwenden die Animation von AppKit.

Setzen Sie `desktop.WindowOptions.Parent` auf ein geöffnetes Fenster der App, um beide zu schließen, wenn das übergeordnete Fenster geschlossen wird. Auf Desktops bleibt das untergeordnete Fenster über seinem übergeordneten und folgt ihm, wenn es ausgeblendet oder minimiert wird; ein ausdrücklich ausgeblendetes untergeordnetes Fenster bleibt ausgeblendet, wenn das übergeordnete zurückkehrt. Unter macOS wird es außerdem über seinem übergeordneten Fenster zentriert, wenn `Frame` nicht gesetzt ist, und folgt ihm in den Vollbildmodus.

`WindowOptions.NoShadow` und `desktop.SetWindowShadow(cx, false)` entfernen auch den Schatten eines Windows-Fensters, dessen Titelleiste ausgeblendet ist und das seine eigene Fensterdekoration zeichnet; ein Fenster mit der Titelleiste des Systems behält seinen Rahmen und Schatten, ebenso ein Linux-Fenster, das sein Fenstermanager dekoriert, wo `NoShadow` nur den Schatten der eigenen Dekorationen von GTK entfernt.

`desktop.SetWindowPointerEvents` lässt den Zeiger durch ein Fenster zu den Fenstern darunter durch, von jeder beliebigen App: `WindowPointerNone` überall, wie es ein Head-up-Display tut, und `WindowPointerPainted` überall dort, wo keine View etwas zeichnet oder den Zeiger hört, wie rund um ein Desktop-Haustier, das auf sich selbst dennoch Drücken, Ziehen und Rechtsklicks annimmt; die Titelleiste, die das System zeichnet, gilt als nicht gezeichnet. `WindowOptions.PointerEvents` und `PanelOptions.PointerEvents` öffnen ein Fenster auf diese Weise. Eine `Desktop.WindowDrag`-Region zieht auch innerhalb eines drückbaren Vorfahren; ein näheres drückbares oder bearbeitendes Steuerelement behält seine eigene Eingabe, anstatt das Fenster zu ziehen. Ein Fenster, das keinen Zeiger annimmt, hört kein Drücken, also gibt die App ihn über ein Tastenkürzel, ein Tray-Symbol oder ein anderes Fenster zurück. Code, den ein `Access` hält, benötigt `core.AccessOverlay`.

```go
desktop.OpenPanel(cx, desktop.PanelOptions{Width: 600, Height: 400, PointerEvents: desktop.WindowPointerPainted}, Pet)
```

Unter macOS und Windows kann ein Bild in einem gezeichneten Fenster `Image.HitTestAlpha` (TSX `hitTestAlpha`) auf einen Wert über 0 und höchstens 1 setzen. Zum Beispiel nimmt 0.5 den Zeiger auf Bildpixeln an, die mindestens zur Hälfte deckend sind. Der Schwellenwert gilt auch für `OnPress` des Bildes. Ein View-Hintergrund oder -Rahmen gilt weiterhin als gezeichnet. Null verwendet die gesamte Bild-View, ebenso Bilder unter Linux.

In einem gezeichneten macOS-Fenster wendet `Video.HitTestAlpha` (TSX `hitTestAlpha`) denselben Schwellenwert auf dekodierte Videoframes an. Es funktioniert mit ausgeschalteten `Controls` und einer nicht transformierten Videospur, etwa HEVC mit Alpha für ein Desktop-Haustier. Null, ein nicht verfügbarer Frame und die anderen Desktop-Hosts verwenden die Grenzen der Video-View.

`desktop.NewTray` kann unter macOS und Windows mit `TrayOptions.View` ein lebendes Symbol zeichnen. Windows passt die transparente Darstellung der View an die Symbolgröße des Infobereichs an und aktualisiert sie, wenn sich die View ändert oder ein Bild fertig geladen ist; verwenden Sie `TrayOptions.OnClick`, `Panel` und `Menu` für seine Aktionen. Linux zeigt derzeit PNG-Symbole; das Zeichnen seiner Tray-View bleibt ein Ziel.

`desktop.OpenPanel` öffnet ein Panel, wie es ein solches Tastenkürzel oder ein Tray-Symbol zeigt: ein randloses Fenster über den anderen. Wie `desktop.OpenWindow`, `desktop.NewTray`, `mobile.OpenOverlay`, `audio.NewPlayer`, `audio.LoadSound` und `core.NewHolder` gehört es direkt zu dem `cx`, das Sie übergeben, und nicht zum Besitzer des laufenden Handlers, sodass das Übergeben des Scopes der App aus dem `OnPress` eines Buttons das Fenster, das Panel oder den Player am Leben hält, nachdem der Scope dieses Buttons endet; reaktive Primitive (`Effect`, `Cleanup`, `AfterFunc`, `OnKey`) gehören dagegen zum laufenden Besitzer, den `cx.Run` wechselt. Eine Popup-Karte zeichnet sich selbst auf einem Panel mit `Transparent`, das keinen eigenen Hintergrund, Rahmen oder Schatten hat, sodass die abgerundeten Ecken und der `BoxShadow` der Karte alles sind, was sichtbar ist, mit Platz für den Schatten innerhalb des Panels; `FitContent` passt die Größe des Panels an seinen Inhalt an, bis zu `Width` und `Height`, bevor es zum ersten Mal angezeigt wird und wenn sich der Inhalt ändert. `desktop.Transparent` sagt, ob das Panel jetzt nichts Eigenes zeichnet, wie bei einem transparenten Fenster, während `desktop.CanMakeWindowsTransparent` nur sagt, ob ein Fenster es sein könnte: Wo es false ist, etwa unter Linux ohne Compositor, geben Sie dem Inhalt einen Hintergrund.

Unter macOS verwendet ein Panel ohne `Transparent` eckige Ecken, wenn eine der beiden Abmessungen unter 24 Punkten liegt, und stellt bei normalen Größen seine abgerundeten Ecken wieder her.

```go
lookup := desktop.OpenPanel(cx, desktop.PanelOptions{Width: 380, Height: 520, Transparent: true, FitContent: true, HideOnBlur: true}, Card)
desktop.RegisterHotKey(cx, "alt+space", lookup.Toggle)
```

`desktop.NewTray` zeigt unter macOS `TrayOptions.Title` neben seinem Bild oder seiner gezeichneten View. Für ein eigenes Popup lesen Sie `tray.ClickBounds()` innerhalb von `OnClick` oder `Menu` und übergeben das verfügbare Rechteck an `panel.ShowAt(bounds, core.PlaceBottom)`. Es verwendet dieselben Bildschirmpunkte wie `Env.Displays` und schließt den Titel ein. Die Grenzen sind vor dem ersten Klick, nach dem Entfernen, unter Linux und dann nicht verfügbar, wenn die Windows-Shell sie nicht liefern kann. Jeder Klick ersetzt die vorherigen Grenzen, sodass ein nicht verfügbares Ergebnis niemals ein altes Rechteck wiederverwendet. Das Setzen von `TrayOptions.Panel` behält stattdessen das eingebaute Umschalten per Klick bei.

`PanelOptions.Dock` hält ein Panel über `Show`, `Focus`, `Toggle` und Größenänderungen des Inhalts hinweg zentriert an einer nutzbaren Kante eines Bildschirms. Setzen Sie `Edge` auf `core.PlaceTop`, `PlaceBottom`, `PlaceLeft` oder `PlaceRight`, `Display` auf eine `Env.Displays`-ID (leer für den primären Bildschirm) und `Margin` auf den Abstand von dieser Kante in Punkten. Ein angedocktes Panel kann `ShowAt` oder `MoveTo` nicht verwenden; `SetWindowFrame` ändert nur seine Größe. Ein Tray-Symbol zeigt es an seinem Andockplatz statt unter dem Symbol.

```go
panel := desktop.OpenPanel(cx, desktop.PanelOptions{
	Width: 320, Height: 80, NoActivate: true,
	Dock: desktop.PanelDock{Edge: core.PlaceBottom, Margin: 12},
}, Candidates)
panel.OnPlacementError(cx, func(err error) { /* offer another presentation */ })
panel.Show()
```

`desktop.CanDockPanels(cx)` meldet die Unterstützung für das Andocken getrennt von der freien Platzierung von Fenstern. macOS, Windows und X11 unterstützen es; Wayland benötigt die optionale Laufzeitbibliothek `libgtk4-layer-shell.so.0` und einen Compositor, der `zwlr_layer_shell_v1` implementiert. NSGo lädt diese Bibliothek vor GTK; ein einbettender Host, der Wayland bereits geladen hat, muss gtk4-layer-shell vor dem Start des Hosts vorladen. Eine fehlende Bibliothek, ein fehlendes Protokoll oder ein fehlender ausgewählter Bildschirm lässt das Panel verborgen und ruft `OnPlacementError` auf. Ein Wayland-Dock verwendet die oberste Ebene, ohne Arbeitsfläche zu reservieren: `NoActivate` lässt die Tastatur beim aktuellen Fenster, während ein Panel ohne diese Option exklusiven Tastaturfokus übernimmt, bis es verborgen wird. Gewöhnliche Wayland-Fenster behalten ihre übliche Platzierung durch den Compositor.

Mit einem Kürzel-Rekorder wählt der Benutzer eine solche Taste. Das `OnKeyDown` einer `Focusable`-View hört jede Taste, die gedrückt wird, während sie den Fokus hat, vor den Bindungen der App, mit den gehaltenen Modifikatoren, einschließlich eines allein gedrückten Modifikators; die Rückgabe von `true` hält die Taste von den Bindungen fern, sodass das Aufzeichnen von Command+S nicht speichert. `KeyEvent.Shortcut` schreibt das Drücken so, wie `OnKey` und `RegisterHotKey` es annehmen, jeden Modifikator mit Namen ("ctrl+shift+k", "alt+space", "shift+meta+k" für Command+Shift+K), und "" für einen Modifikator allein. Ihr `OnKeyUp` hört jede dieser Tasten beim Loslassen, auch nachdem `OnKeyDown` sie verbraucht hat oder der Fokus gewechselt ist, mit den noch gehaltenen Modifikatoren. Beide funktionieren unter macOS, Windows, Linux, im Web, wo der Browser die Tastenkürzel behält, die er keiner Seite überlässt, auf Apple TV und in `nsgotest`; Android liefert Hardware-Tastaturereignisse asynchron an fokussierte Views, sodass deren Rückgabewert von `OnKeyDown` die Taste nicht verbrauchen kann. iPhone und iPad unterstützen das Abfragen von Hardwaretasten. Ein Tastenkürzel, das die App global registriert hat, erhält seine Taste vor der View, also hebt ein Rekorder dessen Registrierung auf, während er aufzeichnet.

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

Ein Spiel oder ein Canvas fragt die Tasten stattdessen ab. `core.KeysDown(cx)` listet die im Fenster gehaltenen Tasten auf, und `core.KeyHeld(cx, "left")` sagt, ob eine gehalten wird: verfolgte Lesevorgänge, die auch eine `RequestAnimationFrame`-Schleife liest. Der erste Lesevorgang lässt die Plattform ab dann jede Taste melden, sodass eine bereits gehaltene Taste unbekannt bleibt, bis sie erneut gedrückt wird; Tasten, die in ein Feld oder eine Web-View getippt werden oder während eine Eingabemethode komponiert, werden nie als gehalten geführt, und eine Taste, die noch gehalten wird, wenn das Fenster den Fokus verliert, gilt dann als losgelassen. Sie funktionieren dort, wo `OnKeyUp` funktioniert, sowie auf iPhone und iPad mit einer Hardware-Tastatur.

Für Maustasten liefern `View.OnMouseDown` und `View.OnMouseUp` ein `core.MouseButtonEvent` für jede Taste (`core.MouseButtonLeft`, `core.MouseButtonRight`, `core.MouseButtonMiddle`, `core.MouseButtonBack`, `core.MouseButtonForward`) unter macOS, Windows, Linux, im Web und in `nsgotest`, ohne die View zu einem Tastaturfokus-Stopp zu machen. `core.MouseButtonsDown(cx)` und `core.MouseButtonHeld(cx, core.MouseButtonRight)` sind verfolgte Lesevorgänge der im Fenster gehaltenen Tasten, die automatisch zurückgesetzt werden, wenn das Fenster den Fokus verliert.

Für ein Steuerelement, das an eine physische Position gebunden ist, lesen Sie `KeyEvent.Code`, `core.KeyCodesDown(cx)` oder `core.KeyCodeHeld(cx, "KeyW")`. Der Code verwendet `KeyboardEvent.code`-Namen: Auf AZERTY hat die QWERTY-Position W `Code: "KeyW"` und `Key: "z"`. `core.KeyCodeLabel(cx, "KeyW")` liefert die Anzeigebeschriftung des aktiven Layouts für eine Eingabeaufforderung. Unter iOS, Android, HarmonyOS und in Browsern ohne Layout-Map verwendet es den zuletzt an dieser Position gemeldeten Tastennamen und gibt eine leere Zeichenkette zurück, bis eine Taste gesehen wurde. HarmonyOS benennt sowohl die Position als auch die Taste anhand des `KeyCode` von InputKit, da ArkUI keinen Scancode meldet, sodass der Tastenname dort einem Nicht-QWERTY-Layout noch nicht folgt.

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

## Den Bildschirm wach halten

`cx.KeepAwake("Playing video", core.KeepAwakeOptions{Display: true})` hält den Bildschirm eingeschaltet, bis die zurückgegebene Freigabefunktion ausgeführt oder der aktuelle Scope verworfen wird. Unter Android gilt das Halten, solange die App sichtbar ist. Im Web funktioniert es in einem sicheren Kontext mit Unterstützung für Screen Wake Lock, und die Seite fordert die Sperre erneut an, wenn sie sichtbar wird. Safari kann ein Drücken oder eine Taste verlangen, bevor es die erste Anfrage gewährt. Halten nur für den Rechner hat auf diesen beiden Plattformen keine Wirkung.

## Zeigersperre

Bei einer Spielkamera oder einem Ziehen, das am Bildschirmrand weitergehen muss, rufen Sie `ref.LockPointer()` aus einem Drücken auf der View auf, die die Ref hält. Der Zeiger bleibt an seinem Platz und ist verborgen, während die View relative Bewegungen in `OnPointerDelta(dx, dy)` erhält. Positive Werte bewegen nach rechts und unten.

Rufen Sie die zurückgegebene Funktion auf, um die Sperre aufzuheben. Escape, ein Fenster, das den Fokus verliert, oder das Ausblenden oder Entfernen der View heben sie ebenfalls auf. Prüfen Sie `cx.Env().CanLockPointer()`, um zu sehen, ob die Oberfläche sie unterstützt. Das Web verlangt eine Benutzergeste wie ein Drücken. Linux unter Wayland verlangt einen Compositor, der Zeigerbeschränkungen und relative Bewegung unterstützt.

## Sound-Grenzen

Verwenden Sie `audio.SetSoundLimits(cx, audio.SoundLimits{MaxSounds: 200, MaxDuration: 20 * time.Second})` vor dem Laden von Sounds, um die Grenzen der Engine zu ändern. `MaxEncodedBytes`, `MaxDecodedBytes` und `MaxPlaybacks` können ebenfalls gesetzt werden. Felder mit dem Wert null verwenden die Standardwerte von 8 MiB kodiert und 5 Sekunden pro Sound, 64 Quellen, 16 MiB dekodiert über alle Sounds hinweg und 32 sich überlappende Wiedergaben pro Ausgabe. Vorhandene Sounds bleiben geladen und zählen zu den neuen Grenzen. Eingeschränkte Scopes behalten die Standardgrenzen innerhalb des Budgets der Engine: Die `nsgo/audio`-Sounds eines signierten TSX-Pakets stoßen an sie, was auch immer die Go-App gesetzt hat, während das eigene TSX der App die Grenzen der Go-App übernimmt. Androids SoundPool spielt höchstens 32 Sounds gleichzeitig und dekodiert von jedem höchstens 1 MiB, unabhängig von den Grenzen.

## Vollbild

`desktop.SetFullScreen(cx, true, displayID)` füllt unter macOS, Windows und Linux den Bildschirm, der durch `cx.Env().Displays()` benannt wird. Lassen Sie die ID weg, um den aktuellen Bildschirm des Fensters zu verwenden. Eine unbekannte ID lässt das Fenster, wo es ist. Das Wiederholen derselben Anfrage hat keine Wirkung. `desktop.SetFullScreen(cx, false)` verlässt den Vollbildmodus und stellt den vorherigen Rahmen des Fensters wieder her. Im Web wechseln Sie aus einem Press-Handler in den Vollbildmodus. Der Browser wählt den Bildschirm, und `cx.Env().FullScreen()` folgt dem Ein- und Austritt, einschließlich Esc.

## Bildschirme

`cx.Env().Display()` gibt die aktuelle `core.DisplayInfo` des Fensters zurück und aktualisiert Bindings, wenn das Fenster auf einen anderen Bildschirm wechselt. `cx.Env().Displays()` listet die Bildschirme auf, die die Plattform meldet. `RefreshRate` und `MaxRefreshRate` sind in Hz angegeben, mit null für eine nicht verfügbare Rate. iOS meldet das Maximum des Displays. HarmonyOS meldet den Bildschirm des Fensters, seine aktuelle Rate und seine höchste unterstützte Rate. Das Web meldet den aktuellen Bildschirm und kennzeichnet seine Messung über Frame-Callbacks mit `RefreshRateEstimated`. In Tests setzen Sie `Displays` und `DisplayID` über `nsgotest.Environment`.

## Text vorlesen

`cx.Speak` liest Text mit der Stimme des Systems für eine Sprache vor, wo `cx.Env().CanSpeak()` sagt, dass die Plattform spricht: macOS, iOS, tvOS, Android, Windows, das Web und Linux mit speech-dispatcher. Es wird jeweils eine Äußerung gesprochen: Ein weiteres `Speak` unterbricht die gerade gesprochene, ebenso `cx.StopSpeaking` und das Ende des Scopes, der sie gesprochen hat. `done` erfährt einmal, wie sie geendet hat: beendet, gestoppt oder fehlgeschlagen mit einem `core.SpeakError`, etwa `core.SpeakLanguageUnavailable`, wenn keine installierte Stimme die Sprache spricht.

```go
cx.Speak("Guten Morgen", core.Voice{Lang: "de-DE", Rate: 0.8}, func(stopped bool, err error) {
	speaking.Set(false)
})
```

`Rate` und `Pitch` reichen von 0,5 bis 2, wobei 1 der eigene Wert der Stimme ist. `cx.Voices("ja", ...)` listet die für eine Sprache installierten Stimmen auf, sodass die App den Benutzer bitten kann, eine fehlende zu installieren, oder ihn eine auswählen lassen kann, deren `ID` in `core.Voice` eingetragen wird. Browser sprechen erst, nachdem der Benutzer etwas auf der Seite gedrückt hat, und schlagen vorher mit `core.SpeakNotAllowed` fehl. Die Go-Komponenten eines TSX-Pakets sprechen nur mit der Fähigkeit `text_to_speech`.

## Audio in Schleife

Setzen Sie `audio.Voice.Loop`, um eine Sound-Wiedergabe bis zu `Stop` oder `Close` zu wiederholen. Für gestreamte Musik setzen Sie `audio.PlayerOptions.LoopStart` und rufen `SetLoop(true)` auf. Der erste Durchlauf schließt das Intro ein. Spätere Durchläufe beginnen am Offset. Ein Offset von null wiederholt den gesamten Titel.

Unter macOS 14 und iOS 17 oder später verwendet ein endlicher Titel die native Warteschlange, um jede Wiederholung vorzubereiten, bevor die aktuelle endet. Frühere Apple-Versionen springen am Ende des aktuellen Durchlaufs zum Schleifenstart, mit einer kurzen Lücke. Unter Linux und im Web dekodiert ein Player in Schleife einen Titel von bis zu 32 MiB und 5 Minuten in den Speicher, etwa 23 MB pro Minute, und wiederholt ihn ohne Lücke. Unter Linux wird ein größerer oder längerer Titel über einen eigenen Decoder gestreamt und ebenfalls ohne Lücke wiederholt; im Web springt er am Ende zurück, mit einer kurzen Lücke. Unter Windows und Android springt eine Schleife mit einem Startpunkt am Ende zu diesem zurück, mit einer kurzen Lücke. Im Web springt ein Player in Schleife mit einer anderen Rate als 1 am Ende ebenfalls zurück, da die lückenlose Wiedergabe des Browsers die Tonhöhe mit der Rate verschieben würde. Im Web spielt ein Player in Schleife, wie ein Sound, erst, nachdem der Benutzer etwas auf der Seite gedrückt hat, und schlägt vorher mit `NotAllowed` fehl.

## Biometrie

`cx.BiometricAvailability` meldet, ob Face ID, Touch ID oder ein Fingerabdruck des Geräts eingerichtet ist; Android sagt nicht, welche Art eingerichtet ist, also meldet es dort einen Fingerabdruck, wenn einer eingerichtet ist, und andernfalls Gesicht, wenn das Gerät Gesichtserkennungs-Hardware hat. `cx.AuthenticateBiometric` bittet das System, den Benutzer mit einer Begründung zu bestätigen, die nicht leer sein darf, und meldet Erfolg, Fehlschlag, Abbruch oder Nichtverfügbarkeit. Es wählt für die App keinen Passcode als Ausweichlösung.

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

Face-ID-Apps deklarieren `NSFaceIDUsageDescription` unter `apple.info`; Android-Apps führen `USE_BIOMETRIC` unter `android.permissions` auf.

## Anmeldung

`cx.AuthSession` meldet den Benutzer bei einem OAuth-Anbieter in der Authentifizierungssitzung des Systems statt in einer Web-View an: `ASWebAuthenticationSession` unter macOS, iOS und watchOS, ein Custom Tab oder Browser unter Android, der Standardbrowser unter Windows, Linux und auf einem Mac ohne Schema sowie ein Popup im Web. Im Web sendet die Weiterleitungsseite ihren Callback außerdem über einen `BroadcastChannel` desselben Origins, sodass eine Anmeldung abgeschlossen wird, wenn die `Cross-Origin-Opener-Policy` des Anbieters das Popup von der Seite trennt. Ein solches Popup erscheint der Seite als geschlossen, sodass das Schließen eines Popups, nachdem es die Seite des Anbieters gezeigt hat, die Sitzung erst nach 5 Minuten als abgebrochen beendet; geben Sie der Sitzung einen Abbrechen-Button. Die App behält das Protokoll: Sie erzeugt den PKCE-Verifier und den State, baut die Autorisierungs-URL für die Weiterleitung, die sie erhält, und prüft den Callback, bevor sie seinen Code eintauscht. Abbrechen ist kein Fehlschlag: `errors.Is(err, core.AuthCancelled)` erkennt, dass der Benutzer die Sitzung geschlossen hat oder das zurückgegebene `cancel` sie beendet hat.

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

Die Weiterleitung ist `com.example.app:/oauth2redirect`, wo ein Schema gesetzt ist (Windows, Linux und Android benötigen es in den `url_schemes` von `nsgo.yaml`, damit das Betriebssystem den Callback an die laufende App übergibt; die Systemsitzung von macOS, iOS und watchOS fängt ihn ohne diese Deklaration ab), und `http://127.0.0.1:<port>/oauth2redirect` über den Loopback der Desktops, also registrieren Sie beide beim Anbieter, oft als eigene Clients. Lassen Sie `Scheme` leer für ausschließlich Loopback oder setzen Sie es auf `https` mit `Host` für die zugeordnete Domain der App, die iOS 17.4, macOS 14.4 und watchOS 10.4 abfangen. watchOS verlangt ein Schema oder eine zugeordnete HTTPS-Domain; es hat keinen Loopback als Ausweichlösung, und das bestehende Minimum von watchOS 26 für NSGo gilt weiterhin. Ein geschlossener Browser-Tab teilt einer Loopback-Sitzung nichts mit, also geben Sie ihr ein `Timeout` oder einen Abbrechen-Button. `Ephemeral` bittet macOS, iOS und watchOS, die Cookies des Browsers nicht zu teilen, und fordert unter Android einen privaten Custom Tab an, wo der Browser einen unterstützt; gehen Sie unter Android nicht von einer Cookie-Isolation aus. tvOS und Miniprogramme antworten vorerst mit `core.AuthUnavailable`. In TSX benötigt `AuthSession.start(url, options)` die Fähigkeit `auth_session` und liefert eine Sitzung, deren `result` mit dem Callback aufgelöst wird oder mit null, sobald abgebrochen wurde.

Android verbraucht einen passenden Callback vor dem gewöhnlichen `OnOpenLinks`, und eine Rückkehr ohne Callback beendet die Sitzung als abgebrochen. Sein Callback muss ein privates Schema verwenden, das der App gehört; ein leeres Schema, ein HTTPS-Callback oder ein nicht deklariertes Schema gibt `core.AuthUnavailable` zurück, ebenso eine Android-Laufzeit, die vor den Anmeldesitzungen gebaut wurde.

Eine Browsersitzung verbraucht nur ihren passenden Callback-Link; nicht zugehörige und verspätete Links erreichen weiterhin `OnOpenLinks`. Das Starten einer weiteren Browsersitzung für dieselbe Callback-URI bricht die frühere ab; unterschiedliche URIs können in verschiedenen Fenstern warten.

Verwenden Sie genau die Weiterleitung, die an die Funktion für die Autorisierungs-URL übergeben wird. Web- und Desktop-Loopback-Weiterleitungen kodieren Unicode-Pfade und lösen Punktsegmente auf, bevor die Funktion sie erhält; ein Loopback-`Path` ist dekodierter Text, sodass auch ein literales `%` maskiert wird.

Miniprogramme verwenden stattdessen `cx.SignInMiniProgram(core.MiniProgramSignInOptions{}, done)`. Der Callback erhält ein `core.MiniProgramCredential` mit seiner `Platform` (`MiniProgramWeChat`, `MiniProgramQQ`, `MiniProgramDouyin` oder `MiniProgramAlipay`) und einem nicht leeren, kurzlebigen `Code`. Senden Sie ihn an Ihren Server für den Code-Austausch des Hosts; bewahren Sie das App-Geheimnis dort auf. Das ist `wx.login`/`qq.login`/`tt.login` oder Alipays grundlegende `auth_base`-Autorisierung, ohne Profilinformationen anzufordern. Es öffnet keine URL und führt keinen OAuth-URL-Builder aus.

`cx.Env().CanSignInMiniProgram()` meldet die Verfügbarkeit der Host-API. Andere Ziele antworten mit `core.MiniProgramSignInUnavailable`; Code ohne `core.AccessMiniProgramSignIn` erhält `AccessDenied`. Douyin fragt standardmäßig still: Ein nicht angemeldeter Host gibt `MiniProgramSignInNotSignedIn` zurück, und `MiniProgramSignInOptions{Prompt: true}` erlaubt sein Anmeldefenster für das Host-Konto. Das Schließen dieses Fensters gibt `MiniProgramSignInCancelled` zurück. Ein anonymer Gerätecode zählt nie als angemeldeter Benutzer. Tests setzen `nsgotest.Environment(core.Environment{MiniProgramSignIn: true})` und `app.MiniProgramSignIn`; `app.MiniProgramSignIns` zeichnet Anfragen auf. TSX läuft nicht in Miniprogrammen.

Eine App, deren Web-Views so angemeldet sein sollen, wie der Benutzer es anderswo bereits ist, etwa in seinem eigenen Browser, schreibt die Cookies dieser Anmeldung mit `cx.SetWebCookies`, einschließlich HttpOnly-Cookies, die Seitenskripte nicht setzen können, und lädt dann die Seiten: done läuft, sobald der Speicher sie enthält. Der Speicher ist der persistente, den jede Web-View der App teilt, sodass Cookies mit einem `Expires` die App überdauern; Inkognito-Views sehen sie nicht, außer unter Android, das einen Speicher pro App hält. HarmonyOS, das Web, tvOS und watchOS antworten mit `Unavailable`.

```go
cx.SetWebCookies([]core.WebCookie{
	{Name: "SID", Value: sid, Domain: ".example.com", Expires: expires, Secure: true, HTTPOnly: true},
}, func(err error) {
	if err == nil {
		page.Set(core.WebSrc("https://www.example.com/"))
	}
})
```

Um ein Cookie zu entfernen, schreiben Sie es erneut mit demselben Namen, derselben Domain und demselben Pfad und einem `Expires` in der Vergangenheit; ein Ablaufdatum von null ergibt ein Sitzungscookie. Warten Sie auf den Callback, bevor Sie eine Seite laden, die die Entfernung bemerken soll.

Umgekehrt listet `cx.WebCookies` die Cookies des Speichers auf, einschließlich HttpOnly-Cookies und ihrer Domain, ihres Pfads, ihres Ablaufdatums und ihrer Flags, die `document.cookie` nicht lesen kann, unabhängig davon, ob eine Seite der Website angezeigt wird: Ein `core.WebCookieFilter` wählt diejenigen aus, die eine Anfrage an eine `URL` senden würde, oder die einer `Domain` und jedes Hosts darunter, und der Nullfilter alle. Eine App, die die Website aus Go mit der Anmeldung des Benutzers aufruft oder anzeigt, wo der Benutzer angemeldet ist, liest sie auf diese Weise. `cx.ClearWebsiteData` meldet den Benutzer von jeder Website ab: Es entfernt die Cookies des Speichers, Local und Session Storage, IndexedDB, Service Worker und Caches, und done läuft, sobald sie weg sind. Androids `CookieManager` antwortet nur mit den Namen und Werten, die eine URL senden würde, sodass dort nur ein `URL`-Filter auflistet, wobei `Domain` mit seinem Host gefüllt wird und die anderen Felder leer bleiben, und ein `Domain`-Filter oder der Nullfilter mit `Unavailable` antwortet; das Löschen lässt dort den HTTP-Cache bestehen. HarmonyOS, das Web, tvOS und watchOS antworten auf beides mit `Unavailable`.

```go
cx.WebCookies(core.WebCookieFilter{Domain: "example.com"}, func(cookies []core.WebCookie, err error) {
	signedIn.Set(err == nil && slices.ContainsFunc(cookies, func(c core.WebCookie) bool { return c.Name == "SID" }))
})
```

## Mit Apple anmelden

`cx.SignInWithApple` zeigt das Sheet „Mit Apple anmelden“ des Systems über den eigenen Button der App unter macOS, iOS und tvOS, wo `cx.Env().CanSignInWithApple()` true ist. Es antwortet einmal mit den Anmeldedaten oder mit einem Fehler: `core.AppleSignInCancelled`, wenn der Benutzer das Sheet schließt, was kein Fehlschlag ist, sonst `AppleSignInFailed` oder, auf anderen Plattformen, `AppleSignInUnavailable`. Apple liefert die E-Mail-Adresse und den Namen nur bei der ersten Anmeldung des Benutzers bei der App, also bewahren Sie sie dann auf. `Nonce` geht als SHA-256 in hexadezimaler Kleinschreibung in die Anfrage ein, die der `nonce`-Claim des Identitätstokens enthält: Der Server hasht die Nonce, die er der App gegeben hat, und vergleicht. `cx.AppleCredentialState` sagt, ob der Benutzer die Anmeldedaten inzwischen widerrufen hat. In Headless-Tests erhält `app.AppleSignIn` eine `nsgotest.AppleSignInRequest` und liefert die Anmeldedaten oder den Fehler.

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

Die App muss mit dem Entitlement „Sign in with Apple“ signiert sein, das `build` hinzufügt, wenn `nsgo.yaml` danach fragt; ohne es schlägt das Sheet fehl:

```yaml
apple:
  sign_in_with_apple: true  # com.apple.developer.applesignin, in iOS and tvOS packages and Mac App Store builds
```

Xcode registriert die Fähigkeit für Geräte-Builds aus `nsgo dev`; ein Provisioning-Profil in `sign.profile` oder `sign.macos_profile` muss von einer App-ID stammen, bei der „Sign in with Apple“ aktiviert ist. macOS gewährt es nur mit einem Profil, sodass sich ein Disk-Image oder ein `nsgo dev`-Lauf auf einem Mac nicht anmelden kann, ebenso wenig wie der Simulator, dessen Builds kein Entwickler-Entitlement beanspruchen.

## In-App-Käufe

`cx.Products`, `cx.Purchase` und die Aufrufe daneben verkaufen über den App Store unter macOS, iOS und tvOS, wo `cx.Env().CanPurchase()` true ist, in einem Build, dessen `nsgo.yaml` nach StoreKit fragt:

```yaml
apple:
  purchases: true  # StoreKit's library, in macOS, iOS and tvOS packages; no entitlement is needed
```

Zeigen Sie die Preise, die der Store liefert, `DisplayPrice`, niemals eigene Preise der App: Der Store legt die Preise für jede Storefront selbst fest. Ein Kauf antwortet einmal, mit der Transaktion oder einem Fehler: `core.PurchaseCancelled`, wenn der Benutzer das Sheet schließt, `core.PurchasePending`, während er auf eine Genehmigung wartet, wie bei „Kaufanfrage“, und andernfalls `core.PurchaseFailed` mit den Worten des Stores oder anderswo `PurchasesUnavailable`. Senden Sie `Signed` der Transaktion, das JWS des Stores, zur Prüfung an den Server der App, liefern Sie aus, was gekauft wurde, und schließen Sie sie dann ab; der Store übergibt eine nicht abgeschlossene Transaktion bei jedem Start erneut an die App.

`cx.ManageSubscriptions` öffnet Apples Oberfläche zur Abo-Verwaltung (das System-Sheet unter iOS, die Abo-Seite des App Store unter macOS und tvOS). `cx.RequestRefund` startet eine Rückerstattungsanfrage für eine Transaktion und antwortet mit `core.RefundSuccess` oder `core.RefundCancelled`.

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

`FinishTransaction` gibt `PurchaseNotFound` für eine bereits abgeschlossene Transaktion zurück, einschließlich einer, die von `CurrentEntitlements` zurückgegeben wurde; ein abgeschlossener nicht verbrauchbarer Artikel bleibt im Besitz.

Registrieren Sie `OnPurchaseUpdate` beim Start der App: Aktualisierungen, die vor einem Handler eintreffen, warten auf den ersten. `cx.CurrentEntitlements` beantwortet, was der Benutzer jetzt besitzt, um es beim Start freizuschalten, und ein Button „Käufe wiederherstellen“ ruft `cx.SyncPurchases` auf, das den Benutzer möglicherweise zur Anmeldung auffordert. `AccountToken` ist eine eigene UUID der App, die die Transaktion und die Server-Benachrichtigungen des App Store mitführen. macOS verkauft nur in einem Mac-App-Store-Build, und ein App Clip verkauft nichts; Android, watchOS, Windows, Linux, das Web und Miniprogramme antworten vorerst mit `PurchasesUnavailable`. Testen Sie Käufe mit `nsgotest`, dessen `Products`, `PurchaseAnswer`, `UpdatePurchase` und `UnfinishedTransactions` den Store vertreten. `app.PurchaseRequests` zeichnet Store-Operationen als `nsgotest.PurchaseRequest`-Werte auf. In TSX benötigt das Modul `Purchases` die Fähigkeit `purchases`: `purchase` wird mit `{ status: "purchased", transaction }`, `"pending"` oder `"cancelled"` aufgelöst, und `on("update", listener)` hört Aktualisierungen.

## Ausrichtung der Oberfläche

Rufen Sie `cx.RequestOrientation(core.OrientationLandscape)` im Scope eines Vollbild-Players auf, um iOS, Android, HarmonyOS oder den Browser um Querformat zu bitten. Die neueste Anfrage, deren Scope lebt, wird wirksam. Wenn dieser Scope endet, wird die vorherige Anfrage fortgesetzt oder die App kehrt zu ihrer üblichen Ausrichtungsrichtlinie zurück.

Unter iOS 16 und später kann die Anfrage die Oberfläche sofort drehen. Unter älteren iOS-Versionen schränkt sie die nächste Systemdrehung ein. Die `IOS.SupportedOrientations` eines präsentierten modalen Dialogs haben Vorrang. Unter Android und HarmonyOS wird eine Mischung aus Hoch- und Querformatrichtungen, die das System nicht darstellen kann, auf eine erlaubte Richtung reduziert, wobei die bereits angezeigte bevorzugt wird. Android kann Ausrichtungsanfragen in Großbild- oder Mehrfenstermodi ignorieren.

In einer Web-App rufen Sie `desktop.ToggleFullScreen(cx)` aus einem Press-Handler auf, der den Scope des Players öffnet. Der Browser wechselt durch das Drücken in den Vollbildmodus, dann wendet NSGo die Ausrichtungsanfrage des Players an. Das Verlassen des Vollbildmodus hebt die Sperre auf. Die Browserkonsole meldet nicht unterstützte Operationen und Ablehnungen.

`nsgotest.App.Orientation` zeichnet die zuletzt angeforderte Maske auf, mit null für die übliche Ausrichtungsrichtlinie.

## Store-Bewertungen

`cx.RequestReview()` fordert eine Bewertungsaufforderung in der App an, vom App Store unter macOS und iOS oder von Google Play unter Android. `cx.Env().CanRequestReview()` meldet, ob der Dienst verfügbar ist. Der Store entscheidet, ob und wann die Aufforderung angezeigt wird, und meldet kein Ergebnis. Verwenden Sie einen Entwicklungs-Build, um die Apple-Aufforderung zu testen. TestFlight-Builds unterdrücken sie.

Eigenständige Android-Pakete enthalten das Play Review SDK, seine Dialog-Activity und die Sichtbarkeitsabfrage für den Play Store; zum Bauen werden weder Android SDK noch JDK benötigt. Ein Gerät ohne aktivierten Play Store meldet den Dienst als nicht verfügbar. Testen Sie ein veröffentlichtes Paket mit [Internal App Sharing](https://developer.android.com/guide/playcore/in-app-review/test).

Wenn Sie NSGo einbetten, fügen Sie `com.google.android.play:review:2.0.2` dem Gradle-Host hinzu. Fügen Sie im Manifest des Hosts `<package android:name="com.android.vending" />` unter `<queries>` hinzu, damit die App den Play Store sehen kann. Wenn der Host seinen Code verkleinert, bewahren Sie die Play-Review- und Tasks-APIs mit den folgenden Regeln, da NSGo sie über ihren Namen lädt.

```proguard
-keep class com.google.android.play.core.review.ReviewManagerFactory { *; }
-keep interface com.google.android.play.core.review.ReviewManager { *; }
-keep class com.google.android.play.core.review.ReviewInfo { *; }
-keep class com.google.android.gms.tasks.Task { *; }
-keep interface com.google.android.gms.tasks.OnCompleteListener { *; }
```

In Tests setzen Sie `CanRequestReview` in `nsgotest.Environment` und prüfen `nsgotest.App.ReviewRequests`.

## Systemgesten am Rand

Ein Spiel mit Touch-Steuerelementen am Bildschirmrand kann `mobile.DeferSystemGestures` aufrufen, während die Steuerelemente aktiv sind. Verwenden Sie die zurückgegebene Funktion, um die Anfrage vorzeitig freizugeben. Sie endet auch, wenn ihr Scope aufgeräumt wird. Anfragen aus mehreren Scopes im selben Fenster werden kombiniert.

```go
release := mobile.DeferSystemGestures(cx, mobile.SystemGestureOptions{
	IOSEdges: mobile.GestureEdgeBottom,
	AndroidRects: []core.Bounds{{
		Origin: core.Point{Y: 120},
		Size:   core.Size{Width: 24, Height: 96},
	}},
})
```

iOS verwendet physische Bildschirmränder. Android 10 und später verwenden Rechtecke in Oberflächenpunkten, gemessen von oben links. Halten Sie die Rechtecke um die Steuerelemente herum, die sie benötigen. Android reserviert seine Home-Geste und begrenzt die Fläche, die eine App von der Zurück-Geste ausnehmen kann. `nsgotest.App.SystemGestures` gibt Tests die aktuelle kombinierte Anfrage auf ihrer Hauptoberfläche. Verwenden Sie `Surface.SystemGestures()` für ein anderes Fenster.

## Overlays unter Android

`mobile.OpenOverlay` zeigt unter Android eine View der App über anderen Apps, einen eigenen Baum, der die Signals der App teilt, und ein Vordergrunddienst hält die App am Laufen, solange es angezeigt wird. Die App führt `SYSTEM_ALERT_WINDOW` unter `android.permissions` auf, und der Benutzer gewährt sie in den Systemeinstellungen, die `mobile.RequestOverlayPermission` öffnet; ohne sie endet das Overlay sofort, und `OnEnd` erfährt den Grund. Es nimmt Berührungen an, überlässt aber die Tasten und die Berührungen um es herum der App im Vordergrund, es sei denn, es ist `Focusable` oder `Passive`, und es bewegt sich mit `desktop.SetWindowFrame` aus seinem Inhalt. Andere Plattformen öffnen keines.

```go
mobile.RequestOverlayPermission(cx, func(p core.OverlayPermission) {
	if p == mobile.OverlayGranted {
		mobile.OpenOverlay(cx, mobile.OverlayOptions{Width: 200, Height: 80}, Score)
	}
})
```

## Standort und Bewegung

`cx.Locate` ermittelt eine Position, `cx.WatchLocation` streamt Positionen, und `cx.WatchMotion` streamt Samples von Beschleunigungssensor, Gyroskop, Magnetometer und fusionierter Gerätebewegung. Jedes gibt einen `*core.SensorStream` zurück, dessen `Stop` ihn abbricht, ebenso wie das Ende des Scopes, der ihn gestartet hat; Callbacks laufen auf der Goroutine der Engine. Die Berechtigungen sind `core.PermissionLocation`, `PermissionBackgroundLocation` und `PermissionMotion` ([Berechtigungen](permissions.md)): Das Starten eines Streams fragt nie, und einer ohne seine Berechtigung schlägt mit `core.SensorNotAllowed` fehl.

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

`Locate` wartet auf eine neue Position, standardmäßig 30 Sekunden (`Timeout`); nur `MaxAge` lässt es eine zwischengespeicherte nehmen, die das mit `Cached` und `Age` angibt. `Approximate` kennzeichnet eine Position, die der Benutzer nur ungefähr erlaubt hat; Android aktualisiert eine solche Position höchstens alle zehn Minuten, also geben Sie `Locate` dort ein `MaxAge` dafür. Koordinaten sind WGS 84; eine Messung, die dem Anbieter fehlt, hat `Valid` false, und `Altitude` ist ellipsoidisch, während `MSLAltitude` über dem mittleren Meeresspiegel liegt. Bewegung verwendet die natürlichen Achsen des Geräts, Beschleunigung in m/s² (+9,81 auf z, wenn es mit der Vorderseite nach oben liegt), Rotation in rad/s und das Magnetfeld in µT, standardmäßig alle 20 ms, begrenzt auf 10 ms–1 s. Unter Last behält ein Stream das neueste Sample pro Sensor und zählt die ersetzten in `Dropped`.

Streams laufen weiter, während die App inaktiv ist, und pausieren im Hintergrund, wobei das erste Sample danach mit `Gap` gekennzeichnet wird; ein ausstehendes `Locate` behält den Rest seines Timeouts. `LocationOptions.Background` hält einen Standort-Stream im Hintergrund unter `PermissionBackgroundLocation` am Laufen: Apple-Apps fügen den Hintergrundmodus `location` hinzu, und unter Android zeigt ein Vordergrunddienst eine Benachrichtigung mit `NotificationTitle` und `NotificationBody` und hält die App am Laufen, nachdem ihre Activity endet. Nichts startet einen Prozess, den das System beendet hat.

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

iOS und watchOS liefern Standort und Bewegung, macOS den Standort, tvOS jeweils eine Position und Android beides über die eigenen Anbieter des Frameworks. Linux erhält Positionen und Streams von GeoClue 2 auf dem Systembus, dessen Agent den Benutzer fragt, wenn die erste Anfrage der App startet; da GeoClue nicht ohne Abfrage gefragt werden kann, gilt die Berechtigung als nicht angefragt, bis die App in diesem Lauf gefragt hat, und Standort im Hintergrund ist nicht verfügbar. Windows erhält sie vom `Geolocator` von WinRT; die Berechtigung gilt als verweigert, solange der Standort für die App ausgeschaltet ist, als gewährt, sobald die App in diesem Lauf gefragt hat und es ihr erlaubt wurde, und andernfalls als nicht angefragt, und Standort im Hintergrund ist dort ebenfalls nicht verfügbar. Ein sicherer Browser liefert Geolocation-Positionen und auf Mobilgeräten Messwerte von Beschleunigungssensor, Gyroskop und fusionierter Bewegung. Rufen Sie `cx.RequestPermission(core.PermissionMotion, done)` unter Safari aus einem Press-Handler auf. Der Browser meldet den Standort ohne `navigator.geolocation` als nicht verfügbar, Bewegung auf Desktops und für Magnetometer-Streams als nicht verfügbar. In TSX hat `Sensors` `permission`, `requestPermission`, `locate`, `watchLocation` und `watchMotion`, unter den Paketfähigkeiten `location`, `backgroundLocation` und `motion`, in Millisekunden.

## Audio

Das Paket `audio` spielt kurze Sounds, die einmal in den Speicher dekodiert werden (`audio.LoadSound`), und lange gestreamte Audioinhalte (`audio.NewPlayer`), wo `cx.Env().CanPlayAudio()` true ist; anderswo schlagen Aufrufe mit `audio.Unavailable` fehl.

```go
audio.LoadSound(cx, audio.Source{URI: "asset:sounds/click.wav"}, func(click *audio.Sound, err error) {
	if err != nil {
		return
	}
	click.Play(audio.Voice{})                                  // center, full volume, normal rate
	click.Play(audio.Voice{Volume: 0.5, Rate: 1.2, Pan: -0.5}) // overlaps, panned left
})
```

`audio.Voice` setzt pro Wiedergabe `Volume` (0 bis 1, 0 als 1), `Rate` (0,5 bis 2, 0 als 1) und `Pan` (-1 ganz links bis 1 ganz rechts, 0 mittig); Werte außerhalb dieser Bereiche werden begrenzt, und NaN nimmt den Standardwert. `Sound.Stop` stoppt alle Wiedergaben des Sounds, und `Close` gibt ihn frei, wie es sein besitzender Scope tut, wenn er verworfen wird. `Player` unterstützt `Play`, `Preload`, `Pause`, `Seek`, `SetVolume`, `SetRate`, `SetLoop` und `Close`. macOS, iOS, Windows, Linux, Android und das Web spielen heute Audio ab. Die Backends für Apple, Windows und Browser unterstützen WAV, AAC und MP3. Linux verwendet GStreamer-Codecs, und sein gestreamter Player spielt mit 1×. Android pausiert einen Player, wenn seine View den Bildschirm verlässt, und begrenzt jeden dekodierten Sound auf 1 MiB. tvOS, watchOS, HarmonyOS und Miniprogramme melden `CanPlayAudio` weiterhin als false.

Browser verlangen eine Benutzergeste für hörbare Wiedergabe. Laden Sie kurze Sounds vorab und rufen Sie dann `Sound.Play` nach einem Klick, einem Tippen oder einem Tastendruck auf. Ein vor einer Geste abgespielter Sound ist stumm. Der `Play`-Callback eines Players erhält `audio.NotAllowed`, wenn der Browser die Wiedergabe blockiert. Versuchen Sie es aus einer Benutzergeste heraus erneut.

`Sound.Play` gibt ein `audio.Playback` zurück, das unter macOS, iOS, Windows, Linux und im Web seine eigene Lautstärke und Rate setzen oder unabhängig stoppen kann. `Ended()` ist reaktiv und wird true, wenn diese Wiedergabe endet, gestoppt wird, durch die Stimmengrenze verdrängt wird oder nicht starten kann. Backends ohne individuelle Steuerung geben ein beendetes Handle zurück.

Unter iOS hält eine eigene Audio-Engine einer Bibliothek, die außerhalb von `audio` und `Video` abspielt, etwa eine `AudioQueue`, die eine Audiositzung der App mit `audio.HoldSession(core.AudioSessionPlayback)`, während sie abspielt, und ruft die zurückgegebene Freigabe auf, wenn sie stoppt. Sie nimmt die Stufen, die die `AudioSession` eines Players nimmt, neben den Videos, Playern und Aufnahmen der App, sodass `AudioSessionPlayback` und `AudioSessionMixWithOthers` weiterspielen, wenn der Klingel-/Stumm-Schalter auf stumm steht. Anderswo hält es nichts.

Verwenden Sie `audio.Voice{Muted: true}`, um einen Effekt stumm zu starten, und `playback.SetVolume(0)`, um einen aktiven Effekt stummzuschalten. Für lange Audioinhalte streamt `audio.NewPlayer` eine Quelle, und `player.SetMuted(true)` schaltet die Ausgabe stumm, während die Wiedergabe weiterläuft. Das Aufheben der Stummschaltung stellt die konfigurierte Lautstärke des Players wieder her. `Voice.Volume` und `Player.SetVolume` verwenden null für die standardmäßige volle Lautstärke.

In TSX stellt `nsgo/audio` `supported()` und `loadSound(uri)` bereit, abgesichert durch die Paketfähigkeit `audio`. `loadSound` wird mit einem Sound aufgelöst, dessen `play({ volume, rate, pan, loop, muted })` eine Wiedergabe mit `setVolume`, `setRate`, `stop` und einem reaktiven `ended` zurückgibt; dort ist eine Lautstärke von 0 Stille. Ein abgelehntes Laden wird mit einem `Error` zurückgewiesen, dessen `reason` der Name des Audiofehlers ist, etwa `quota-exceeded`, und dessen `code` ohne die Fähigkeit `ERR_ACCESS_DENIED`, jenseits einer Grenze `ERR_NSH_QUOTA_EXCEEDED` und dort, wo nichts Audio abspielt, `ERR_FEATURE_UNAVAILABLE_ON_PLATFORM` ist. Ein Sound wird nur durch `close()` freigegeben oder wenn seine View verschwindet; in einer Sandbox zählt jedes Laden gegen `Limits.Sounds` (standardmäßig 256), bis es fehlschlägt oder sein Sound geschlossen wird.

## Gamepads

Das Paket `gamepad` liest verbundene Gamecontroller im W3C-Standard-Gamepad-Layout (`ButtonSouth`/`ButtonA` bis `ButtonHome`, `AxisLeftX`/`AxisLeftY`/`AxisRightX`/`AxisRightY`). `cx.Env().SupportsGamepads()` meldet, ob das Ziel Controller unterstützt (macOS, iOS, Windows, Linux, Android und Browser mit der Gamepad API).

`gamepad.Pads(cx)` ist ein verfolgter reaktiver Lesevorgang, der Scopes neu rendert, wenn Controller verbunden oder getrennt werden, ohne bei Änderungen von Tasten oder Sticks neu zu rendern. Jedes `gamepad.Pad` stellt seine `Family` bereit (`FamilyXbox`, `FamilyPlayStation`, `FamilyNintendo` oder `FamilyGeneric`, auch über `gamepad.DetectFamily` klassifiziert), USB-`VendorID` und `ProductID`, wo die Plattform sie meldet, und `CanRumble` für die Unterstützung von Vibrationsmotoren. Innerhalb einer `RequestAnimationFrame`-Schleife gibt `gamepad.State(cx, id)` die Tasten des Controllers (`[0, 1]`, mit `Pad.Pressed` true bei `>= 0.5`) und Achsen (`[-1, 1]`, negativ links/oben und positiv rechts/unten) zurück, einmal pro Display-Frame als Momentaufnahme erfasst.

`gamepad.Rumble(cx, id, lowFreq, highFreq, duration)` steuert die niederfrequenten (schweren/linken) und hochfrequenten (leichten/rechten) Vibrationsmotoren mit auf `[0, 1]` begrenzten Intensitäten, ersetzt jede laufende Vibration auf diesem Controller und stoppt automatisch nach `duration`, wenn die App in den Hintergrund wechselt oder den Fokus verliert (`cx.Env().Lifecycle() != core.LifecycleActive`), wenn `cx` verworfen wird oder wenn der Controller getrennt wird. `gamepad.StopRumble(cx, id)` stoppt die Vibration vorzeitig; Controller mit `CanRumble == false` ignorieren Vibrationsaufrufe sauber.

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

Rufen Sie `gamepad.EnableFocusNavigation(cx)` auf, um die Navigation zwischen nativen Fokus-Stopps mit einem Controller zu aktivieren (Steuerkreuz und linker Stick bewegen den Fokus, `ButtonSouth` aktiviert das fokussierte Steuerelement und `ButtonEast` löst Zurück aus). Aktive Gameplay-Views rufen `gamepad.CaptureInput(cx)` auf, um die Fokusnavigation zu pausieren, solange sie eingehängt sind. In TSX stellt `nsgo/gamepad` `supported()`, `pads()`, `state(id)`, `Button` und `Axis` bereit, abgesichert durch die Paketfähigkeit `gamepad`.

## Audio-Timing

Verwenden Sie `audio.Now(cx)` als Uhr für zeitgesteuerte Wiedergabe. Setzen Sie `audio.Voice.StartAt` auf einen zukünftigen Zeitpunkt, wenn `audio.CanSchedule(cx)` true ist. Rufen Sie `audio.OutputLatency(cx, func(latency time.Duration, known bool) { ... })` für die aktive Ausgaberoute auf. macOS, iOS, Linux und das Web unterstützen zeitgesteuerte Starts; unter Windows und Android ist `CanSchedule` false, und ein `StartAt` spielt sofort. Der Wert `known` des Callbacks sagt, ob die Route eine Latenz gemeldet hat, was Windows und Android nicht tun.

## Spieledienste

Das Paket `gameservices` stellt eine anbieterneutrale API für die Spieleranmeldung, Erfolge, Bestenlisten und native Game-Center-/Play-Games-Overlays bereit, wo `cx.Env().CanUseGameServices()` true ist. Jeder Callback läuft auf der Goroutine der Engine, sodass er Signals direkt setzen kann, ohne Frames zu blockieren.

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

`gameservices.ReportAchievement` begrenzt den Fortschritt auf `0..100` (`100` schaltet den Erfolg frei). `gameservices.LoadScores` liest gerankte `gameservices.ScoreEntry`-Zeilen für `Global` oder `Friends` über `AllTime`, `Week` oder `Today`, und `gameservices.ShowAchievements` und `gameservices.ShowLeaderboard` öffnen die nativen Overlays der Plattform. macOS, iOS und tvOS erreichen GameKit über purego; Android wird Play Games Services über einen Java-Laufzeit-Hook erreichen, der noch nicht in der Laufzeit enthalten ist, daher meldet es vorerst `CanUseGameServices() == false`; Windows, Linux, das Web, watchOS und Miniprogramme melden `CanUseGameServices() == false` und antworten mit `gameservices.ErrUnsupported`.

In Tests aktivieren Sie Spieledienste mit `nsgotest.Environment(core.Environment{CanUseGameServices: true})` und steuern die isolierte Attrappe jeder App mit `App.SetGameServicesPlayer`, `App.SetGameServicesFriends`, `App.SeedLeaderboardScores`, `App.SetGameServicesError`, `App.GameServicesAchievements`, `App.GameServicesScores`, `App.GameServicesShownAchievements` und `App.GameServicesShownLeaderboards`.

## Weiter

- [Benachrichtigungen](notifications.md): Berechtigung, Badges und Drücken.
- [Formulare](forms.md): Textfelder und die Tastatur.
- [Einbetten](embedding.md): Dienste, die eine Host-App ihrer eingebetteten App bereitstellt.
- [Bauen und Ausliefern](building.md): die Berechtigungen und Dateien, die ein Paket benötigt.

### Fitness, Regionen und begrenzte Bewegung

`cx.WatchSteps`, `cx.WatchActivity` und `cx.WatchPressure` geben Streams zurück, die an einen Scope gebunden sind. Fragen Sie vor Schritten/Aktivität nach `core.PermissionActivityRecognition`, und unter Apple auch vor dem Luftdruck. Apple liefert alle drei über Core Motion; Android liefert Schrittzähler und Luftdruck und meldet die Aktivitätsklassifizierung als nicht verfügbar. Zählungen gehören zum `Since`-Fenster des Samples; fortgesetzte Fenster tragen `Gap`. Der Luftdruck ist in Pascal angegeben, mit optionaler relativer Höhe in Metern. Fehlende Distanzen, Stockwerke und Höhen haben `Valid` false.

`cx.WatchGeofences` nimmt `core.GeofenceOptions` mit ein bis zwanzig unterschiedlichen `GeofenceRegion`-Kreisen (WGS-84-Koordinaten und Radius in Metern). Es meldet pro Region `GeofenceInside`, `GeofenceOutside` oder `GeofenceUnknown`. `Background` erfordert Zugriff/Deklarationen für Standort im Hintergrund und Apples Autorisierung „Immer“; Android benötigt außerdem einen Benachrichtigungstitel. Stoppen oder Verwerfen entfernt seine Registrierungen; kein Abonnement überlebt das Ende des Prozesses.

`cx.RequestPreciseLocation("route", done)` fragt ausdrücklich nach einer Autorisierung für den genauen Standort und antwortet mit einem booleschen Wert, niemals mit einer Position. Deklarieren Sie in `apple.info` `NSLocationTemporaryUsageDescriptionDictionary: {route: Explains why this feature needs precision.}`. Android benötigt sowohl die ungefähre als auch die genaue Berechtigung. Eine Antwort false kann den ungefähren Standort weiterhin autorisiert lassen.

`cx.WatchBackgroundMotion(core.BackgroundMotionOptions{Motion: core.MotionOptions{}, Duration: 20 * time.Second, NotificationTitle: "Recording motion"}, sample, failed)` fordert einen endlichen Live-Stream an. Es benötigt `AccessMotion` und `AccessBackgroundMotion`; TSX-Pakete deklarieren `motion` und `backgroundMotion`. iOS kann seine Ausführungsfreigabe früher ablaufen lassen. Android-Apps setzen `android.background_motion: true` und deklarieren `FOREGROUND_SERVICE`; der nicht persistente Dienst stoppt innerhalb von drei Minuten, selbst wenn die angeforderte Dauer länger ist. Das Ende der Dauer oder ein Ablauf durch das Betriebssystem meldet `SensorTimedOut`. Stoppen/Verwerfen beendet den Anbieter und die Freigabe; nach der Beendigung wird nichts aufgezeichnet. Andere Ziele melden nicht verfügbar.

TSX `Sensors` spiegelt diese sechs Methoden und verwendet Millisekunden. Headless-Tests speisen `FeedSteps`, `FeedActivity`, `FeedPressure` und `FeedGeofence` ein und setzen `PreciseLocationAnswer` für Genauigkeitsanfragen. Physisches Fitness-Verhalten, das Aufwecken durch Regionen und das Verhalten im angehaltenen Hintergrund benötigen weiterhin eine Abnahme auf Geräten.
