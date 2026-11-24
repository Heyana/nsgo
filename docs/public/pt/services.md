---
title: Serviços da plataforma
description: Alertas, a área de transferência, arquivos, fotos, compartilhamento, armazenamento, segredos, fala, leitura de texto em voz alta, rede, login, compras no aplicativo e o ciclo de vida do aplicativo.
group: Recursos de plataforma
order: 44
---

## Alertas e a área de transferência

`cx.ShowAlert` exibe o alerta ou a folha de ações da plataforma. Suas `Actions` retornam por índice, e um papel desenha os botões de cancelar e destrutivos do jeito da plataforma. No macOS, alertas e seletores de arquivos usam uma folha enquanto a janela está visível. Uma janela oculta, minimizada ou fora da tela, ou um aplicativo de bandeja sem janela, abre o diálogo separadamente. Fechar a janela solicitante cancela seus diálogos.

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

`SetClipboard` escreve texto e `SetClipboardImage` uma imagem. `ReadClipboard`, `ReadClipboardImage` e `ReadClipboardFiles` respondem de forma assíncrona, e `HasClipboardText` e `HasClipboardImage` leem o que a área de transferência contém sem um aviso de colagem. `core.FileFromURL` abre as URLs que `ReadClipboardFiles` e `View.OnFileDrop` fornecem como um `core.File`: URLs `file:` e, na web, URLs `blob:`.

O tvOS não tem área de transferência pública. Escritas na área de transferência são ignoradas, leituras não retornam conteúdo e verificações de presença retornam falso.

```go
cx.ReadClipboard(func(text string) { pasted.Set(text) })
cx.SetClipboard("Hello")
```

No macOS e no Windows, `ReadClipboardItems` fornece um instantâneo de todas as representações em bytes disponíveis, até 48 MiB. Cada item carrega valores `ClipboardRepresentation` ordenados com um `Type` nativo e seus `Data` brutos, incluindo formatos que indicam só presença, com dados vazios. `ClipboardFormats` lista os nomes de formatos nativos sem copiar seus conteúdos. Para restaurar um instantâneo, passe-o para `WriteClipboardItems` junto com um callback opcional que recebe o erro de escrita. Um slice vazio limpa a área de transferência. Compare a igualdade de valores de `ClipboardChangeCount` para detectar uma nova cópia, inclusive quando o texto copiado é idêntico.

O macOS mantém os limites dos itens e os nomes de tipos da pasteboard. O Windows usa um único item e preserva nomes Win32 baseados em bytes, como `CF_HDROP`, e formatos registrados, como `Rich Text Format`. Ele combina itens com formatos distintos na escrita e usa um byte zero para um conteúdo vazio. Handles GDI, formatos owner-display e IDs numéricos de formatos privados são omitidos. Em outras plataformas, os callbacks de leitura e escrita recebem `CodeUnavailable` e o callback de contagem recebe `-1`.

## Abrindo links e arquivos

`cx.OpenURL` abre uma URL no navegador ou aplicativo do sistema, e `core.CheckURL` diz o que ele recusa: sem esquema, mais de 16 KiB, espaços, um endereço web sem host, e `javascript:`, `data:`, `blob:`, `file:`, `about:` e `asset:`.

```go
if err := core.CheckURL(url); err == nil {
	cx.OpenURL(url, func(opened bool) {
		if !opened {
			status.Set("No app opens that link.")
		}
	})
}
```

`cx.OpenFile` abre um arquivo que o aplicativo possui no aplicativo associado ao seu tipo, e `cx.Share` exibe a folha de compartilhamento do sistema para arquivos, texto ou ambos no macOS, iOS, Android, Windows, Linux (por meio do portal da área de trabalho ou do gerenciador de arquivos) e na web (`navigator.share`). Um `core.SharedFile` é um caminho no disco — ou, para `Share` na web e em qualquer lugar onde um arquivo esteja em memória, seus bytes `Data` — com o nome e o tipo que o outro aplicativo vê. Em desktops, `desktop.RevealFile` mostra um arquivo ou pasta selecionado no Finder, no Explorer ou no gerenciador de arquivos, e `desktop.TrashFile` o move para a Lixeira; ele nunca exclui permanentemente, respondendo `core.ErrNoTrash` e deixando o item no lugar quando o volume não tem lixeira ou a lixeira não o manteria, como um arquivo maior que a Lixeira do Windows.

```go
cx.Share(core.ShareOptions{
	Files: []core.SharedFile{{Path: path, Name: "Notes.txt"}},
	Text:  "A note from NSGo",
}, func(shown bool, err error) {})
```

`mobile.OnShared(cx, func(items []mobile.SharedItem) { ... })` recebe texto e arquivos para [`android.share_types`](building.md#incoming-shares-on-android) ou [`apple.share_types`](building.md#incoming-shares-on-ios). Compartilhamentos no Android podem iniciar o aplicativo; o iOS os enfileira em um App Group até que o aplicativo esteja ativo ou o usuário o abra novamente. Compartilhamentos pendentes aguardam o primeiro handler. O iOS retém o lote da caixa de entrada até o callback retornar; a entrega acontece pelo menos uma vez, mesmo com o encerramento do processo. O texto chega em `Text`, e os arquivos chegam como cópias legíveis em cache com `Path`, `Name` e `MIME`. Mova ou copie um arquivo para um armazenamento persistente se ele precisar durar mais que o cache.

`desktop.LookUp` exibe o painel de dicionário do sistema para um texto, como faz um toque com três dedos no macOS, desenhado sobre uma caixa em uma view: o texto e a caixa de uma seleção com o `Ref` de sua view `Selectable`. `desktop.CanLookUp` diz se é possível, em uma janela no macOS; Windows, Linux e a web não têm esse painel. Em testes, `nsgotest.App.LookUps` lista os painéis exibidos quando `nsgotest.Environment` define `CanLookUp`.

```go
var area core.Ref
var selected core.TextSelection
core.View{Ref: &area, Selectable: true, OnTextSelection: func(s core.TextSelection) { selected = s }, Children: subtitles}
// From a menu item or a shortcut:
if desktop.CanLookUp(cx) {
	desktop.LookUp(cx, selected.Text, &area, selected.Bounds)
}
```

`desktop.QuickLook` solicita pré-visualizações de arquivos, começando em um índice, e atualiza os arquivos quando chamado novamente enquanto o painel está aberto. `desktop.CanQuickLook` informa a disponibilidade na própria janela de um aplicativo no macOS e no Windows, e no Linux quando a sessão expõe o serviço D-Bus em execução ou ativável do GNOME Sushi. O Windows usa o handler de pré-visualização registrado para o tipo de arquivo; os formatos suportados dependem dos handlers ou visualizadores instalados. Um resultado verdadeiro significa que a plataforma recebeu a solicitação, não que ela renderizou o arquivo.

Espaço ou Esc fecha o painel. As teclas de seta selecionam arquivos da solicitação no Windows e nas versões do Sushi que relatam seleção e propriedade no X11. No Wayland, o Linux atualmente abre uma pré-visualização sem janela-pai; o fechamento automático pelo dono e a seleção por setas não estão disponíveis. Visualizadores específicos do KDE e a web não são suportados.

```go
if desktop.CanQuickLook(cx) {
	desktop.QuickLook(cx, []string{reportPath, sourcePath}, 0)
}
```

## Ícones de menu nativos

`desktop.MenuItem.Icon` exibe um ícone ao lado de um item nos menus de aplicativo, Dock, bandeja e popup no macOS, Windows e Linux. Um `desktop.MenuIcon` recebe um ícone vetorial já analisado (`SVG`, como `lucide.Folder()`), bytes de imagem codificados (`Data`, PNG, JPEG, GIF, WebP ou SVG) ou uma `core.ImageSource` local, de asset ou de pixels (`Source`). Definir `Template` tinge uma imagem monocromática com a cor do texto do menu, seguindo a aparência clara e escura (e o estado de destaque no macOS); ícones `SVG` monocromáticos são tratados como templates por padrão.

```go
desktop.SetApplicationMenu(cx, []desktop.MenuItem{{
	Label: "File",
	Items: []desktop.MenuItem{
		{Label: "Open Folder…", Icon: desktop.MenuIcon{SVG: lucide.Folder()}, OnSelect: openFolder},
		{Label: "Export", Icon: desktop.MenuIcon{Data: exportPNG, Template: true}, OnSelect: exportFile},
	},
}})
```

Os ícones são carregados de forma síncrona a partir de fontes locais; URLs remotas e imagens inválidas deixam o item sem ícone. Rótulos de menu, atalhos e indicadores de marcação e de opção mantêm seu comportamento nativo.

Em testes, `nsgotest.MenuItem.Icon` registra o ícone de cada item em `app.Menu`, `app.DockMenu` e `app.PopupMenu`.

## Fotos e arquivos

`cx.PickImages` exibe o seletor de fotos do sistema e responde com `core.PickedFile`s, cujos bytes, nome, tipo e tamanho em pixels o aplicativo lê. `cx.TakePhoto` exibe a câmera onde a plataforma tem uma; `cx.Env().CanTakePhoto` diz se tem. No macOS, Windows e Linux, ele exibe a pré-visualização da câmera do leitor com um botão Tirar foto. `cx.ScanCode` abre uma pré-visualização da câmera e responde com o primeiro código que lê, um QR code a menos que `ScanOptions.Kinds` peça outros, ou nil quando o usuário o fecha; `cx.Env().CanScanCode` diz se a plataforma tem um leitor, em dispositivos macOS, iOS e Android com câmera, máquinas Windows com uma, desktops Linux com o portal Camera e o `pipewiresrc` do GStreamer, e navegadores com `getUserMedia` e `BarcodeDetector`. O macOS apresenta uma folha quando a janela solicitante está visível, o Windows e o Linux um modal sobre a janela, e a web uma sobreposição sobre a página. Coloque `NSCameraUsageDescription` em `apple.info` para macOS e iOS, e `CAMERA` em `android.permissions` para Android. Android, Windows e Linux leem todos os tipos exceto PDF417. Trate o texto do código como entrada não confiável.

No tvOS, `PickImages` responde sem imagens e `TakePhoto` retorna `core.ErrNoCamera`; o NSGo não fornece o seletor da Câmera de Continuidade.

Em testes, `nsgotest.App.PickImages`, `nsgotest.App.TakePhoto` e `nsgotest.App.ScanCode` recebem registros `nsgotest.ImagePick` para seleções e capturas e registros `nsgotest.CodeScan` para leituras, e respondem no lugar da interface do sistema. Os hooks funcionam em modo headless e sob os hosts suportados de `nsgo test -target`. `nsgotest.Environment(core.Environment{Camera: true, ScanCode: true})` dá uma câmera a um alvo que não tem.

```go
cx.PickImages(core.PickOptions{Limit: 1, MaxDimension: 2048}, func(files []core.PickedFile) {
	if len(files) == 1 {
		avatar.Set(files[0].Data)
	}
})
```

`cx.PickFiles` seleciona arquivos de qualquer tipo, `cx.PickDirectories` seleciona pastas como um `fs.FS`, e `cx.SaveFile` salva um arquivo ou bytes pelo diálogo de salvar do sistema. Ambos os seletores recebem um `Title`, um rótulo `Button` e uma pasta `Start`, e `PickFilesOptions.TypesName` dá nome ao menu de tipos, para que um aplicativo possa exibi-los em seu próprio idioma; são dicas, que macOS, Windows e Linux seguem, o iOS apenas para `Start`, e o Android e a web de forma alguma. `cx.PickScreenColor` seleciona uma cor em qualquer lugar da tela onde `cx.Env().CanPickScreenColor()` é verdadeiro: macOS, Windows, Linux com o portal Screenshot da área de trabalho, e navegadores com `EyeDropper`.

Defina `core.SaveOptions.Start` como uma pasta absoluta existente para abrir o diálogo de salvar nela no macOS, Windows, Linux e iOS. Com uma dica vazia ou inutilizável, ou em outros alvos, o sistema escolhe a pasta.

Defina `core.SaveOptions.Accept` para validar o destino escolhido antes de escrever. Ele roda na goroutine do engine. Retornar um erro deixa o destino intacto. No macOS, no Windows e no seletor GTK em processo no Linux, o diálogo mostra o motivo e deixa o usuário escolher outro caminho. Cancelar após uma recusa relata `saved == false`, o caminho recusado e o erro. No Android e no portal da área de trabalho do Linux, uma recusa conclui o salvamento com esse resultado. O Android deixa o usuário escolher uma pasta e fornece uma URI de árvore com o nome de arquivo solicitado como seu parâmetro de consulta `name`. Ele cria o documento após a aprovação e relata a URI do novo documento em caso de sucesso. A plataforma abandona a escrita se a verificação não responder em cinco segundos. O iOS e o HarmonyOS relatam um erro para salvamentos que exigem uma verificação. A web não tem caminho de destino e usa seu fluxo de salvamento comum.

Em testes headless, `App.PickFiles`, `App.PickDirectories` e `App.SaveFile` recebem registros `nsgotest.FilePick`, `nsgotest.DirectoryPick` e `nsgotest.FileSave`. Os mesmos registros ficam em `app.FilePicks`, `app.DirectoryPicks` e `app.Saves`.

### Assets empacotados

Coloque arquivos no diretório `resources/` do aplicativo e use `cx.ReadAsset("asset:maps/level.bin", done)` para lê-los a partir de Go. O callback recebe `([]byte, error)` na goroutine do engine após a leitura e é descartado se seu dono deixar de existir ou rodar novamente. Use `errors.Is(err, core.ErrAssetNotFound)` para identificar arquivos ausentes. A leitura carrega o arquivo inteiro na memória.

## Áudio PCM sob demanda

Use `audio.NewOutput(cx, audio.OutputConfig{SampleRate: 48000, Channels: 2, Render: render})` para reproduzir PCM Float32 intercalado que seu aplicativo decodifica ou sintetiza. `Start` começa a puxar amostras, `Pause` interrompe as requisições e o escopo dono fecha a saída quando é encerrado. macOS, iOS, Android, Windows, Linux e a web suportam esse fluxo. Outros alvos retornam `audio.ErrUnsupported`.

`Render` retorna o número de quadros que preencheu. Ele roda na thread de áudio e deve evitar bloqueio, alocação e acesso ao grafo. Um preenchimento incompleto toca silêncio no restante e incrementa `Underruns`. `Position` retorna a contagem monotônica de quadros e o tempo do host naquele quadro. Use `Flush` para descartar quadros pendentes e `Latency` para ler a latência do buffer. Mudanças de rota e interrupções pausam a saída e chamam `OnInterrupt` no engine. O aplicativo pode retomá-la com `Start`.

## Armazenamento

`cx.Storage()` é o armazenamento chave-valor do aplicativo, mantido no dispositivo e compartilhado por todos os grafos do aplicativo. As leituras são síncronas, e uma leitura dentro de uma computação a inscreve nas mudanças; `SetItem` retorna um `core.StorageFullError` além da cota (`core.StorageQuota`, 5 MiB por padrão). É um arquivo simples que qualquer programa do usuário pode ler, então guarde tokens e senhas em [segredos](#secrets).

`cx.Store()` retorna o store subjacente, para um host que cria outra raiz com `core.RootOptions.Storage`, como um que roda um plugin em uma goroutine própria: ambas as raízes passam a compartilhar as entradas do aplicativo e o que é mantido no dispositivo. É o store inteiro, não a partição que `cx.ProvideStorage` dá ao código que o aplicativo não escreveu.

Um host que fornece sua própria persistência chama `core.NewStore(backend)` com um `core.StorageBackend`: `Load() (map[string]string, error)` carrega uma vez, e `Write(core.StorageChange)` recebe as mudanças em ordem. Faça um switch em `change.Op` (`core.StorageSet`, `core.StorageRemove`, `core.StorageClear`) e use `change.Key` e `change.Value`. Passe `nil` para um armazenamento mantido apenas em memória.

```go
store := cx.Storage()
if tab, ok := store.GetItem("tab"); ok {
	selected.Set(tab)
}
store.SetItem("tab", "library")
```

`store.Keys()` lista as chaves, `RemoveItem` exclui uma e `Clear` esvazia o armazenamento. `cx.DataDir()` e `cx.CacheDir()` fornecem as pastas do aplicativo para arquivos grandes demais para o armazenamento chave-valor; `cx.LogsDir()` fornece o diretório da plataforma para arquivos de log, e um `Scope.ProvideStorage` dá a uma subárvore uma partição própria.

Um App Clip do iOS e seu aplicativo completo compartilham esses locais de dados e armazenamento quando `app_clip` e um `apple.app_group` explícito estão configurados; seus caches permanecem separados. Ativar o grupo não migra os dados existentes do sandbox. [Armazenamento do App Clip](building.md#app-clips) cobre a configuração e a assinatura.

No HarmonyOS, operações de arquivo do Go como `os.ReadFile`, `os.WriteFile` e `os.Rename` funcionam nesses diretórios e em `os.TempDir()`. Caminhos relativos começam no diretório de arquivos. Locks consultivos de arquivo e links simbólicos não estão disponíveis.

### As pastas do usuário

`desktop.UserDir(cx, desktop.DownloadsDir)` retorna o caminho de uma das pastas do usuário: `DocumentsDir`, `DownloadsDir`, `DesktopDir`, `PicturesDir`, `MusicDir` ou `VideosDir`. Ele consulta o sistema a cada vez, então uma pasta que o usuário moveu é acompanhada: as pastas no diretório pessoal no macOS (`Movies` para vídeos), as pastas conhecidas no Windows, incluindo as do OneDrive, e os diretórios de usuário XDG no Linux. A pasta pode ainda não existir. Celulares, a web e o HarmonyOS não têm pastas desse tipo que um aplicativo alcance por caminho, nem o Linux para uma pasta que `user-dirs.dirs` omite: `UserDir` retorna `core.ErrUserDirUnavailable`, e o aplicativo pede ao usuário com `cx.PickDirectories` ou `cx.SaveFile`.

```go
dir, err := desktop.UserDir(cx, desktop.DownloadsDir)
if err != nil {
	dir = cx.DataDir()
}
```

Os testes nunca recebem as pastas do usuário: cada uma é um diretório vazio dentro do diretório temporário do teste, e `nsgotest.UserDirs(map[core.StandardDir]string{…})` as fixa, com uma pasta omitida ficando indisponível.

### Relatórios de falha

Builds nativas de release mantêm rastros de falhas do Go em `cx.LogsDir()/crashes`. A Apple registra exceções Objective-C não capturadas, o Android registra exceções Java não capturadas e o Windows registra exceções SEH que chegam ao seu filtro de nível superior. Liste os relatórios pendentes na próxima inicialização com `cx.CrashReports()`. Cada relatório identifica a versão, a build e a plataforma que o produziram. Chame `Open()` para ler o texto de diagnóstico, feche o leitor e então chame `Acknowledge()` para excluir o relatório quando o aplicativo o tiver processado. Execute essas operações de arquivo em uma goroutine.

## Espaço em disco

`cx.DiskSpace(func(space core.DiskSpace, err error) { … })` consulta de forma assíncrona o volume que contém `cx.DataDir()` e chama de volta na goroutine do engine. `Total` é a capacidade relatada pelo sistema, `Available` é o que o aplicativo pode usar para uma operação solicitada pelo usuário, e `Opportunistic` é o que ele pode usar para caches e pré-carregamento. Os três estão em bytes. `Available` inclui o espaço que o sistema recupera sob demanda onde ele relata esse número. Plataformas com um único número de espaço livre o retornam nos dois campos. Na web, `Total` é a cota de armazenamento estimada pelo navegador para a origem e `Available` é a parte não utilizada. Quando o serviço não está disponível, ele retorna `core.ErrDiskSpaceUnavailable`.

Builds da Apple exigem que `apple.privacy.accessed_apis` em `nsgo.yaml` declare `NSPrivacyAccessedAPICategoryDiskSpace`. Declare `E174.1` para verificar o espaço antes de escrever ou para liberar arquivos quando o espaço estiver baixo, com uma mudança que o usuário possa observar. Declare `85F4.1` para mostrar ao usuário o espaço disponível. O aplicativo deve seguir as restrições da Apple sobre o uso e o compartilhamento das leituras. Uma build sem a categoria retorna `core.ErrNotDeclared`. [Motivos aprovados pela Apple](https://developer.apple.com/documentation/bundleresources/app-privacy-configuration/nsprivacyaccessedapitypes/nsprivacyaccessedapitype)

Defina a resposta de um teste com `nsgotest.DiskSpace(space, err)`.

## Armazenamento em nuvem

O pacote `cloud` sincroniza pequenas entradas chave-valor (`cloud.Get`, `cloud.Set`, `cloud.Delete`, `cloud.Keys`) e arquivos maiores ou instantâneos de salvamento (`cloud.ReadFile`, `cloud.WriteFile`, `cloud.DeleteFile`, `cloud.ListFiles`) entre dispositivos: iCloud Key-Value Store e contêineres de documentos ubiquity no macOS e iOS, e Google Play Games Saved Games no Android.

Toda operação entrega seu callback na goroutine do engine e o ignora se `cx` tiver sido descartado. `cx.Env().CanUseCloudStorage()` informa se o armazenamento em nuvem está disponível; quando o usuário está desconectado, os entitlements estão ausentes ou a plataforma não tem backend de nuvem nativo (Windows, Linux e a web), toda chamada retorna `cloud.ErrUnavailable` e nunca descarta escritas silenciosamente.

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

`cloud.WatchChanges` e `cloud.OnConflict` entregam mudanças externas vindas de vários dispositivos e conflitos de versão na goroutine do engine e se limpam automaticamente quando `cx` é descartado. Ler um arquivo em conflito retorna um `*cloud.ConflictError` (que corresponde a `errors.Is(err, cloud.ErrConflict)`).

Para ativar os entitlements do iCloud da Apple em `nsgo build`, adicione uma seção `cloud:` (ou `icloud:`) ao `nsgo.yaml`:

```yaml
cloud:
  kv_store: true
  containers:
    - iCloud.com.example.myapp
```

O perfil de assinatura deve conceder as capacidades do iCloud que você solicita. No macOS, apenas builds da Mac App Store (`sign.macos_profile`) incluem o iCloud, já que o macOS o concede somente com um perfil de provisionamento; uma build Developer ID o deixa de fora com um aviso. No Android, o armazenamento em nuvem precisa do SDK do Play Games Services v2 no aplicativo e de um jogador conectado.

Em lojas de desktop como a Steam, o Steam Auto-Cloud sincroniza `cx.DataDir()` diretamente sem nenhum código extra — continue escrevendo arquivos de salvamento locais ali e deixe a Steam sincronizar o diretório.

Em testes, `nsgotest` dá a cada aplicativo um store de nuvem isolado em memória: alterne a disponibilidade com `nsgotest.CloudAvailable` ou `app.SetCloudAvailable`, simule atualizações externas vindas de vários dispositivos com `app.SimulateCloudChange` e teste a resolução de conflitos com `app.SimulateCloudConflict`.

## Segredos

`cx.Secrets()` é o store seguro do aplicativo, para refresh tokens, chaves de API e senhas: `Get`, `Set` e `Delete` de uma string por conta, que a plataforma protege. Uma conta ausente é lida como `("", false, nil)`, e onde a plataforma não tem store seguro, como na web, toda chamada retorna `core.ErrSecretsUnavailable`; o NSGo nunca recorre a um armazenamento simples. O sistema pode fazer uma chamada esperar, como faz um keychain que pergunta ao usuário, então faça essas chamadas fora da goroutine do engine e devolva o resultado com `cx.Post`.

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

| Plataforma | Onde um segredo é guardado |
| --- | --- |
| macOS | Uma senha genérica do keychain de login: `kSecAttrService` é o serviço, `kSecAttrAccount` a conta |
| iOS, tvOS, watchOS | Uma senha genérica do keychain, da mesma forma |
| Windows | Uma credencial genérica do Gerenciador de Credenciais cujo destino é `service/account`, com no máximo 2560 bytes |
| Linux | Um item do Secret Service (GNOME Keyring, KWallet) com os atributos `service` e `account`; indisponível onde a sessão não executa nenhum |
| Android | Criptografado com uma chave AES do Android Keystore, nos arquivos privados do aplicativo, fora dos backups; as leituras também recorrem a valores que o `expo-secure-store` criptografou em suas `SharedPreferences` `SecureStore` |
| Mini programas do WeChat | Armazenamento criptografado pela plataforma (`wx.setStorage` / `wx.getStorage` com `encrypt: true`, biblioteca base 2.21.3+); indisponível em bibliotecas base mais antigas |
| Web, outros mini programas | Indisponível (`core.ErrSecretsUnavailable`): os navegadores não têm um store de segredos do cliente protegido pelo SO (mantenha as sessões web em cookies `HttpOnly` definidos pelo servidor), e os mini programas do QQ, Alipay e Douyin não têm armazenamento criptografado |

O serviço é o ID do aplicativo (`core.App().ID`) por padrão. Um aplicativo que vem de outro framework mantém seus usuários conectados ao nomear os itens que esse framework escreveu: `Service` e, nas plataformas Apple, `AccessGroup` e `Accessible`. No Android, defina `Service` como o `keychainService` do `expo-secure-store` (`"key_v1"` por padrão) para ler valores que ele criptografou em `SharedPreferences`; escrever ou excluir limpa a entrada migrada. No iOS, `core.SecretAfterFirstUnlock` permite que um trabalho em segundo plano leia um item enquanto o telefone está bloqueado, e uma acessibilidade `ThisDeviceOnly` o mantém fora dos backups. No macOS, um `AccessGroup` move os itens para o keychain de proteção de dados, o que exige uma build assinada por uma equipe cujo entitlement `keychain-access-groups` lista o grupo, configurado em `macos.entitlements` no `nsgo.yaml` tanto para builds da Mac App Store quanto Developer ID.

```go
s := cx.Secrets()
s.Service = "app:no-auth" // the service an earlier build used
s.Accessible = core.SecretAfterFirstUnlockThisDeviceOnly
err := s.Set("refresh-token", token)
```

`nsgotest` mantém os segredos de um aplicativo em memória, em `App.Secrets`, e `nsgotest.Secrets` inicia um teste com alguns, para que um teste nunca toque o keychain da máquina. Um pacote TSX acessa o store com a capacidade `secrets`, sob um serviço próprio.

## Rede

`cx.Env().Network()` informa se o sistema tem uma rota utilizável (`Online`) e se considera essa rota tarifada ou cara (`Expensive`). Os bindings a acompanham como qualquer outro valor do ambiente. `cx.OnNetworkChange(func(core.Network))` chama o handler na goroutine do engine a cada mudança enquanto seu escopo existir. A rota do sistema não diz nada sobre se um servidor específico responde. Um alvo que não consegue relatar a conectividade assume online por padrão, e um custo desconhecido assume falso.

Defina `core.Environment{Network: &core.Network{Online: false}}` por meio de `nsgotest.Environment` ou `App.SetEnvironment` para testar o comportamento offline.

O próprio `net/http` do Go faz requisições em todos os alvos, com as configurações de TLS e proxy do sistema. Os signals pertencem à goroutine do engine, então uma resposta vinda de outra goroutine chega até eles por meio de `cx.Post`.

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

`cx.Post` executa sua função na goroutine do grafo, onde os signals podem ser definidos. No Android, o manifesto pede a permissão de internet, e os downloads de imagens ficam em cache no disco.

`core.NewResource` faz o mesmo para um valor carregado a partir de uma fonte. Seu fetcher roda em uma goroutine própria e carrega novamente quando a fonte muda:

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

`items.Get()`, `items.Err()` e `items.Loading()` são signals que os bindings acompanham. Uma nova consulta cancela, por meio de `ctx`, a requisição ainda em andamento, então uma resposta atrasada nunca substitui uma mais recente. `items.Refetch()` tenta novamente, mantendo a última lista enquanto carrega, e `items.Mutate` define um valor imediatamente para uma atualização otimista.

`core.Suspense` exibe um fallback enquanto os recursos que seu conteúdo lê carregam seu primeiro valor, então uma tela não precisa de um sinalizador de carregamento próprio:

```go
core.Suspense{
	Content:  func(cx *core.Scope) core.Node { return itemList(cx, items) },
	Fallback: func(*core.Scope) core.Node { return ui.Spinner{Label: "Loading"} },
}
```

O conteúdo permanece montado, mas oculto, enquanto espera, então aparece imediatamente quando os valores chegam. Um refetch o mantém visível com o último valor.

### Descobrindo serviços locais

No macOS, iOS, Android, Windows e Linux, `cx.AdvertiseService` publica uma porta em escuta e atributos TXT. `cx.BrowseServices` relata serviços resolvidos com seu host, porta e atributos TXT. `Host` pode ser um nome de host DNS ou um endereço IP. Quando um serviço sai, o callback o recebe novamente com `Removed` definido. Verifique `cx.Env().CanDiscoverServices()` para o suporte da plataforma. Ambas as chamadas retornam uma função de parada e terminam quando seu escopo é descartado. Um erro da plataforma é relatado de forma assíncrona na goroutine do engine. O Linux usa o Avahi e relata um erro se o daemon não estiver disponível.

Para o Bonjour nas plataformas Apple, declare os tipos de serviço e explique o acesso à rede local no `nsgo.yaml`. O sistema gerencia o pedido de permissão e lembra a escolha do usuário.

```yaml
apple:
  info:
    NSLocalNetworkUsageDescription: Finds nearby devices to sync with.
    NSBonjourServices:
      - _example._tcp
```

O Android usa `NsdManager`. O Android 12 e anteriores e o Android 13 antes do T Extensions 7 exigem `CHANGE_WIFI_MULTICAST_STATE` para a descoberta via Wi-Fi. Builds independentes declaram essa permissão. Se você incorporar o NSGo em um host Android, adicione-a ao manifesto do host para esses dispositivos. Os valores TXT anunciados devem ser UTF-8 válido em dispositivos sem a API de TXT bruto do Android (API 37 ou T Extensions 22). A navegação preserva os valores TXT brutos. Se um host Android tiver como alvo a API 37 ou superior, ele deve declarar e solicitar `ACCESS_LOCAL_NETWORK` no Android 17 ou posterior.

Use `nsgotest.LocalServices` para fornecer resultados de navegação em um teste headless.

## Reproduzindo agora

Use `cx.SetNowPlaying(core.NowPlaying{...})` para mostrar a mídia atual do aplicativo na Central de Controle e na barra de menus do macOS. Forneça `Title`, `Artist`, `Album` e, opcionalmente, `Artwork` como um `*core.ImageSource`. Uma `Duration` zero significa uma transmissão ao vivo. Informe a `Position` atual e a `Rate` de reprodução (zero enquanto pausado, um na velocidade normal). Publique novamente quando a faixa, a posição ou a taxa mudar. O sistema avança a posição entre as chamadas.

Liste os controles de transporte suportados em `Commands`, como `core.MediaPlay`, `core.MediaPause`, `core.MediaTogglePlayPause` e `core.MediaSeek`. Trate as solicitações com `cx.OnMediaCommand`. O callback recebe o `Kind` do comando e, para um seek, a `Position` solicitada. O handler ativo mais recente recebe os comandos. Chame `cx.ClearNowPlaying()` quando o aplicativo terminar de reproduzir. Enquanto o aplicativo publica manualmente, os vídeos controlados no macOS suspendem a própria publicação. Verifique `cx.Env().CanShowNowPlaying()` para o suporte da plataforma.

## Downloads em segundo plano

Use `cx.Download` e `cx.OnDownloadUpdate` em aplicativos independentes de iOS e macOS, e no Android. Views Apple incorporadas não têm host de transferência. Aplicativos Android ativam `android.background_downloads: true` no `nsgo.yaml`. Inicie um download a partir de uma ação do usuário enquanto o aplicativo estiver visível. Use um destino absoluto com um diretório-pai existente em que o aplicativo possa escrever.

```go
cx.OnDownloadUpdate(func(p core.DownloadProgress) {
    progress.Set(p)
})
task, err := cx.Download(core.DownloadRequest{URL: url}, path, core.DownloadOptions{
    CertificatePEM: serverCertificate,
})
```

A transferência usa GET e aceita cabeçalhos de requisição opcionais. Ela usa o repositório de confiança HTTPS normal, a menos que `CertificatePEM` fixe o certificado folha do servidor. A fixação também exige um nome de host correspondente e um certificado válido. Use a URL final, já que o serviço exige uma resposta bem-sucedida dessa URL.

Na Apple, um certificado autoassinado em um nome de host público precisa de uma exceção do App Transport Security para esse host em `apple.info`. A transferência ainda verifica a fixação exata do certificado, o nome de host e a validade. Uma conexão com um NAS local também precisa de um `NSLocalNetworkUsageDescription` explicando o acesso, e hosts não qualificados ou `.local` usam `NSAllowsLocalNetworking`. [Configurações de ATS da Apple](https://developer.apple.com/documentation/security/preventing-insecure-network-connections)

```yaml
apple:
  info:
    NSAppTransportSecurity:
      NSExceptionDomains:
        files.example.com:
          NSExceptionAllowsInsecureHTTPLoads: true
```

O serviço em primeiro plano do Android continua depois que a activity sai da tela. Ele publica uma notificação de progresso, sujeita à permissão de notificações do Android. O progresso usa contagens de bytes e define `Total` como -1 até ser conhecido. O sucesso substitui o destino de forma atômica. `task.Pause()` mantém o arquivo parcial, `task.Resume()` usa um intervalo HTTP quando o servidor fornece um validador, e `task.Cancel()` remove o arquivo parcial.

O iOS usa uma URLSession de segundo plano do sistema que continua as transferências enquanto o aplicativo está suspenso. O aplicativo reconecta sua sessão na inicialização e publica os arquivos antes de confirmar a conclusão em segundo plano. As transferências no macOS rodam no processo. As plataformas Apple usam dados nativos de retomada para downloads pausados e mantêm os metadados da requisição no armazenamento privado do aplicativo até que um resultado final seja salvo. Um servidor que não consegue retomar inicia a transferência novamente.

Registre `OnDownloadUpdate` novamente quando o aplicativo iniciar para ver as tarefas ativas e os resultados salvos. Use `p.Task.Forget()` após tratar um resultado final. O Android pode parar o serviço, por exemplo quando o tempo de transferência em primeiro plano expira. O aplicativo então recebe `DownloadInterrupted`. No Windows, Linux, tvOS, watchOS e na web, `Download` retorna `ErrDownloadUnavailable`.

## Fala e o ciclo de vida

`cx.StartSpeech` reconhece fala com o reconhecedor do sistema onde `cx.Env().CanRecognizeSpeech()` diz que ele existe (macOS, iOS, Android, Windows e navegadores que expõem `SpeechRecognition` ou `webkitSpeechRecognition`), e responde com a transcrição à medida que ela muda. Nas plataformas Apple, declare `NSMicrophoneUsageDescription` e `NSSpeechRecognitionUsageDescription` em `apple.info`; no Android, adicione `RECORD_AUDIO` a `android.permissions`. Em um teste headless, `app.Speech` contém uma `nsgotest.SpeechSession` enquanto o reconhecimento está ativo. Use `app.HearSpeech` e `app.EndSpeech` para entregar resultados.

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

`cx.Env().Lifecycle()` diz se o aplicativo está ativo, inativo ou em segundo plano: em desktops, a janela em que `cx` está, de modo que o conteúdo de uma janela minimizada pode pausar enquanto outra janela é exibida, e no escopo do próprio aplicativo, todas as suas janelas juntas. `desktop.Minimized(cx)` diz se essa janela está minimizada, e `desktop.FrontmostApp(cx)` qual aplicativo está à frente no desktop, incluindo o próprio aplicativo, e se sua janela está em tela cheia, como um pet de desktop ou um rastreador de tempo o acompanha; `desktop.CanReadFrontmostApp(cx)` diz se ele é acompanhado, no macOS, Windows, X11 e compositores Wayland suportados. `cx.OnQuit` faz a limpeza ao encerrar, seja qual for o modo, com 200 ms para seu trabalho. `desktop.OnQuitRequest` pode cancelar ou adiar um item de menu Sair do desktop ou `desktop.RequestQuit(cx)`. No Windows e no Linux, ele também roda após `OnClosing` quando fechar uma janela encerraria o aplicativo. `desktop.RequestRestart(cx)` consulta os mesmos handlers antes de reiniciar; `desktop.Restart(cx)` continua imediato. Relançamentos do atualizador no Linux e no Windows também consultam, então um salvamento pode cancelá-los ou adiá-los. Quando o sistema relata logout, reinicialização ou desligamento, `q.Forced()` é verdadeiro e o aplicativo prossegue por `OnQuit`. Verifique `Forced()` antes de pedir uma decisão ao usuário. `cx.SetAppearance` força o modo claro ou escuro, `cx.SetLocale` escolhe o idioma, `cx.OnKey` associa um atalho de teclado, que fica em repouso enquanto o que seu dono exibe está oculto ou sob um diálogo, a menos que `core.KeyOptions{OverModals: true}` o mantenha, que recebe sua tecla antes do menu do aplicativo, a menos que `YieldsToMenu` a deixe para um item de menu habilitado que use o mesmo atalho onde está o foco (enquanto um campo de texto ou web view tem o foco, um item `MenuNotEditing` deixa a tecla para o binding e impede que itens posteriores com o mesmo atalho a recebam, e o menu do Linux não recebe nenhuma tecla pressionada sem Control, Alt ou uma tecla de logotipo, então o binding mantém essa tecla ali), que funciona na janela em que `cx` está, a menos que `AllSurfaces` o associe em todas as janelas e painéis do aplicativo, incluindo os abertos depois, para um atalho de todo o aplicativo, que fica em repouso em todos eles enquanto o que seu dono exibe está oculto na janela em que está associado, e que deixa para um controle focado as teclas que ele usa, Return e Espaço para um botão e as setas para um slider, barra de abas ou grupo de opções, a menos que seja associado com `core.KeyAlways` ou `core.KeyNotEditing`; `cx.OnOpenFiles` recebe documentos abertos no macOS, Windows, Linux e iOS. O iOS importa arquivos comuns para um armazenamento temporário do aplicativo; mova-os para `cx.DataDir()` para mantê-los. No macOS, fechar a janela que forneceu o menu do aplicativo restaura o menu da janela restante mais recente, ou o padrão quando não resta nenhuma.

No Wayland, o acompanhamento do aplicativo em primeiro plano exige a versão 2 ou posterior do protocolo wlr foreign-toplevel do compositor; outros compositores relatam `CanReadFrontmostApp` como falso. Seu ID é o ID de aplicativo Wayland, seu nome vem da entrada desktop quando disponível, e seu PID é 0. Aplicativos Windows empacotados usam seu AUMID em vez de um caminho de frame-host. O macOS segue o estado de apresentação em tela cheia do AppKit em vez de inferi-lo a partir do tamanho da janela.

Um papel de edição no menu do aplicativo recebe o editor focado antes do fallback de seu `OnSelect`. No macOS, Desfazer e Refazer seguem o histórico desse editor, e Recortar segue sua seleção, inclusive enquanto o aplicativo está inativo; sem editor, o fallback permanece habilitado.

No iOS, dê a `cx.OnKeyWith` uma função `core.KeyOptions.Title` para oferecer o atalho no menu de teclado do sistema enquanto o binding pode disparar. `Group` dá nome ao seu menu no iPadOS 26 e posteriores. As funções podem ler o catálogo do aplicativo para que os títulos dos atalhos sigam o idioma. Use `app.KeyCommands()` em testes para inspecionar a lista atual.

```go
cx.OnKey("mod+k", func() { palette.Set(true) })
cx.SetLocale("de")
cx.OnQuit(func(ctx context.Context) { saveDraft(ctx) })
```

`cx.Env().DeviceName()` é o nome que o usuário deu ao dispositivo, e `cx.Env().DeviceModel()` seu modelo de hardware, como `Mac15,7`, `iPhone15,2` ou `Google Pixel 8`, para que um aplicativo possa distinguir dois dispositivos pareados. macOS, Android, Windows e Linux fornecem os dois; o iOS fornece o modelo, e o nome apenas para um aplicativo com o entitlement da Apple para o nome de dispositivo atribuído pelo usuário. O watchOS fornece seu identificador de hardware, o HarmonyOS seu nome comercial ou modelo de produto, e os mini programas a marca e o modelo da plataforma; esses alvos deixam vazio o nome atribuído pelo usuário. A web não fornece nenhum dos dois. Os testes os definem com `nsgotest.Environment`.

`cx.Env().ThermalState()` relata `core.ThermalNominal`, `ThermalFair`, `ThermalSerious` ou `ThermalCritical` nas plataformas Apple e no Android API 29 ou posterior. `cx.Env().LowPowerMode()` relata `core.LowPowerOn` ou `LowPowerOff` nas plataformas Apple, no Android, no Windows e em sessões Linux com power-profiles-daemon. Onde o sistema não fornece valor, as leituras retornam `ThermalUnknown` e `LowPowerUnknown`. Cada leitura acompanha as mudanças de forma independente, para que um aplicativo possa ajustar sua carga de trabalho enquanto roda. Em testes, defina `ThermalState` e `LowPowerMode` em `core.Environment` por meio de `nsgotest.Environment` ou `app.SetEnvironment`.

`desktop.CursorPoint(cx, func(p core.Point, available bool) { ... })` lê o ponteiro mesmo fora das janelas do aplicativo no macOS, Windows e X11. Ele toma uma amostra por chamada. O ponto está nas mesmas coordenadas de tela que `desktop.Monitors(cx)` e `Window.ShowAt`. No Wayland e em outros alvos não suportados, ou se o escopo não tiver `core.AccessCursorPoint`, `available` é falso.

`desktop.SetWindowConstraints` limita o tamanho do conteúdo e controla o redimensionamento, a minimização e a maximização; defina `WindowOptions.Constraints` ao abrir uma janela ou use `nsgo.WindowConstraints` para a janela principal. O Linux aplica máximos não fixos no X11 e remove os botões de minimizar/maximizar proibidos da barra de título nativa do GTK no X11 e no Wayland. O Wayland atualmente suporta tamanhos fixos apenas para um limite superior.

`desktop.SetWindowLevel` faz uma janela flutuar acima das outras, e `desktop.SetWindowWorkspaces(cx, desktop.WindowWorkspacesAll)` ou `WindowOptions.Workspaces` a mantém em todos os espaços de trabalho, como fica um pet de desktop, um HUD ou um timer: no macOS em todos os Spaces e sobre os espaços em tela cheia de outros aplicativos, fora do Cmd-`, e no Linux em todas as áreas de trabalho virtuais de um gerenciador de janelas X11. O Windows e o Wayland a deixam naquele em que ela abriu, e `desktop.CanSetWindowWorkspaces` diz se isso tem efeito: verdadeiro no macOS e sob um gerenciador de janelas X11 que suporte `_NET_WM_DESKTOP`. Uma janela principal vai para todos os espaços de trabalho chamando `desktop.SetWindowWorkspaces` a partir de seu conteúdo. Em testes, `nsgotest.App.Window` lista as solicitações, e `nsgotest.Environment` define `CanSetWindowWorkspaces`.

`desktop.RegisterHotKey` associa um atalho global, que funciona enquanto qualquer aplicativo está em primeiro plano, no macOS, no Linux e em um aplicativo Windows executado com `nsgo.Run(App)`. O `State` de seu handle diz se o sistema o aceitou: `HotKeyPending` até responder, depois `HotKeyRegistered`, `HotKeyTaken` quando outro registro detém a combinação, ou `HotKeyUnsupported` para uma tecla ou plataforma sem atalhos globais, para que um gravador de atalhos possa pedir outra combinação. No macOS, uma combinação só conta como ocupada quando o aplicativo já a detém ou outro aplicativo a registrou com exclusividade. No Wayland, o compositor pede ao usuário que associe o atalho: ele fica registrado quando o usuário o faz, e ocupado quando o usuário ou o compositor o deixam de fora. Em testes, `nsgotest.HotKeyTaken` e `nsgotest.HotKeyUnsupported` fazem as combinações responderem assim.

Com `desktop.TitleBarHidden`, o Windows e o Linux oferecem o menu do aplicativo como um popup ao pressionar e soltar Alt sozinho ou F10. Alt com a primeira letra ou dígito de um rótulo de nível superior abre aquele submenu ou escolhe aquele comando, no sistema de escrita do rótulo. Os bindings do aplicativo mantêm suas teclas; Shift+F10 continua sendo a tecla do menu de contexto, e outros acordes com Alt ficam com o sistema. Esc fecha o popup e restaura o foco do teclado.

```go
shortcut := desktop.RegisterHotKey(cx, "alt+space", launcher.Toggle)
warning := core.Text{ContentFunc: func() string {
	if shortcut.State() == desktop.HotKeyTaken {
		return "Another app uses Alt+Space"
	}
	return ""
}}
```

Redimensionamentos no desktop no macOS, Windows e Linux esperam até 16 ms pelo novo layout antes de desenhar. Mantenha rápidas as computações reativas e os handlers de eventos para que o layout cumpra esse prazo; faça E/S lenta em uma goroutine e publique os resultados por meio de `cx.Post`. Um aplicativo ocupado continua redimensionando e exibe seu layout quando alcança o atraso.

`desktop.SetWindowFrame` move uma janela quando `WindowFrame.Move` é verdadeiro, usando `X` e `Y` em pontos a partir do canto superior esquerdo da tela principal. Dimensões não definidas mantêm seus tamanhos atuais. Verifique `desktop.CanPlaceWindows`: macOS, Windows e Linux no X11 suportam o posicionamento; uma toplevel comum do Wayland mantém a posição do compositor. `desktop.Frame` acompanha a posição da janela nativa nesses desktops.

No macOS e no Windows, `WindowFrame{Animated: true}` leva suavemente um painel visível até seu novo frame, incluindo o tamanho de um painel ancorado. Painéis ocultos e o movimento reduzido aplicam o novo frame imediatamente. Janelas comuns do macOS usam a animação do AppKit.

Defina `desktop.WindowOptions.Parent` como uma janela aberta do aplicativo para fechar as duas quando a janela-pai fechar. Em desktops, a janela-filha fica acima da janela-pai e a acompanha quando ela é ocultada ou minimizada; uma filha ocultada explicitamente continua oculta quando a pai retorna. No macOS, ela também se centraliza sobre a pai quando `Frame` não está definido e a acompanha para a tela cheia.

`WindowOptions.NoShadow` e `desktop.SetWindowShadow(cx, false)` também removem a sombra de uma janela do Windows cuja barra de título está oculta, que desenha sua própria moldura; uma janela com a barra de título do sistema mantém sua moldura e sombra, assim como uma janela Linux decorada pelo gerenciador de janelas, onde `NoShadow` apenas remove a sombra das decorações do próprio GTK.

`desktop.SetWindowPointerEvents` deixa o ponteiro atravessar uma janela até as janelas abaixo dela, de qualquer aplicativo: `WindowPointerNone` em toda parte, como faz um heads-up display, e `WindowPointerPainted` onde nenhuma view desenha algo ou recebe o ponteiro, como ao redor de um pet de desktop que ainda recebe toques, arrastos e cliques com o botão direito sobre si mesmo; a barra de título que o sistema desenha conta como nada desenhado. `WindowOptions.PointerEvents` e `PanelOptions.PointerEvents` abrem uma janela assim. Uma região `Desktop.WindowDrag` ainda arrasta dentro de um ancestral pressionável; um controle pressionável ou de edição mais próximo mantém sua própria entrada em vez de arrastar a janela. Uma janela que não recebe o ponteiro não recebe toques, então o aplicativo o devolve a partir de um atalho, um ícone de bandeja ou outra janela. O código que um `Access` contém precisa de `core.AccessOverlay`.

```go
desktop.OpenPanel(cx, desktop.PanelOptions{Width: 600, Height: 400, PointerEvents: desktop.WindowPointerPainted}, Pet)
```

No macOS e no Windows, uma imagem em uma janela pintada pode definir `Image.HitTestAlpha` (TSX `hitTestAlpha`) com um valor acima de 0 e no máximo 1. Por exemplo, 0.5 recebe o ponteiro nos pixels da imagem que são pelo menos metade opacos. O limite também se aplica ao `OnPress` da imagem. O fundo ou a borda de uma view ainda contam como pintados. Zero usa a view de imagem inteira, assim como as imagens no Linux.

Em uma janela pintada do macOS, `Video.HitTestAlpha` (TSX `hitTestAlpha`) aplica o mesmo limite aos quadros de vídeo decodificados. Ele funciona com `Controls` desativado e uma faixa de vídeo sem transformação, como HEVC com alfa para um pet de desktop. Zero, um quadro indisponível e os outros hosts de desktop usam os limites da view de vídeo.

`desktop.NewTray` pode desenhar um ícone dinâmico com `TrayOptions.View` no macOS e no Windows. O Windows ajusta a renderização transparente da view ao tamanho de ícone da área de notificação e a atualiza quando a view muda ou uma imagem termina de carregar; use `TrayOptions.OnClick`, `Panel` e `Menu` para suas ações. O Linux atualmente exibe ícones PNG; desenhar sua view de bandeja continua sendo um objetivo.

`desktop.OpenPanel` abre um painel que um atalho desses ou um ícone de bandeja exibe: uma janela sem bordas acima das outras. Como `desktop.OpenWindow`, `desktop.NewTray`, `mobile.OpenOverlay`, `audio.NewPlayer`, `audio.LoadSound` e `core.NewHolder`, ele pertence diretamente ao `cx` que você passa, e não ao dono do handler em execução, então passar o escopo do aplicativo a partir do `OnPress` de um botão mantém a janela, o painel ou o player vivos depois que o escopo desse botão termina; as primitivas reativas (`Effect`, `Cleanup`, `AfterFunc`, `OnKey`), por outro lado, pertencem ao dono em execução, que `cx.Run` troca. Um cartão popup se desenha em um painel com `Transparent`, que não tem fundo, moldura ou sombra próprios, então os cantos arredondados e o `BoxShadow` do cartão são tudo o que aparece, com espaço para a sombra dentro do painel; `FitContent` dimensiona o painel ao seu conteúdo, até `Width` e `Height`, antes de ele aparecer pela primeira vez e conforme o conteúdo muda. `desktop.Transparent` diz se o painel não desenha nada próprio no momento, como acontece com uma janela transparente, enquanto `desktop.CanMakeWindowsTransparent` diz apenas se uma janela poderia ser: onde é falso, como no Linux sem um compositor, dê um fundo ao conteúdo.

No macOS, um painel sem `Transparent` usa cantos retos quando qualquer dimensão está abaixo de 24 pontos e restaura seus cantos arredondados em tamanhos normais.

```go
lookup := desktop.OpenPanel(cx, desktop.PanelOptions{Width: 380, Height: 520, Transparent: true, FitContent: true, HideOnBlur: true}, Card)
desktop.RegisterHotKey(cx, "alt+space", lookup.Toggle)
```

`desktop.NewTray` exibe `TrayOptions.Title` ao lado de sua imagem ou view desenhada no macOS. Para um popup personalizado, leia `tray.ClickBounds()` dentro de `OnClick` ou `Menu` e passe o retângulo disponível para `panel.ShowAt(bounds, core.PlaceBottom)`. Ele usa os mesmos pontos de tela que `Env.Displays` e inclui o título. Os limites não estão disponíveis antes do primeiro clique, após a remoção, no Linux e quando o shell do Windows não consegue fornecê-los. Cada clique substitui os limites anteriores, então um resultado indisponível nunca reutiliza um retângulo antigo. Definir `TrayOptions.Panel` mantém, em vez disso, o comportamento embutido de alternar com um clique.

`PanelOptions.Dock` mantém um painel centralizado ao longo da borda utilizável de uma tela durante `Show`, `Focus`, `Toggle` e o redimensionamento do conteúdo. Defina `Edge` como `core.PlaceTop`, `PlaceBottom`, `PlaceLeft` ou `PlaceRight`, `Display` como um ID de `Env.Displays` (vazio para a tela principal), e `Margin` como a distância dessa borda em pontos. Um painel ancorado não pode usar `ShowAt` nem `MoveTo`; `SetWindowFrame` altera apenas seu tamanho. Um ícone de bandeja o exibe em sua âncora em vez de sob o ícone.

```go
panel := desktop.OpenPanel(cx, desktop.PanelOptions{
	Width: 320, Height: 80, NoActivate: true,
	Dock: desktop.PanelDock{Edge: core.PlaceBottom, Margin: 12},
}, Candidates)
panel.OnPlacementError(cx, func(err error) { /* offer another presentation */ })
panel.Show()
```

`desktop.CanDockPanels(cx)` relata o suporte a ancoragem separadamente do posicionamento arbitrário de janelas. macOS, Windows e X11 o suportam; o Wayland precisa da biblioteca de runtime opcional `libgtk4-layer-shell.so.0` e de um compositor que implemente `zwlr_layer_shell_v1`. O NSGo carrega essa biblioteca antes do GTK; um host de incorporação que já carregou o Wayland deve pré-carregar o gtk4-layer-shell antes de iniciar o host. Uma biblioteca, um protocolo ou uma tela selecionada ausente deixa o painel oculto e chama `OnPlacementError`. Uma ancoragem no Wayland usa a camada superior sem reservar área de trabalho: `NoActivate` deixa o teclado com a janela atual, enquanto um painel sem ela recebe o foco exclusivo do teclado até ser ocultado. Janelas comuns do Wayland mantêm o posicionamento habitual do compositor.

Um gravador de atalhos permite que o usuário escolha essa tecla. O `OnKeyDown` de uma view `Focusable` recebe cada tecla pressionada enquanto ela tem o foco, antes dos bindings do aplicativo, com os modificadores pressionados, incluindo um modificador pressionado sozinho; retornar `true` impede que a tecla chegue aos bindings, então gravar Command+S não salva. `KeyEvent.Shortcut` escreve o pressionamento como `OnKey` e `RegisterHotKey` o recebem, cada modificador pelo nome ("ctrl+shift+k", "alt+space", "shift+meta+k" para Command+Shift+K), e "" para um modificador sozinho. Seu `OnKeyUp` recebe cada uma dessas teclas ao ser solta, mesmo depois que `OnKeyDown` a consumiu ou o foco mudou, com os modificadores ainda pressionados. Ambos funcionam no macOS, Windows, Linux, na web, onde o navegador mantém os atalhos que não entrega a nenhuma página, na Apple TV e no `nsgotest`; o Android entrega os eventos de teclado físico às views focadas de forma assíncrona, então o valor de retorno de seu `OnKeyDown` não pode consumir a tecla. iPhone e iPad suportam a consulta de teclas físicas. Um atalho que o aplicativo registrou globalmente recebe sua tecla antes da view, então um gravador o cancela enquanto grava.

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

Um jogo ou um canvas consulta as teclas em vez disso. `core.KeysDown(cx)` lista as teclas pressionadas na janela e `core.KeyHeld(cx, "left")` diz se uma está: leituras rastreadas, que um loop de `RequestAnimationFrame` também lê. A primeira leitura faz a plataforma relatar todas as teclas a partir de então, então uma tecla já pressionada permanece desconhecida até ser pressionada novamente; teclas digitadas em um campo ou em uma web view, ou enquanto um método de entrada compõe, nunca ficam pressionadas, e uma tecla ainda pressionada quando a janela perde o foco é solta nesse momento. Elas funcionam onde `OnKeyUp` funciona e no iPhone e iPad com um teclado físico.

Para botões do mouse, `View.OnMouseDown` e `View.OnMouseUp` entregam um `core.MouseButtonEvent` para cada botão (`core.MouseButtonLeft`, `core.MouseButtonRight`, `core.MouseButtonMiddle`, `core.MouseButtonBack`, `core.MouseButtonForward`) no macOS, Windows, Linux, na web e no `nsgotest`, sem tornar a view uma parada de foco do teclado. `core.MouseButtonsDown(cx)` e `core.MouseButtonHeld(cx, core.MouseButtonRight)` são leituras rastreadas dos botões pressionados na janela, que se limpam automaticamente quando a janela perde o foco.

Para um controle ligado a uma posição física, leia `KeyEvent.Code`, `core.KeyCodesDown(cx)` ou `core.KeyCodeHeld(cx, "KeyW")`. O código usa os nomes de `KeyboardEvent.code`: no AZERTY, a posição do W no QWERTY tem `Code: "KeyW"` e `Key: "z"`. `core.KeyCodeLabel(cx, "KeyW")` fornece o rótulo de exibição do layout ativo para uma instrução na tela. No iOS, Android, HarmonyOS e em navegadores sem um mapa de layout, ele usa o último nome de tecla relatado naquela posição e retorna uma string vazia até que uma tecla seja vista. O HarmonyOS nomeia tanto a posição quanto a tecla a partir do `KeyCode` do InputKit, já que o ArkUI não relata scan code, então ali o nome da tecla ainda não segue um layout que não seja QWERTY.

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

## Mantendo a tela ligada

`cx.KeepAwake("Playing video", core.KeepAwakeOptions{Display: true})` mantém a tela ligada até que a função de liberação retornada seja executada ou o escopo atual seja descartado. No Android, a retenção se aplica enquanto o aplicativo está visível. Na web, funciona em um contexto seguro com suporte a Screen Wake Lock, e a página solicita o bloqueio novamente quando fica visível. O Safari pode exigir um toque ou uma tecla antes de conceder a primeira solicitação. Retenções apenas do computador não têm efeito nessas duas plataformas.

## Bloqueio do ponteiro

Na câmera de um jogo ou em um arrasto que precisa continuar na borda da tela, chame `ref.LockPointer()` a partir de um toque na view que contém o ref. O ponteiro fica parado e oculto enquanto a view recebe o movimento relativo em `OnPointerDelta(dx, dy)`. Valores positivos movem para a direita e para baixo.

Chame a função retornada para liberar o bloqueio. Esc, uma janela que perde o foco, ou ocultar ou remover a view também o libera. Verifique `cx.Env().CanLockPointer()` para ver se a superfície o suporta. A web exige um gesto do usuário, como um toque. O Linux no Wayland exige um compositor que suporte restrições de ponteiro e movimento relativo.

## Limites de som

Use `audio.SetSoundLimits(cx, audio.SoundLimits{MaxSounds: 200, MaxDuration: 20 * time.Second})` antes de carregar sons para alterar os limites do engine. `MaxEncodedBytes`, `MaxDecodedBytes` e `MaxPlaybacks` também podem ser definidos. Campos zerados usam os padrões de 8 MiB codificados e 5 segundos por som, 64 fontes, 16 MiB decodificados somando todos os sons e 32 reproduções sobrepostas por saída. Os sons existentes permanecem carregados e contam para os novos limites. Escopos restritos mantêm os limites padrão dentro do orçamento do engine: os sons `nsgo/audio` de um pacote TSX assinado os atingem independentemente do que o aplicativo Go definiu, enquanto o próprio TSX do aplicativo recebe os limites do aplicativo Go. O SoundPool do Android reproduz no máximo 32 sons de uma vez e decodifica no máximo 1 MiB de cada, independentemente dos limites.

## Tela cheia

`desktop.SetFullScreen(cx, true, displayID)` preenche a tela nomeada por `cx.Env().Displays()` no macOS, Windows e Linux. Omita o ID para usar a tela atual da janela. Um ID desconhecido deixa a janela onde está. Repetir a mesma solicitação não tem efeito. `desktop.SetFullScreen(cx, false)` sai da tela cheia e restaura o frame anterior da janela. Na web, entre a partir de um handler de toque. O navegador escolhe a tela, e `cx.Env().FullScreen()` acompanha a entrada e a saída, incluindo Esc.

## Telas

`cx.Env().Display()` retorna o `core.DisplayInfo` atual da janela e atualiza os bindings quando a janela se move para outra tela. `cx.Env().Displays()` lista as telas que a plataforma relata. `RefreshRate` e `MaxRefreshRate` estão em Hz, com zero para uma taxa indisponível. O iOS relata o máximo do painel. O HarmonyOS relata a tela da janela, sua taxa atual e sua taxa mais alta suportada. A web relata a tela atual e marca sua medição por callback de quadro com `RefreshRateEstimated`. Em testes, defina `Displays` e `DisplayID` por meio de `nsgotest.Environment`.

## Lendo texto em voz alta

`cx.Speak` lê o texto em voz alta com a voz do sistema para um idioma onde `cx.Env().CanSpeak()` diz que a plataforma fala: macOS, iOS, tvOS, Android, Windows, a web e Linux com speech-dispatcher. Uma fala de cada vez: outro `Speak` interrompe a que está falando, assim como `cx.StopSpeaking` e o fim do escopo que a iniciou. `done` recebe uma vez como ela terminou: concluída, interrompida ou com falha, com um `core.SpeakError`, como `core.SpeakLanguageUnavailable` quando nenhuma voz instalada fala o idioma.

```go
cx.Speak("Guten Morgen", core.Voice{Lang: "de-DE", Rate: 0.8}, func(stopped bool, err error) {
	speaking.Set(false)
})
```

`Rate` e `Pitch` vão de 0.5 a 2, sendo 1 o valor da própria voz. `cx.Voices("ja", ...)` lista as vozes instaladas para um idioma, para que o aplicativo possa pedir ao usuário que instale uma que falta ou deixá-lo escolher uma, cujo `ID` vai em `core.Voice`. Os navegadores só falam depois que o usuário tocou em algo na página, e antes disso falham com `core.SpeakNotAllowed`. Os componentes Go de um pacote TSX só falam com a capacidade `text_to_speech`.

## Áudio em loop

Defina `audio.Voice.Loop` para repetir a reprodução de um som até `Stop` ou `Close`. Para música transmitida, defina `audio.PlayerOptions.LoopStart` e chame `SetLoop(true)`. A primeira passagem inclui a introdução. As passagens seguintes começam no deslocamento. Um deslocamento zero repete a faixa inteira.

No macOS 14 e no iOS 17 ou posteriores, uma faixa finita usa a fila nativa para preparar cada repetição antes que a atual termine. Versões anteriores da Apple fazem um seek até o início do loop quando a passagem atual termina, com uma pequena pausa. No Linux e na web, um player em loop decodifica na memória uma faixa de até 32 MiB e 5 minutos, cerca de 23 MB por minuto, e a repete sem pausa. No Linux, uma faixa maior ou mais longa é transmitida pelo próprio decodificador e também se repete sem pausa; na web, ela volta ao início no fim, com uma pequena pausa. No Windows e no Android, um loop com início volta até ele no fim, com uma pequena pausa. Na web, um player em loop com uma taxa diferente de 1 também volta no fim, já que a reprodução sem pausas do navegador alteraria o tom com a taxa. Na web, um player em loop, como um som, só toca depois que o usuário tocou em algo na página, e antes disso falha com `NotAllowed`.

## Biometria

`cx.BiometricAvailability` informa se Face ID, Touch ID ou uma impressão digital do dispositivo está cadastrada; o Android não diz qual tipo está cadastrado, então ali ele relata impressão digital quando há uma cadastrada e, caso contrário, rosto quando o dispositivo tem hardware de reconhecimento facial. `cx.AuthenticateBiometric` pede ao sistema que confirme o usuário com um motivo, que não pode ser vazio, e relata sucesso, falha, cancelamento ou indisponibilidade. Ele não escolhe um fallback por código de acesso para o aplicativo.

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

Aplicativos com Face ID declaram `NSFaceIDUsageDescription` em `apple.info`; aplicativos Android listam `USE_BIOMETRIC` em `android.permissions`.

## Login

`cx.AuthSession` faz o login do usuário com um provedor OAuth na sessão de autenticação do sistema em vez de em uma web view: `ASWebAuthenticationSession` no macOS, iOS e watchOS, uma Custom Tab ou o navegador no Android, o navegador padrão no Windows, no Linux e em um Mac sem esquema, e um popup na web. Na web, a página de redirecionamento também envia seu callback por um `BroadcastChannel` de mesma origem, então um login se conclui quando a `Cross-Origin-Opener-Policy` do provedor separa o popup da página. Esse popup aparece como fechado para a página, então fechar um popup depois que ele mostrou a página do provedor só encerra a sessão como cancelada após 5 minutos; dê à sessão um botão de cancelar. O aplicativo cuida do protocolo: ele cria o verificador PKCE e o state, monta a URL de autorização para o redirecionamento que recebe e verifica o callback antes de trocar seu código. Cancelar não é falhar: `errors.Is(err, core.AuthCancelled)` identifica o usuário fechando a sessão, ou o `cancel` retornado encerrando-a.

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

O redirecionamento é `com.example.app:/oauth2redirect` onde um esquema está definido (Windows, Linux e Android precisam dele em `url_schemes` do `nsgo.yaml` para que o SO entregue o callback ao aplicativo em execução; a sessão do sistema do macOS, do iOS e do watchOS o captura sem essa declaração), e `http://127.0.0.1:<port>/oauth2redirect` no loopback dos desktops, então registre ambos com o provedor, muitas vezes como clientes separados. Deixe `Scheme` vazio para usar apenas o loopback, ou defina-o como `https` com `Host` para o domínio associado do aplicativo, que o iOS 17.4, o macOS 14.4 e o watchOS 10.4 capturam. O watchOS exige um esquema ou um domínio HTTPS associado; ele não tem fallback de loopback, e o mínimo existente do NSGo de watchOS 26 continua valendo. Uma aba de navegador fechada não informa nada a uma sessão de loopback, então dê a ela um `Timeout` ou um botão de cancelar. `Ephemeral` pede ao macOS, iOS e watchOS que não compartilhem os cookies do navegador, e solicita uma Custom Tab privada no Android onde o navegador a suporta; não presuma isolamento de cookies no Android. O tvOS e os mini programas respondem `core.AuthUnavailable` por enquanto. Em TSX, `AuthSession.start(url, options)` precisa da capacidade `auth_session` e fornece uma sessão cujo `result` é resolvido com o callback, ou null quando cancelada.

O Android consome um callback correspondente antes do `OnOpenLinks` comum, e retornar sem um callback encerra a sessão como cancelada. Seu callback deve usar um esquema privado do aplicativo; um esquema vazio, um callback HTTPS ou um esquema não declarado retorna `core.AuthUnavailable`, assim como um runtime Android compilado antes das sessões de login.

Uma sessão de navegador consome apenas o link de callback correspondente; links não relacionados e atrasados ainda chegam a `OnOpenLinks`. Iniciar outra sessão de navegador para a mesma URI de callback cancela a anterior; URIs distintas podem aguardar em janelas diferentes.

Use exatamente o redirecionamento passado para a função da URL de autorização. Redirecionamentos de loopback na web e no desktop codificam caminhos Unicode e resolvem segmentos de ponto antes que a função os receba; um `Path` de loopback é texto decodificado, então um `%` literal também é escapado.

Os mini programas usam `cx.SignInMiniProgram(core.MiniProgramSignInOptions{}, done)` em vez disso. O callback recebe uma `core.MiniProgramCredential` com sua `Platform` (`MiniProgramWeChat`, `MiniProgramQQ`, `MiniProgramDouyin` ou `MiniProgramAlipay`) e um `Code` não vazio e de curta duração. Envie-o ao seu servidor para a troca de código do host; mantenha o segredo do aplicativo lá. Isso é `wx.login`/`qq.login`/`tt.login`, ou a autorização básica `auth_base` do Alipay, sem solicitar informações de perfil. Não abre uma URL nem executa um construtor de URL OAuth.

`cx.Env().CanSignInMiniProgram()` informa a disponibilidade da API do host. Outros alvos respondem `core.MiniProgramSignInUnavailable`; código sem `core.AccessMiniProgramSignIn` recebe `AccessDenied`. O Douyin pergunta silenciosamente por padrão: um host sem login retorna `MiniProgramSignInNotSignedIn`, e `MiniProgramSignInOptions{Prompt: true}` permite sua janela de login da conta do host. Fechar essa janela retorna `MiniProgramSignInCancelled`. Um código anônimo de dispositivo nunca conta como um usuário conectado. Os testes definem `nsgotest.Environment(core.Environment{MiniProgramSignIn: true})` e `app.MiniProgramSignIn`; `app.MiniProgramSignIns` registra as solicitações. O TSX não roda em mini programas.

Um aplicativo cujas web views devem estar conectadas como o usuário já está em outro lugar, como em seu próprio navegador, escreve os cookies desse login com `cx.SetWebCookies`, incluindo os HttpOnly, que os scripts da página não podem definir, e então carrega as páginas: done roda quando o store os contém. O store é o persistente que todas as web views do aplicativo compartilham, então cookies com `Expires` sobrevivem ao aplicativo; views anônimas não os veem, exceto no Android, que mantém um store por aplicativo. HarmonyOS, a web, tvOS e watchOS respondem `Unavailable`.

```go
cx.SetWebCookies([]core.WebCookie{
	{Name: "SID", Value: sid, Domain: ".example.com", Expires: expires, Secure: true, HTTPOnly: true},
}, func(err error) {
	if err == nil {
		page.Set(core.WebSrc("https://www.example.com/"))
	}
})
```

Para remover um cookie, escreva-o novamente com o mesmo nome, domínio e caminho e um `Expires` no passado; uma expiração zero cria um cookie de sessão. Aguarde o callback antes de carregar uma página que deva perceber a remoção.

No sentido inverso, `cx.WebCookies` lista os cookies do store, incluindo os HttpOnly e seu domínio, caminho, expiração e flags, que `document.cookie` não consegue ler, esteja ou não sendo exibida uma página do site: um `core.WebCookieFilter` escolhe os que uma requisição a uma `URL` enviaria, ou os de um `Domain` e de todos os hosts abaixo dele, e o filtro zero escolhe todos. Um aplicativo que chama o site a partir de Go com o login do usuário, ou que mostra onde o usuário está conectado, os lê dessa forma. `cx.ClearWebsiteData` desconecta o usuário de todos os sites: remove os cookies do store, o armazenamento local e de sessão, o IndexedDB, os service workers e os caches, e done roda quando eles desaparecem. O `CookieManager` do Android responde apenas os nomes e valores que uma URL enviaria, então ali apenas um filtro `URL` lista, preenchendo `Domain` com seu host e deixando os outros campos vazios, e um filtro `Domain` ou o filtro zero responde `Unavailable`; limpar ali mantém o cache HTTP. HarmonyOS, a web, tvOS e watchOS respondem `Unavailable` a ambos.

```go
cx.WebCookies(core.WebCookieFilter{Domain: "example.com"}, func(cookies []core.WebCookie, err error) {
	signedIn.Set(err == nil && slices.ContainsFunc(cookies, func(c core.WebCookie) bool { return c.Name == "SID" }))
})
```

## Iniciar sessão com a Apple

`cx.SignInWithApple` exibe a folha do sistema de Iniciar sessão com a Apple a partir do próprio botão do aplicativo no macOS, iOS e tvOS, onde `cx.Env().CanSignInWithApple()` é verdadeiro. Ele responde uma vez com a credencial ou com um erro: `core.AppleSignInCancelled` quando o usuário fecha a folha, o que não é uma falha, caso contrário `AppleSignInFailed` ou, em outras plataformas, `AppleSignInUnavailable`. A Apple fornece o e-mail e o nome apenas no primeiro login do usuário no aplicativo, então guarde-os nesse momento. `Nonce` vai na solicitação como seu SHA-256 em hexadecimal minúsculo, que a claim `nonce` do token de identidade contém: o servidor calcula o hash do nonce que deu ao aplicativo e compara. `cx.AppleCredentialState` informa se o usuário a revogou desde então. Em testes headless, `app.AppleSignIn` recebe uma `nsgotest.AppleSignInRequest` e fornece a credencial ou o erro.

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

O aplicativo deve ser assinado com o entitlement de Iniciar sessão com a Apple, que `build` adiciona quando o `nsgo.yaml` o solicita; sem ele, a folha falha:

```yaml
apple:
  sign_in_with_apple: true  # com.apple.developer.applesignin, in iOS and tvOS packages and Mac App Store builds
```

O Xcode registra a capacidade para builds de dispositivo a partir de `nsgo dev`; um perfil de provisionamento em `sign.profile` ou `sign.macos_profile` deve vir de um App ID com Iniciar sessão com a Apple ativado. O macOS só a concede com um perfil, então uma imagem de disco ou uma execução de `nsgo dev` em um Mac não consegue fazer login, nem o simulador, cujas builds não reivindicam nenhum entitlement de desenvolvedor.

## Compras no aplicativo

`cx.Products`, `cx.Purchase` e as chamadas ao lado deles vendem pela App Store no macOS, iOS e tvOS, onde `cx.Env().CanPurchase()` é verdadeiro, em uma build cujo `nsgo.yaml` solicita o StoreKit:

```yaml
apple:
  purchases: true  # StoreKit's library, in macOS, iOS and tvOS packages; no entitlement is needed
```

Mostre os preços que a loja fornece, `DisplayPrice`, nunca preços do próprio aplicativo: a loja define o preço de cada storefront por conta própria. Uma compra responde uma vez, com a transação ou um erro: `core.PurchaseCancelled` quando o usuário fecha a folha, `core.PurchasePending` enquanto aguarda aprovação, como faz o Pedir para Comprar, e caso contrário `core.PurchaseFailed`, com as palavras da loja, ou `PurchasesUnavailable` em outros lugares. Envie o `Signed` da transação, o JWS da loja, ao servidor do aplicativo para verificação, entregue o que foi comprado e então finalize-a; a loja entrega novamente uma transação não finalizada ao aplicativo a cada inicialização.

`cx.ManageSubscriptions` abre a interface de gerenciamento de assinaturas da Apple (a folha do sistema no iOS, a página de assinaturas da App Store no macOS e no tvOS). `cx.RequestRefund` inicia uma solicitação de reembolso para uma transação e responde `core.RefundSuccess` ou `core.RefundCancelled`.

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

`FinishTransaction` retorna `PurchaseNotFound` para uma transação já finalizada, incluindo uma retornada por `CurrentEntitlements`; um item não consumível finalizado continua sendo do usuário.

Registre `OnPurchaseUpdate` quando o aplicativo iniciar: atualizações que chegam antes de qualquer handler aguardam o primeiro. `cx.CurrentEntitlements` responde o que o usuário possui agora, para desbloqueá-lo na inicialização, e um botão Restaurar compras chama `cx.SyncPurchases`, que pode pedir ao usuário que faça login. `AccountToken` é um UUID do próprio aplicativo que a transação e as notificações de servidor da App Store carregam. O macOS só vende em uma build da Mac App Store, e um App Clip não vende nada; Android, watchOS, Windows, Linux, a web e os mini programas respondem `PurchasesUnavailable` por enquanto. Teste as compras com `nsgotest`, cujos `Products`, `PurchaseAnswer`, `UpdatePurchase` e `UnfinishedTransactions` substituem a loja. `app.PurchaseRequests` registra as operações da loja como valores `nsgotest.PurchaseRequest`. Em TSX, o módulo `Purchases` precisa da capacidade `purchases`: `purchase` é resolvido com `{ status: "purchased", transaction }`, `"pending"` ou `"cancelled"`, e `on("update", listener)` recebe as atualizações.

## Orientação da interface

Chame `cx.RequestOrientation(core.OrientationLandscape)` no escopo de um player em tela cheia para pedir ao iOS, Android, HarmonyOS ou ao navegador a orientação paisagem. A solicitação mais recente cujo escopo está vivo entra em vigor. Quando esse escopo termina, a solicitação anterior é retomada ou o aplicativo volta à sua política de orientação habitual.

No iOS 16 e posteriores, a solicitação pode girar a interface imediatamente. Em versões mais antigas do iOS, ela restringe a próxima rotação do sistema. O `IOS.SupportedOrientations` de um modal apresentado tem precedência. No Android e no HarmonyOS, uma combinação de direções retrato e paisagem que o sistema não consegue representar é reduzida a uma única direção permitida, preferindo a que já está sendo exibida. O Android pode desconsiderar solicitações de orientação em modos de tela grande ou de várias janelas.

Em um aplicativo web, chame `desktop.ToggleFullScreen(cx)` a partir de um handler de toque que abre o escopo do player. O navegador entra em tela cheia a partir do toque, e então o NSGo aplica a solicitação de orientação do player. Sair da tela cheia libera o bloqueio. O console do navegador relata operações não suportadas e recusas.

`nsgotest.App.Orientation` registra a máscara solicitada mais recente, com zero para a política de orientação habitual.

## Avaliações na loja

`cx.RequestReview()` solicita uma janela de avaliação dentro do aplicativo à App Store no macOS e iOS, ou ao Google Play no Android. `cx.Env().CanRequestReview()` informa se o serviço está disponível. A loja decide se e quando mostrar a janela e não relata nenhum resultado. Use uma build de desenvolvimento para testar a janela da Apple. Builds do TestFlight a suprimem.

Pacotes Android independentes incluem o SDK do Play Review, sua activity de diálogo e a consulta de visibilidade da Play Store; compilá-los não exige o Android SDK nem o JDK. Um dispositivo sem a Play Store ativada relata o serviço como indisponível. Teste um pacote lançado usando o [Compartilhamento interno de apps](https://developer.android.com/guide/playcore/in-app-review/test).

Ao incorporar o NSGo, adicione `com.google.android.play:review:2.0.2` ao host Gradle. No manifesto do host, adicione `<package android:name="com.android.vending" />` em `<queries>` para que o aplicativo possa ver a Play Store. Se o host reduzir seu código, preserve as APIs do Play Review e de Tasks com as regras abaixo, já que o NSGo as carrega pelo nome.

```proguard
-keep class com.google.android.play.core.review.ReviewManagerFactory { *; }
-keep interface com.google.android.play.core.review.ReviewManager { *; }
-keep class com.google.android.play.core.review.ReviewInfo { *; }
-keep class com.google.android.gms.tasks.Task { *; }
-keep interface com.google.android.gms.tasks.OnCompleteListener { *; }
```

Em testes, defina `CanRequestReview` em `nsgotest.Environment` e verifique `nsgotest.App.ReviewRequests`.

## Gestos de borda do sistema

Um jogo com controles de toque na borda da tela pode chamar `mobile.DeferSystemGestures` enquanto os controles estão ativos. Use a função retornada para liberar a solicitação antes. Ela também termina quando seu escopo é limpo. Solicitações de vários escopos na mesma janela se combinam.

```go
release := mobile.DeferSystemGestures(cx, mobile.SystemGestureOptions{
	IOSEdges: mobile.GestureEdgeBottom,
	AndroidRects: []core.Bounds{{
		Origin: core.Point{Y: 120},
		Size:   core.Size{Width: 24, Height: 96},
	}},
})
```

O iOS usa as bordas físicas da tela. O Android 10 e posteriores usam retângulos em pontos da superfície medidos a partir do canto superior esquerdo. Mantenha os retângulos ao redor dos controles que precisam deles. O Android reserva seu gesto de início e limita a área que um aplicativo pode excluir do gesto Voltar. `nsgotest.App.SystemGestures` fornece aos testes a solicitação combinada atual em sua superfície principal. Use `Surface.SystemGestures()` para outra janela.

## Sobreposições no Android

`mobile.OpenOverlay` exibe uma view do aplicativo sobre outros aplicativos no Android, uma árvore própria que compartilha os signals do aplicativo, e um serviço em primeiro plano mantém o aplicativo em execução enquanto ela é exibida. O aplicativo lista `SYSTEM_ALERT_WINDOW` em `android.permissions`, e o usuário a concede nas configurações do sistema, que `mobile.RequestOverlayPermission` abre; sem ela, a sobreposição termina imediatamente e `OnEnd` recebe o motivo. Ela recebe toques, mas deixa as teclas e os toques ao seu redor para o aplicativo em primeiro plano, a menos que seja `Focusable` ou `Passive`, e se move com `desktop.SetWindowFrame` a partir de seu conteúdo. Outras plataformas não abrem nenhuma.

```go
mobile.RequestOverlayPermission(cx, func(p core.OverlayPermission) {
	if p == mobile.OverlayGranted {
		mobile.OpenOverlay(cx, mobile.OverlayOptions{Width: 200, Height: 80}, Score)
	}
})
```

## Localização e movimento

`cx.Locate` obtém uma posição, `cx.WatchLocation` transmite posições e `cx.WatchMotion` transmite amostras do acelerômetro, do giroscópio, do magnetômetro e do movimento combinado do dispositivo. Cada um retorna um `*core.SensorStream` cujo `Stop` o cancela, assim como o término do escopo que o iniciou; os callbacks rodam na goroutine do engine. As permissões são `core.PermissionLocation`, `PermissionBackgroundLocation` e `PermissionMotion` ([permissões](permissions.md)): iniciar um fluxo nunca pergunta, e um fluxo sem sua permissão falha com `core.SensorNotAllowed`.

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

`Locate` espera por uma nova posição, 30 segundos por padrão (`Timeout`); apenas `MaxAge` permite que ele use uma em cache, que indica isso com `Cached` e `Age`. `Approximate` identifica uma posição que o usuário permitiu apenas de forma aproximada; o Android atualiza essa posição no máximo a cada dez minutos, então dê a `Locate` um `MaxAge` para ela ali. As coordenadas são WGS 84; uma medição que o provedor não tem fica com `Valid` falso, e `Altitude` é elipsoidal, enquanto `MSLAltitude` é acima do nível médio do mar. O movimento usa os eixos naturais do dispositivo, aceleração em m/s² (+9.81 em z com a tela para cima), rotação em rad/s e campo magnético em µT, a cada 20 ms por padrão, limitado entre 10 ms e 1 s. Sob carga, um fluxo mantém a amostra mais recente por sensor e conta as que substituiu em `Dropped`.

Os fluxos continuam enquanto o aplicativo está inativo e pausam em segundo plano, marcando a primeira amostra depois disso com `Gap`; um `Locate` pendente mantém o restante de seu tempo limite. `LocationOptions.Background` mantém um fluxo de localização em segundo plano sob `PermissionBackgroundLocation`: aplicativos Apple adicionam o modo de segundo plano `location`, e no Android um serviço em primeiro plano exibe uma notificação com `NotificationTitle` e `NotificationBody` e mantém o aplicativo em execução depois que sua activity termina. Nada inicia um processo que o sistema encerrou.

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

O iOS e o watchOS fornecem localização e movimento, o macOS localização, o tvOS uma posição por vez e o Android ambos por meio dos provedores do próprio framework. O Linux obtém posições e fluxos do GeoClue 2 no barramento do sistema, cujo agente pergunta ao usuário quando a primeira solicitação do aplicativo começa; como não é possível consultar o GeoClue sem exibir um pedido, a permissão é lida como não solicitada até que o aplicativo a tenha solicitado naquela execução, e a localização em segundo plano não está disponível. O Windows os obtém do `Geolocator` do WinRT; a permissão é lida como negada enquanto a localização está desativada para o aplicativo, como concedida depois que o aplicativo a solicitou naquela execução e foi permitido, e caso contrário como não solicitada, e a localização em segundo plano também não está disponível ali. Um navegador seguro fornece posições de geolocalização e, em dispositivos móveis, leituras do acelerômetro, do giroscópio e do movimento combinado. Chame `cx.RequestPermission(core.PermissionMotion, done)` a partir de um handler de toque no Safari. O navegador relata a localização como indisponível sem `navigator.geolocation`, e o movimento como indisponível em desktops e para fluxos do magnetômetro. Em TSX, `Sensors` tem `permission`, `requestPermission`, `locate`, `watchLocation` e `watchMotion`, sob as capacidades de pacote `location`, `backgroundLocation` e `motion`, em milissegundos.

## Áudio

O pacote `audio` reproduz sons curtos decodificados uma vez na memória (`audio.LoadSound`) e áudio longo transmitido (`audio.NewPlayer`) onde `cx.Env().CanPlayAudio()` é verdadeiro; em outros lugares, as chamadas falham com `audio.Unavailable`.

```go
audio.LoadSound(cx, audio.Source{URI: "asset:sounds/click.wav"}, func(click *audio.Sound, err error) {
	if err != nil {
		return
	}
	click.Play(audio.Voice{})                                  // center, full volume, normal rate
	click.Play(audio.Voice{Volume: 0.5, Rate: 1.2, Pan: -0.5}) // overlaps, panned left
})
```

`audio.Voice` define, por reprodução, `Volume` (0 a 1, 0 vale 1), `Rate` (0.5 a 2, 0 vale 1) e `Pan` (-1 totalmente à esquerda a 1 totalmente à direita, 0 centralizado); valores fora desses intervalos são limitados e NaN assume o padrão. `Sound.Stop` interrompe todas as reproduções do som e `Close` o libera, como faz seu escopo dono quando descartado. `Player` suporta `Play`, `Preload`, `Pause`, `Seek`, `SetVolume`, `SetRate`, `SetLoop` e `Close`. macOS, iOS, Windows, Linux, Android e a web reproduzem áudio hoje. Os backends da Apple, do Windows e dos navegadores suportam WAV, AAC e MP3. O Linux usa os codecs do GStreamer, e seu player de streaming reproduz em 1×. O Android pausa um player quando sua view sai da tela e limita cada som decodificado a 1 MiB. tvOS, watchOS, HarmonyOS e mini programas ainda relatam `CanPlayAudio` como falso.

Os navegadores exigem um gesto do usuário para reprodução audível. Carregue os sons curtos com antecedência e então chame `Sound.Play` após um clique, toque ou pressionamento de tecla. Um som reproduzido antes de um gesto fica em silêncio. O callback de `Play` de um player recebe `audio.NotAllowed` se o navegador bloquear a reprodução. Tente novamente a partir de um gesto do usuário.

`Sound.Play` retorna um `audio.Playback`, que no macOS, iOS, Windows, Linux e na web pode definir seu próprio volume e taxa ou parar de forma independente. `Ended()` é reativo e se torna verdadeiro quando essa reprodução termina, é interrompida, é deslocada pelo limite de vozes ou não consegue começar. Backends sem controles individuais retornam um handle já encerrado.

No iOS, um engine de áudio próprio de uma biblioteca que toca fora de `audio` e `Video`, como uma `AudioQueue`, mantém a única sessão de áudio do aplicativo com `audio.HoldSession(core.AudioSessionPlayback)` enquanto toca e chama a função de liberação retornada quando para. Ele aceita os níveis que o `AudioSession` de um player aceita, ao lado dos vídeos, players e gravações do aplicativo, então `AudioSessionPlayback` e `AudioSessionMixWithOthers` continuam tocando com a chave Toque/Silencioso em silencioso. Em outros lugares, não retém nada.

Use `audio.Voice{Muted: true}` para iniciar um efeito em silêncio e `playback.SetVolume(0)` para silenciar um efeito ativo. Para áudio longo, `audio.NewPlayer` transmite uma fonte e `player.SetMuted(true)` silencia a saída enquanto a reprodução continua. Desativar o mudo restaura o volume configurado do player. `Voice.Volume` e `Player.SetVolume` usam zero para o volume máximo padrão.

Em TSX, `nsgo/audio` expõe `supported()` e `loadSound(uri)`, controlados pela capacidade de pacote `audio`. `loadSound` é resolvido com um som cujo `play({ volume, rate, pan, loop, muted })` retorna uma reprodução com `setVolume`, `setRate`, `stop` e um `ended` reativo; ali, um volume de 0 é silêncio. Um carregamento recusado é rejeitado com um `Error` cujo `reason` é o nome do erro de áudio, como `quota-exceeded`, e cujo `code` é `ERR_ACCESS_DENIED` sem a capacidade, `ERR_NSH_QUOTA_EXCEEDED` além de um limite e `ERR_FEATURE_UNAVAILABLE_ON_PLATFORM` onde nada reproduz áudio. Um som só é liberado por `close()` ou quando sua view deixa de existir; em um sandbox, cada carregamento conta contra `Limits.Sounds` (256 por padrão) até falhar ou seu som ser fechado.

## Controles de jogo

O pacote `gamepad` lê controles de jogo conectados no layout W3C Standard Gamepad (de `ButtonSouth`/`ButtonA` até `ButtonHome`, `AxisLeftX`/`AxisLeftY`/`AxisRightX`/`AxisRightY`). `cx.Env().SupportsGamepads()` informa se o alvo suporta controles (macOS, iOS, Windows, Linux, Android e navegadores com a Gamepad API).

`gamepad.Pads(cx)` é uma leitura reativa rastreada que renderiza novamente os escopos quando controles se conectam ou desconectam, sem renderizar novamente em mudanças de botões ou alavancas. Cada `gamepad.Pad` expõe sua `Family` (`FamilyXbox`, `FamilyPlayStation`, `FamilyNintendo` ou `FamilyGeneric`, também classificada via `gamepad.DetectFamily`), o `VendorID` e o `ProductID` USB onde a plataforma os relata, e `CanRumble` para suporte a motores hápticos. Dentro de um loop de `RequestAnimationFrame`, `gamepad.State(cx, id)` retorna os botões do controle (`[0, 1]`, com `Pad.Pressed` verdadeiro em `>= 0.5`) e os eixos (`[-1, 1]`, negativo para esquerda/cima e positivo para direita/baixo), capturados uma vez por quadro de exibição.

`gamepad.Rumble(cx, id, lowFreq, highFreq, duration)` aciona os motores de vibração de baixa frequência (pesado/esquerdo) e de alta frequência (leve/direito) com intensidades limitadas a `[0, 1]`, substituindo qualquer vibração em andamento nesse controle e parando automaticamente após `duration`, quando o aplicativo vai para segundo plano ou perde o foco (`cx.Env().Lifecycle() != core.LifecycleActive`), quando `cx` é descartado ou quando o controle se desconecta. `gamepad.StopRumble(cx, id)` interrompe a vibração antes; controles com `CanRumble == false` ignoram as chamadas de vibração sem erros.

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

Chame `gamepad.EnableFocusNavigation(cx)` para optar por navegar entre as paradas de foco nativas com um controle (o direcional e a alavanca esquerda movem o foco, `ButtonSouth` ativa o controle focado e `ButtonEast` aciona voltar). Views de jogabilidade ativas chamam `gamepad.CaptureInput(cx)` para pausar a navegação por foco enquanto estão montadas. Em TSX, `nsgo/gamepad` expõe `supported()`, `pads()`, `state(id)`, `Button` e `Axis`, controlados pela capacidade de pacote `gamepad`.

## Sincronização de áudio

Use `audio.Now(cx)` como relógio para reprodução agendada. Defina `audio.Voice.StartAt` como um momento futuro quando `audio.CanSchedule(cx)` for verdadeiro. Chame `audio.OutputLatency(cx, func(latency time.Duration, known bool) { ... })` para a rota de saída ativa. macOS, iOS, Linux e a web suportam inícios agendados; no Windows e no Android, `CanSchedule` é falso e um `StartAt` toca imediatamente. O valor `known` do callback diz se a rota relatou uma latência, o que o Windows e o Android não fazem.

## Serviços de jogos

O pacote `gameservices` fornece uma API independente de provedor para login do jogador, conquistas, placares e as sobreposições nativas do Game Center / Play Games onde `cx.Env().CanUseGameServices()` é verdadeiro. Todo callback roda na goroutine do engine, então pode definir signals diretamente sem bloquear quadros.

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

`gameservices.ReportAchievement` limita o progresso a `0..100` (`100` desbloqueia a conquista). `gameservices.LoadScores` lê linhas classificadas de `gameservices.ScoreEntry` para `Global` ou `Friends` em `AllTime`, `Week` ou `Today`, e `gameservices.ShowAchievements` e `gameservices.ShowLeaderboard` abrem as sobreposições nativas da plataforma. macOS, iOS e tvOS acessam o GameKit via purego; o Android acessará o Play Games Services por meio de um hook de runtime Java que ainda não está no runtime, então por enquanto relata `CanUseGameServices() == false`; Windows, Linux, a web, watchOS e mini programas relatam `CanUseGameServices() == false` e respondem `gameservices.ErrUnsupported`.

Em testes, ative os serviços de jogos com `nsgotest.Environment(core.Environment{CanUseGameServices: true})` e controle a simulação isolada de cada aplicativo com `App.SetGameServicesPlayer`, `App.SetGameServicesFriends`, `App.SeedLeaderboardScores`, `App.SetGameServicesError`, `App.GameServicesAchievements`, `App.GameServicesScores`, `App.GameServicesShownAchievements` e `App.GameServicesShownLeaderboards`.

## Próximos passos

- [Notificações](notifications.md): permissão, emblemas e toques.
- [Formulários](forms.md): campos de texto e o teclado.
- [Incorporação](embedding.md): serviços que um aplicativo host fornece ao aplicativo incorporado.
- [Compilação e distribuição](building.md): as permissões e os arquivos de que um pacote precisa.

### Condicionamento físico, regiões e movimento limitado

`cx.WatchSteps`, `cx.WatchActivity` e `cx.WatchPressure` retornam fluxos com escopo. Solicite `core.PermissionActivityRecognition` antes de passos/atividade, e antes da pressão na Apple. A Apple fornece os três por meio do Core Motion; o Android fornece contadores de passos e pressão, e relata a classificação de atividade como indisponível. As contagens pertencem à janela `Since` da amostra; janelas retomadas carregam `Gap`. A pressão está em pascals, com altitude relativa opcional em metros. Distâncias, andares e altitude ausentes têm `Valid` falso.

`cx.WatchGeofences` recebe `core.GeofenceOptions` com de um a vinte círculos `GeofenceRegion` distintos (coordenadas WGS 84 e raio em metros). Ele relata `GeofenceInside`, `GeofenceOutside` ou `GeofenceUnknown` por região. `Background` exige acesso e declarações de localização em segundo plano e a autorização Sempre da Apple; o Android também precisa de um título de notificação. Parar ou descartar remove seus registros; nenhuma inscrição sobrevive ao fim do processo.

`cx.RequestPreciseLocation("route", done)` solicita explicitamente a autorização de localização precisa e responde com um booleano, nunca com uma posição. Em `apple.info`, declare `NSLocationTemporaryUsageDescriptionDictionary: {route: Explains why this feature needs precision.}`. O Android precisa das permissões coarse e fine. Uma resposta falsa pode deixar a localização aproximada autorizada.

`cx.WatchBackgroundMotion(core.BackgroundMotionOptions{Motion: core.MotionOptions{}, Duration: 20 * time.Second, NotificationTitle: "Recording motion"}, sample, failed)` solicita um fluxo ao vivo finito. Ele precisa de `AccessMotion` e `AccessBackgroundMotion`; pacotes TSX declaram `motion` e `backgroundMotion`. O iOS pode expirar sua concessão de execução antes. Aplicativos Android definem `android.background_motion: true` e declaram `FOREGROUND_SERVICE`; o serviço não persistente para em até três minutos, mesmo que a duração solicitada seja maior. A expiração da duração ou do SO relata `SensorTimedOut`. Parar/descartar encerra o provedor e a concessão; nada é gravado após o encerramento. Outros alvos relatam indisponível.

O `Sensors` do TSX espelha esses seis métodos e usa milissegundos. Testes headless alimentam `FeedSteps`, `FeedActivity`, `FeedPressure` e `FeedGeofence`, e definem `PreciseLocationAnswer` para solicitações de precisão. O comportamento físico de condicionamento físico, de despertar por região e em segundo plano suspenso ainda precisa de validação em dispositivo.
