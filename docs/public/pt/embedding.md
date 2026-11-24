---
title: Incorporação
description: Coloque um aplicativo NSGo dentro do seu próprio aplicativo iOS, Android, desktop ou web.
group: Ferramentas
order: 23
---

## Compile um aplicativo incorporável

Um aplicativo incorporável é um pacote de aplicativo normal com um `Root` e um `main`, compilado com `-embed` para o alvo do host. Cada view executa o aplicativo uma vez e é um motor próprio, de modo que um host exibe quantas views quiser, cada uma com seu próprio estado.

```sh
go tool nsgo build -embed -target ios examples/embedded
go tool nsgo build -embed -target android_arm64 examples/embedded
go tool nsgo build -target web examples/embedded
```

A compilação iOS gera `<Name>.xcframework` e `<Name>Resources`; a compilação Android gera `<Name>.aar`; a compilação web gera um diretório com `nsgo.json` e o carregador.

## iOS

Quando o aplicativo importa `base/terminal`, a compilação também gera `GhosttyVT.xcframework` ao lado do framework do aplicativo. Adicione esse pacote complementar ao alvo do host com **Embed & Sign**, tanto para frameworks de aplicativo estáticos quanto `-dynamic`. Ele contém bibliotecas para dispositivo, simulador Intel e Apple silicon e Mac Catalyst; mantenha o nome `GhosttyVT.framework`.

Adicione o framework ao alvo do aplicativo como biblioteca estática (não o incorpore) e a pasta de recursos como referência de pasta, e então faça `import <Name>`. Chame `NSGoStart()` uma vez quando o aplicativo iniciar, antes da primeira view. O código da biblioteca estática pertence ao aplicativo host, então o manifesto de privacidade do host declara as APIs de motivo obrigatório que o NSGo usa: user defaults (`CA92.1`), carimbos de data/hora de arquivo (`C617.1`) e tempo de inicialização do sistema (`35F9.1`); um framework `-dynamic` já os carrega em seu próprio `PrivacyInfo.xcprivacy`.

```swift
import Embedded
import UIKit

@main
final class AppDelegate: UIResponder, UIApplicationDelegate {
	func application(_ app: UIApplication, didFinishLaunchingWithOptions: [UIApplication.LaunchOptionsKey: Any]?) -> Bool {
		NSGoStart()
		return true
	}
}

let view = NSGoView(frame: .zero)
view.params = ["user": "Ada"]              // cx.HostParams()
view.dataDirectory = documents.path        // where storage lives
view.localeIdentifier = "de-CH"            // in place of the system's
view.setFontScale(1.3)
view.overrideUserInterfaceStyle = .dark
view.tokens = ["accent": "#cf222e"]        // over the app's theme
view.handle("time") { _, reply in reply(Date().description, nil) }
view.send("message", ["text": "Hello"])    // cx.OnHostEvent("message", …)
view.onOpenURL = { url in router.open(url) }
view.onError = { message in log(message) }
view.register(component: "host.badge") { context in Badge(context) }
```

`NSGoView` inicia o aplicativo assim que entra em uma janela e o interrompe quando é desalocada ou recebe `stop()`. Defina os valores de inicialização antes de ela estar em uma janela; o restante pode mudar a qualquer momento. Manipuladores, componentes e callbacks rodam na thread principal, e uma resposta pode vir depois de qualquer thread. Um view controller pode encaminhar `preferredStatusBarStyle`, `prefersStatusBarHidden` e `prefersHomeIndicatorAutoHidden` da view para seguir suas configurações de barra do sistema.
O host também pode retornar `preferredScreenEdgesDeferringSystemGestures` da view a partir de seu controller para encaminhar `mobile.DeferSystemGestures`.

## Android

Para um aplicativo arm64 que importa `base/terminal`, o AAR inclui `jni/arm64-v8a/libghostty-vt.so` ao lado de `libnsgo.so`; o host mantém a extração de bibliotecas nativas habilitada. A pré-compilação do Android e o carregador de terminal exigem [#1452](https://github.com/nsgo-dev/nsgo/issues/1452); bibliotecas de terminal x86_64 ainda não estão disponíveis.

Adicione o AAR ao módulo do aplicativo e empacote as bibliotecas nativas extraídas, para que o runtime as encontre:

```kotlin
implementation(files("libs/Embedded.aar"))

android { packaging { jniLibs { useLegacyPackaging = true } } }
```

Crie `dev.oneo.nsgo.NsgoView`s em código ou em um layout. Cada uma inicia o aplicativo em um processo próprio assim que recebe um tamanho pela primeira vez, e `stop()` o encerra. Encaminhe os resultados da activity para cada view e trate alterações de configuração na activity para que os aplicativos não sejam reiniciados.

```java
NsgoView app = new NsgoView(context);
app.setParams(Map.of("user", "Ada"));
app.setStorageDirectory(dir);
app.setLocale(Locale.forLanguageTag("de-CH"));
app.setFontScale(1.3f);
app.setAppearance(NsgoView.APPEARANCE_DARK);
app.setTokens(Map.of("accent", "#cf222e"), null);
app.handle("time", (data, reply) -> reply.resolve(now()));
app.handle("tapped", (data, reply) -> status.setText("tapped " + ((Map<?, ?>) data).get("count")));
app.setOpenUrlHandler(url -> router.open(url));
app.setErrorListener(message -> log(message));
app.registerComponent("host.badge", Badge::new);
```

```java
@Override
protected void onActivityResult(int request, int result, Intent data) {
	super.onActivityResult(request, result, data);
	for (NsgoView app : apps) app.activityResult(request, result, data);
}
```

Para desanexar a árvore nativa mantendo o aplicativo Go vivo, retenha um `NsgoEngine` no host e anexe sua view explicitamente:

```java
NsgoEngine engine = new NsgoEngine(context.getApplicationContext());
app.attachToEngine(engine);
// Before taking the view off screen:
app.detachFromEngine();
// Reuse the same view later:
app.attachToEngine(engine);
// When the host ends the app:
engine.destroy();
```

Desanexar libera os filhos nativos; reanexar restaura os signals atuais, entrada, foco, posição de rolagem e desenhos sem reexecutar componentes. Páginas web e reprodutores são recriados a partir de sua configuração do aplicativo. `stop()` apenas desanexa um motor fornecido explicitamente. Cada motor ainda retém sua primeira view e sua Activity e rejeita outra view, portanto trate alterações de configuração nessa Activity; substituí-la ou exibir views independentes em um único processo ainda não é suportado.

Para verificações de atualização do Google Play, adicione `com.google.android.play:app-update:2.1.0` às dependências Gradle do host Android. Preserve as APIs AppUpdateManager, AppUpdateInfo, AppUpdateOptions e Tasks que o NSGo carrega pelo nome usando as [regras de encolhimento de runtime](https://github.com/nsgo-dev/nsgo/blob/main/internal/platform/android/runtime/play-review.pro). O encaminhamento do resultado da activity acima também entrega o cancelamento da atualização.

Para sessões de autenticação no Android em um host incorporado, declare o esquema privado de callback na Activity hospedeira e encaminhe os links que essa Activity aceita para `view.openLink(uri.toString())` em `onNewIntent` (e em um `onCreate` novo). Uma sessão ativa correspondente consome seu callback; outros links chegam a `OnOpenLinks`. A `NsgoActivity` autônoma os encaminha por conta própria.

## Desktop

Um host Go executa o loop de eventos da plataforma e coloca o `Handle` de cada view em sua janela. `nsgo.NewView` inicia o aplicativo assim que o handle está em uma janela, de modo que as configurações feitas antes já estão em seu primeiro quadro.

```go
channel := core.NewHost()
channel.SetParams(map[string]any{"user": "Ada"})
channel.Handle("time", func(c *core.HostCall) { c.Resolve(time.Now().Format("15:04:05")) })
channel.HandleOpenURL(func(url string, answer func(bool)) {
	answer(strings.HasPrefix(url, "https://example.com/orders/"))
})

view := nsgo.NewView(app.Root, nsgo.HostChannel(channel), nsgo.DataDir(dataDir))
defer view.Close()

view.SetTokens(core.Tokens{Values: map[string]string{"accent": "#cf222e"}})
view.SetLocale("de-CH")
view.SetFontScale(1.3)
view.SetAppearance(core.AppearanceDark)
handle := view.Handle() // an NSView, GtkWidget or child window
```

`view.Host()` envia eventos ao aplicativo e recebe suas chamadas, e `nsgo.Component` registra um componente nativo para a plataforma do host.

## Web

A compilação web é uma página própria, mas uma página pode montá-la em um elemento e ser seu host. `mount(element, url, options)` retorna um player, e `nsgo.json` nomeia o carregador, o wasm e os recursos relativos a si mesmo.

```html
<div id="app"></div>
<script type="module">
const url = new URL("./Embedded-0.1.0-web/nsgo.json", location.href);
const config = await (await fetch(url)).json();
const { mount } = await import(new URL(config.loader, url));

const player = await mount(document.getElementById("app"), url.href, {
	config,
	params: { user: "Ada" },
	host: {
		call(name) {
			if (name === "time") return new Date().toLocaleTimeString();
			throw new Error(`no host function ${name}`);
		},
		post(name, data) { console.log(name, data); },
		openURL(url) { return url.startsWith("https://example.com/orders/"); },
	},
});
player.hostEvent("message", { text: "Hello from the page" });
</script>
```

Sem uma opção `host`, o carregador importa o módulo que `web.host` do `nsgo.yaml` nomeia e aguarda seu `createHost(player)` caso o módulo o exporte. Um aplicativo em um elemento ou em um quadro deixa o histórico e o título do navegador sob responsabilidade da página.

`appearance` (`"light"`, `"dark"` ou `"system"`), `locale` e `fontScale` substituem os do navegador, como fazem as configurações de um host nativo, e `setAppearance`, `setLocale` e `setFontScale` do player os alteram depois. `module`, um `WebAssembly.Module` compilado ou uma promise dele, permite que uma página que monta o aplicativo muitas vezes o baixe e compile uma única vez.

O carregador exibe um elemento `progress.nsgo-load-progress` antes de o aplicativo iniciar. Estilize-o com CSS, passe `onProgress` nas opções de montagem ou exporte `onProgress` de `web.host` para a própria página do aplicativo. O callback substitui a barra e recebe `{ loaded, total }` em bytes decodificados, com `total` definido como `null` quando o tamanho é desconhecido. O carregador o chama com `null` quando a inicialização termina para que ele possa limpar sua interface. Um `module` compilado pula o progresso do download.

```yaml
web:
  host: host.js
```

## O canal do host

`core.Host` é o contrato único que todo host implementa: parâmetros de inicialização, eventos que o host envia e as funções do host que o aplicativo chama. Valores atravessam como em JSON, portanto mapas viram `map[string]any`, números viram `float64` e o mesmo código funciona em Swift, Java, Go e JavaScript.

- `HostParams` lê os parâmetros de inicialização uma única vez, no início.
- `cx.OnHostEvent` escuta `Host.Send` e `hostEvent` da página.
- `cx.CallHost` chama um manipulador nomeado e resolve com sua resposta; `cx.PostHost` chama um sem esperar resposta.
- `Host.HandleOpenURL` vê `cx.OpenURL` antes da plataforma, e sua resposta decide se a plataforma também a abre.
- `Host.SetParams`, `Send`, `Handle` e `HandleDefault` são seguros para uso em qualquer goroutine.

No iOS, o host define `params`, instala manipuladores com `handle`, envia eventos com `send` e responde a `onOpenURL`. No Android, faz o mesmo com `setParams`, `handle`, `send` e `setOpenUrlHandler`. No desktop usa os métodos de `core.Host`, e na web o objeto `host` da página responde.

## Componentes nativos

`core.Native` exibe uma view criada pelo host: suas `Props` chegam ao `SetProp` do componente do host, o que o componente emite chega a `OnEvent` e uma prop cujo valor é um nó animado se move no relógio de quadros sem trabalho para o motor.

```go
core.Native{
	Component: "host.badge",
	PropsFunc: func() map[string]any { return map[string]any{"count": count.Get()} },
	OnEvent:   func(name string, data any) { if name == "tap" { count.Set(0) } },
	Style:     core.Style{MinWidth: core.Pt(120), MinHeight: core.Pt(28)},
}
```

Registre o componente no host: `app.register(component:)` e um `NSGoComponent` no iOS, `registerComponent` e um `NsgoComponent` no Android, `nsgo.Component` no desktop. A web não tem componentes nativos, portanto um nome desconhecido exibe uma caixa vazia lá.

## Configurações

`tokens` substitui o tema do aplicativo por nome de papel (`accent`, `inset`, `raised`), em ambas as aparências ou em uma delas; `cx.Palette()` os enxerga. O host também define o locale, o tamanho do texto, a aparência e o diretório de dados, e escuta as falhas do aplicativo. `nsgo build -embed` não cria janelas, menus nem ícones de bandeja: esses pertencem ao host.

Um panic interno na goroutine do motor de uma view interrompe aquele motor, destrói suas superfícies anexadas e relata o panic e a pilha ao ouvinte de erros do host (`nsgo.OnError` em Go). Outros motores continuam rodando; crie uma nova view para reiniciar o aplicativo que falhou. Panics comuns em callbacks do aplicativo continuam indo primeiro para o `core.Catch` mais próximo.

`examples/embed-ios`, `examples/embed-android`, `examples/embed-desktop` e `examples/embed-web` são o mesmo aplicativo com cada tipo de host.

## Próximos passos

- [Serviços de plataforma](services.md): o que o aplicativo solicita à plataforma.
- [Compilação e distribuição](building.md): os pacotes que `-embed` cria e como assiná-los.
- [Exemplo: Um aplicativo de tarefas](tutorials/todo.md): o aplicativo que um host incorpora.
