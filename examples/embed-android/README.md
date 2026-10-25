# Embedding an NSGo app in an Android app

`src/dev/oneo/nsgo/embedhost/MainActivity.java` is an Android app of its own that shows the
NSGo app in `examples/embedded` twice, below its own views.

1. Build the app as a library: `go tool nsgo build -embed -target android_arm64 examples/embedded`
   (`android_x64` for the emulator, which needs an NDK). `Embedded.aar` holds NSGo's Java
   runtime (`dev.oneo.nsgo`), the app as `libnsgo.so` and its resources as assets.
2. Add it to the app module: `implementation(files("libs/Embedded.aar"))`. The runtime starts the
   app from the native library directory, so the module packages native libraries extracted:

   ```kotlin
   android { packaging { jniLibs { useLegacyPackaging = true } } }
   ```

3. Make `dev.oneo.nsgo.NsgoView`s, in code or in a layout. Each starts a process of the app as it
   first gets a size, and stops it with `stop()`, which the activity calls as it is destroyed.
   The activity should handle configuration changes itself (`android:configChanges`), or each
   change restarts the apps.

Set what the app starts with before it has a size, the rest at any time:

```java
NsgoView app = new NsgoView(context);
app.setParams(Map.of("user", "Ada"));              // cx.HostParams()
app.setStorageDirectory(dir);                      // where its storage lives
app.setLocale(Locale.forLanguageTag("de-CH"));     // in place of the system's
app.setFontScale(1.3f);
app.setAppearance(NsgoView.APPEARANCE_DARK);
app.setTokens(Map.of("accent", "#cf222e"), null);  // cx.Env().Token("accent")
app.handle("time", (data, reply) -> reply.resolve(now()));     // cx.CallHost("time", …)
app.send("message", Map.of("text", "Hello"));      // cx.OnHostEvent("message", …)
app.setOpenUrlHandler(url -> router.open(url));    // cx.OpenURL, before the system
app.setErrorListener(message -> log(message));
app.registerComponent("host.badge", Badge::new);   // core.Native, an NsgoComponent
```

Handlers, components and listeners run on the UI thread; a reply may come later from any
thread. Values cross as JSON: maps, lists, strings, numbers, booleans and null.
