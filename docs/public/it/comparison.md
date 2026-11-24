---
title: Confronto con altri framework
description: Come si colloca NSGo rispetto a NSH, MyGo e ad altri framework UI, e a cosa rinuncia.
order: 2
---

La collocazione di NSGo tra i framework UI deriva da tre scelte: le app si scrivono in Go (o in TSX al di sopra), ogni piattaforma disegna con le proprie viste e un'unica codebase copre telefoni, desktop e web. Altri framework fanno scelte diverse, e ogni scelta ha un costo.

## A colpo d'occhio

| Framework | Linguaggio | Disegna con | Target |
| --- | --- | --- | --- |
| NSGo | Go, TSX | Le viste di ciascuna piattaforma | iOS, App Clips, tvOS, watchOS, Android, macOS, Windows, Linux, web; mini program, in via sperimentale |
| [NSH](https://nsh.dev) | Rust, TSX | Le viste di ciascuna piattaforma | iOS, Android, macOS, Windows, Linux, web, app GPUI, terminali; HarmonyOS e mini program WeChat tramite piattaforme della community |
| [MyGo](https://github.com/egoist/mygo) | Go, pagine web | Il proprio renderer GPU o la webview di sistema | macOS, Windows, Linux, web |
| [Fyne](https://fyne.io), [Gio](https://gioui.org) | Go | Il proprio renderer | Desktop, telefoni, web |
| [Wails](https://wails.io) | Go, pagine web | La webview di sistema | Desktop |
| [Flutter](https://flutter.dev) | Dart | Il proprio renderer | Telefoni, desktop, web |
| [Compose Multiplatform](https://www.jetbrains.com/compose-multiplatform/) | Kotlin | Il proprio renderer; Compose su Android | Telefoni, desktop, web |
| [React Native](https://reactnative.dev) | JavaScript, TSX | Le viste di ciascuna piattaforma | iOS, Android; altri tramite piattaforme della community |
| [.NET MAUI](https://dotnet.microsoft.com/apps/maui) | C# | I controlli di ciascuna piattaforma | iOS, Android, macOS, Windows |
| [Avalonia](https://avaloniaui.net) | C# | Il proprio renderer | Desktop, telefoni, web |
| [Qt](https://www.qt.io) | C++, QML | Il proprio renderer | Desktop, telefoni, dispositivi embedded |
| [Electron](https://www.electronjs.org), [Tauri](https://tauri.app) | Pagine web; Rust per Tauri | Un Chromium integrato; la webview di sistema per Tauri | Desktop; telefoni per Tauri |

## Punti di forza

- **Viste native.** Testo, metodi di input, selezione, scorrimento, accessibilità e menu sono quelli della piattaforma stessa, così un'app si comporta come gli utenti si aspettano su quel sistema, laddove un framework che disegna in autonomia deve ricostruire ciascuno di questi aspetti. Ciò che è presente solo su una piattaforma, come il Liquid Glass di iOS, viene imitato allo stesso modo sulle altre anziché essere omesso.
- **Aggiornamenti granulari.** I componenti vengono eseguiti una sola volta e i signal aggiornano solo ciò che li legge, senza virtual DOM né funzioni di vista da confrontare a ogni frame; le animazioni girano sul clock dei frame della piattaforma.
- **I pacchetti più piccoli e l'avvio più rapido.** Nei [benchmark](https://nsgo.dev/benchmarks/), la stessa app contatore occupa circa 8 MB su macOS, Windows e Linux, meno di MyGo, GPUI e NSH (da 33 a 45 MB per NSH); inoltre si avvia più velocemente e consuma meno memoria rispetto ai framework che disegnano su GPU, come MyGo e GPUI.
- **Solo Go.** Il motore, il layout e i backend sono scritti in Go, senza C, senza cgo e senza motore JavaScript per le app Go.
- **Ogni piattaforma da un solo Mac.** Un Mac con Go e Xcode compila in cross-compilation ogni target: niente NDK, Android SDK, JDK, GTK o toolchain di Windows.
- **Ogni file di rilascio con un solo comando.** `nsgo build` genera un'immagine disco firmata e notarizzata per macOS, un installer firmato per Windows, pacchetti deb e Arch e un AppImage, pacchetti per Mac App Store, Microsoft Store, Google Play e Flathub, e script di installazione dai rilasci GitHub ([compilazione e distribuzione](building.md)).
- **Aggiornamenti nativi di ciascuna piattaforma.** Su macOS Sparkle e su Windows WinSparkle scaricano, verificano e installano la versione successiva; su Linux e Android l'app la individua e apre la relativa pagina di rilascio; su iOS provvede l'App Store. Un'app installata tramite uno store rimane gestita da quello store ([aggiornamenti](building.md#updates)).
- **Ogni piattaforma.** Telefoni, tablet, TV, orologi, desktop e web condividono un unico motore e un'unica API, insieme ad App Clips su iOS, widget per iPhone e Mac e, in via sperimentale, mini program per WeChat, QQ, Alipay e Douyin.
- **Integrazione.** Un'app NSGo può essere inserita come vista in un'app iOS, Android, desktop Go o web esistente, per migrare una schermata alla volta ([integrazione](embedding.md)).
- **Un ciclo di sviluppo rapido.** Salvare un file Go ricompila e riavvia l'app; una modifica a un file TSX aggiorna l'app in esecuzione sul posto mantenendone lo stato; gli errori di compilazione e i panic durante il rendering vengono mostrati sopra l'app ([esecuzione e debug](debugging.md)).
- **Un'unica suite di test ovunque.** I test individuano e utilizzano i controlli tramite testo o ruolo proprio come fanno gli utenti, rapidamente senza finestra oppure invariati in finestre reali su ciascun target con l'input della piattaforma stessa, salvando screenshot ([test](testing.md)).
- **Localizzazione integrata.** I testi dell'app e dei suoi componenti risiedono in cataloghi dedicati, e le pseudo-locale individuano testi non tradotti, troncati o non pronti per la scrittura da destra a sinistra ([localizzazione](localization.md)).
- **Shader in Go.** `core.Shader` esegue shader GPU scritti con sintassi Go o in WGSL, su ogni piattaforma ([canvas](canvas.md#shaders)).
- **TSX quando serve.** Le schermate possono essere scritte in TSX con classi Tailwind, sullo stesso motore dei componenti Go.

## Punti deboli

- **Progetto giovane.** La 0.1.0 è la prima release e l'API non è ancora stabile; la [roadmap](https://github.com/nsgo-dev/nsgo/blob/main/docs/roadmap.md) elenca ciò che manca, come un limite di heap per il codice TSX caricato a runtime.
- **Ecosistema ridotto.** Pochi componenti di terze parti, esempi o risposte online rispetto a Flutter, React Native o Qt.
- **Differenze tra piattaforme.** L'uso di viste native comporta piccole differenze tra piattaforme, quindi un'app va verificata su ciascuna di esse. Un framework che disegna autonomamente ha lo stesso aspetto ovunque.
- **Bridge dedicati.** Su Android il programma Go gira accanto al runtime Java e comunica tramite pipe, e su Linux gli utenti devono avere GTK 4.14 o successivo.
- **TSX è interpretato.** TSX viene eseguito in goja, un interprete JavaScript scritto in Go, più lento rispetto ai motori usati da React Native o Electron.

## NSH

NSGo riprende l'architettura di NSH: la struttura, i backend, i nomi delle API e i componenti privi di stile di `base` derivano da NSH ([stato](index.md#status)).

- **Scegli NSGo** per scrivere app in Go anziché in Rust o TSX, per compilare ogni target da un Mac con Go e Xcode, oppure per ciò che NSGo aggiunge: shader GPU scritti in Go, icone vettoriali, un terminale, app che aprono le proprie finestre, pannelli e icone nella tray di sistema, e componenti con stile in `ui`.
- **Scegli NSH** per un progetto più maturo, TSX eseguito da QuickJS con bytecode e limiti di heap, il backend GPUI o Android a 32 bit.

## MyGo

Entrambi offrono UI scritte in Go senza cgo, ma MyGo disegna autonomamente la propria UI nativa sulla GPU e la ricostruisce da una funzione di vista a ogni frame, mentre NSGo passa alle viste native solo ciò che è cambiato.

- **Scegli MyGo** per app desktop con lo stesso aspetto su ogni sistema operativo desktop, finestre basate su pagine web con chiamate tipizzate verso Go e le sue API desktop e di distribuzione: icone nella tray, associazioni di file, installer, notarizzazione e aggiornamenti delta firmati.
- **Scegli NSGo** per raggiungere anche i telefoni, per sfruttare testo, input e accessibilità nativi di ciascuna piattaforma, oppure per uno stato mantenuto in signal anziché ridisegnato a partire dai dati dell'app.

## Altri toolkit Go

- **Fyne e Gio** disegnano tutto in autonomia, come fa MyGo, ed entrambi raggiungono telefoni e web. Fyne fornisce un set completo di widget con un proprio stile visivo; Gio è in immediate mode, a basso livello e flessibile. Entrambi hanno versioni rilasciate e utenti attivi; nessuno dei due ha l'aspetto o il comportamento nativo.
- **Wails** inserisce un frontend web nella webview di sistema con Go alle spalle: funziona qualsiasi libreria web, ma l'interfaccia è una pagina web ed è solo per desktop.

## Framework in altri linguaggi

- **Flutter e Compose Multiplatform** disegnano ogni pixel autonomamente: identici su ogni piattaforma e maturi, con ecosistemi ampi, ma con testo, scorrimento e input ricostruiti anziché nativi, e Dart o Kotlin da imparare.
- **React Native** condivide con NSGo il modello di un core reattivo che guida viste native e dispone dell'ecosistema più vasto della categoria. È centrato su iOS e Android ed esegue la logica applicativa in JavaScript; NSGo la esegue in Go e tratta desktop e web come target di prima classe.
- **.NET MAUI** incapsula i controlli di ciascuna piattaforma in C#, senza Linux né web. **Avalonia** disegna autonomamente, vicino a WPF, su ogni desktop, telefoni e web.
- **Qt** è maturo, veloce e raggiunge i dispositivi embedded, ma disegna i propri widget, richiede C++ o QML ed è distribuito sotto licenza LGPL o commerciale.
- **Electron e Tauri** mostrano pagine web: tutti gli strumenti e le librerie del web al completo, al costo della memoria di un browser, soprattutto nel caso di Electron, e di un'interfaccia che si percepisce come una pagina web.

## Quale scegliere

Scegli NSGo per un'app che deve risultare nativa su telefoni, desktop e web partendo da un'unica codebase Go, quando un progetto ancora giovane è accettabile. Per un'app che deve essere distribuita a breve su un'API stabile, scegli un framework maturo dalla tabella sopra: Flutter o React Native per i telefoni, MyGo, Wails o Tauri per il solo desktop.
