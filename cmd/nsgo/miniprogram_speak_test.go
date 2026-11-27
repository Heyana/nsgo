package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	projectcfg "github.com/nsgo-dev/nsgo/project"
)

func TestMiniProgramHostSpeak(t *testing.T) {
	vm := runMiniProgramModule(t, "speak.js")
	_, err := vm.RunString(`
const speakMod = module.exports;
function check(cond, msg) { if (!cond) throw new Error(msg); }

function makeAudioAPI() {
  const contexts = [];
  const api = {
    createInnerAudioContext() {
      const ctx = {
        src: '',
        playbackRate: 1,
        played: 0,
        stopped: 0,
        destroyed: 0,
        _ended: null,
        _stop: null,
        _error: null,
        onEnded(fn) { this._ended = fn; },
        onStop(fn) { this._stop = fn; },
        onError(fn) { this._error = fn; },
        play() { this.played++; },
        stop() { this.stopped++; if (this._stop) this._stop(); },
        destroy() { this.destroyed++; },
        emitEnded() { if (this._ended) this._ended(); },
        emitError() { if (this._error) this._error({ errCode: -1 }); },
      };
      contexts.push(ctx);
      return ctx;
    },
  };
  return { api, contexts };
}

// 1. Unconfigured WeChat must not call requirePlugin and must report unavailable.
{
  let required = 0;
  const { api } = makeAudioAPI();
  const s = speakMod.createSpeaker('wechat', api, {
    pluginName: '',
    requirePlugin() { required++; return { textToSpeech() {} }; },
  });
  check(required === 0, 'unconfigured WeChat called requirePlugin');
  check(s.available === false, 'unconfigured WeChat reported available');
  check(s.voices().length === 0, 'unconfigured WeChat returned voices');
  let got;
  s.start({ id: 1, text: '你好', lang: 'zh-CN' }, (r) => { got = r; });
  check(got && got.error === 'synthesis-unavailable' && got.stopped === false, 'unconfigured start did not fail unavailable');
}

// 2. QQ, Alipay and Douyin have no built-in client TTS API and must stay unavailable.
for (const platform of ['qq', 'alipay', 'douyin']) {
  const { api } = makeAudioAPI();
  const s = speakMod.createSpeaker(platform, api, {
    pluginName: 'WechatSI',
    requirePlugin() { return { textToSpeech() {} }; },
  });
  check(s.available === false, platform + ' reported available');
  check(s.voices().length === 0, platform + ' returned voices');
  let got;
  s.start({ id: 2, text: 'hello', lang: 'en-US' }, (r) => { got = r; });
  check(got && got.error === 'synthesis-unavailable', platform + ' did not refuse unavailable');
}

// 3. WeChat with throwing/missing plugin or missing createInnerAudioContext reports unavailable.
{
  const { api } = makeAudioAPI();
  const sThrow = speakMod.createSpeaker('wechat', api, {
    pluginName: 'WechatSI',
    requirePlugin() { throw new Error('plugin not declared'); },
  });
  check(sThrow.available === false, 'throwing requirePlugin reported available');

  const sBadPlugin = speakMod.createSpeaker('wechat', api, {
    pluginName: 'WechatSI',
    requirePlugin() { return {}; },
  });
  check(sBadPlugin.available === false, 'plugin without textToSpeech reported available');

  const sNoAudio = speakMod.createSpeaker('wechat', {}, {
    pluginName: 'WechatSI',
    requirePlugin() { return { textToSpeech() {} }; },
  });
  check(sNoAudio.available === false, 'missing createInnerAudioContext reported available');
}

// 4. Configured WeChat with WechatSI plugin + InnerAudioContext full lifecycle.
{
  const { api, contexts } = makeAudioAPI();
  const requests = [];
  let loadedName = '';
  const plugin = {
    textToSpeech(opts) { requests.push(opts); },
  };
  const s = speakMod.createSpeaker('wechat', api, {
    pluginName: 'WechatSI',
    requirePlugin(name) { loadedName = name; return plugin; },
  });
  check(loadedName === 'WechatSI', 'loaded wrong plugin name: ' + loadedName);
  check(s.available === true, 'configured WeChat unavailable');

  const voices = s.voices();
  check(voices.length === 2 && voices[0].id === 'WechatSI:zh_CN' && voices[0].lang === 'zh-CN' &&
        voices[1].id === 'WechatSI:en_US' && voices[1].lang === 'en-US', 'unexpected voices list');

  // Normal synthesis + playback completion + duplicate end ignored.
  const results = [];
  s.start({ id: 10, text: '早上好', lang: 'zh-CN', rate: 1.5 }, (r) => results.push(r));
  check(requests.length === 1 && requests[0].lang === 'zh_CN' && requests[0].tts === true && requests[0].content === '早上好', 'bad tts request');
  requests[0].success({ retcode: 0, filename: 'wxfile://tmp_1.mp3' });
  check(contexts.length === 1 && contexts[0].src === 'wxfile://tmp_1.mp3' && contexts[0].playbackRate === 1.5 && contexts[0].played === 1, 'audio did not start');
  contexts[0].emitEnded();
  contexts[0].emitEnded();
  check(results.length === 1 && results[0].stopped === false && results[0].error === '', 'end callback wrong');
  check(contexts[0].destroyed === 1, 'audio context not destroyed after end');

  // Explicit voice ID + rate clamping + stop(id) stops and destroys audio.
  const stopResults = [];
  s.start({ id: 11, text: 'Hello', lang: 'zh-CN', voice: 'WechatSI:en_US', rate: 5 }, (r) => stopResults.push(r));
  check(requests[1].lang === 'en_US', 'voice ID did not select en_US');
  requests[1].success({ retcode: 0, filename: 'wxfile://tmp_2.mp3' });
  check(contexts[1].playbackRate === 2, 'rate was not clamped to 2');
  s.stop(999);
  check(stopResults.length === 0 && contexts[1].stopped === 0, 'mismatched stop ID stopped utterance');
  s.stop(11);
  check(stopResults.length === 1 && stopResults[0].stopped === true && stopResults[0].error === '', 'stop did not report stopped');
  check(contexts[1].stopped === 1 && contexts[1].destroyed === 1, 'stop did not stop/destroy audio');

  // Replacing an in-flight synthesis stops the first and ignores its late callback.
  const r1 = [], r2 = [];
  s.start({ id: 12, text: 'First', lang: 'en' }, (r) => r1.push(r));
  s.start({ id: 13, text: 'Second', lang: 'zh' }, (r) => r2.push(r));
  check(r1.length === 1 && r1[0].stopped === true, 'replaced synthesis not stopped');
  const beforeCtx = contexts.length;
  requests[2].success({ retcode: 0, filename: 'wxfile://stale.mp3' });
  check(contexts.length === beforeCtx && r1.length === 1, 'late synthesis callback leaked');
  requests[3].success({ retcode: 0, filename: 'wxfile://second.mp3' });
  contexts[contexts.length - 1].emitError();
  check(r2.length === 1 && r2[0].error === 'synthesis-failed' && r2[0].stopped === false, 'audio error not mapped');

  // Language and voice validation before calling plugin.
  const beforeReqs = requests.length;
  let badLang, badVoice;
  s.start({ id: 14, text: 'Bonjour', lang: 'fr-FR' }, (r) => { badLang = r; });
  s.start({ id: 15, text: 'Hello', lang: 'en-US', voice: 'unknown' }, (r) => { badVoice = r; });
  check(requests.length === beforeReqs, 'invalid lang/voice called plugin');
  check(badLang.error === 'language-unavailable', 'unsupported lang error: ' + badLang.error);
  check(badVoice.error === 'voice-unavailable', 'unknown voice error: ' + badVoice.error);

  // Plugin retcode error mapping and missing filename.
  let eLang, eLong, eNet, eEmpty;
  s.start({ id: 16, text: 'a', lang: 'zh-CN' }, (r) => { eLang = r; });
  requests[requests.length - 1].fail({ retcode: -20001, msg: 'lang error' });
  check(eLang.error === 'language-unavailable', 'retcode -20001 error: ' + eLang.error);

  s.start({ id: 17, text: 'a', lang: 'zh-CN' }, (r) => { eLong = r; });
  requests[requests.length - 1].fail({ retcode: -20002, msg: 'text length limit exceeded' });
  check(eLong.error === 'text-too-long', 'retcode -20002 length error: ' + eLong.error);

  s.start({ id: 18, text: 'a', lang: 'zh-CN' }, (r) => { eNet = r; });
  requests[requests.length - 1].fail({ retcode: -20005, msg: 'network error' });
  check(eNet.error === 'synthesis-failed', 'retcode -20005 error: ' + eNet.error);

  s.start({ id: 19, text: 'a', lang: 'zh-CN' }, (r) => { eEmpty = r; });
  requests[requests.length - 1].success({ retcode: 0, filename: '' });
  check(eEmpty.error === 'synthesis-failed', 'empty filename error: ' + eEmpty.error);

  // close() tears down active playback.
  let closed;
  s.start({ id: 20, text: 'bye', lang: 'en-US' }, (r) => { closed = r; });
  requests[requests.length - 1].success({ retcode: 0, filename: 'wxfile://bye.mp3' });
  s.close();
  check(closed && closed.stopped === true && contexts[contexts.length - 1].destroyed === 1, 'close did not stop playback');
}

// 5. BCP 47 tag resolution rules.
check(speakMod.resolveWeChatLang('zh_Hans_CN', '').lang === 'zh_CN', 'zh_Hans_CN');
check(speakMod.resolveWeChatLang('zh-TW', '').error === 'language-unavailable', 'zh-TW must not silently map to zh_CN');
check(speakMod.resolveWeChatLang('en-GB', '').error === 'language-unavailable', 'en-GB must not silently map to en_US');
`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestMiniProgramSpeakProjectConfig(t *testing.T) {
	wechat, _ := miniProgramPlatformOf("miniprogram_wechat")

	// Default WeChat project does not declare WechatSI or speechPlugin.
	defDir := t.TempDir()
	defProj := &project{Config: projectcfg.Config{Dir: t.TempDir(), Name: "Demo", ID: "dev.demo"}}
	if err := writeMiniProgram(defProj, wechat, defDir, []byte("module")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(defDir, "nsgo", "speak.js")); err != nil {
		t.Fatalf("missing nsgo/speak.js: %v", err)
	}
	appRaw, err := os.ReadFile(filepath.Join(defDir, "app.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(appRaw), "WechatSI") || strings.Contains(string(appRaw), `"plugins"`) {
		t.Fatalf("default app.json unexpectedly declares plugins: %s", appRaw)
	}
	cfgRaw, err := os.ReadFile(filepath.Join(defDir, "nsgo", "config.js"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(cfgRaw), "speechPlugin") {
		t.Fatalf("default config.js unexpectedly sets speechPlugin: %s", cfgRaw)
	}

	// Opt-in WeChat project declares WechatSI in app.json and speechPlugin in config.js.
	optDir := t.TempDir()
	optProj := &project{Config: projectcfg.Config{Dir: t.TempDir(), Name: "Demo", ID: "dev.demo"}}
	optProj.MiniProgram.WeChatSpeechPlugin = "0.3.5"
	if err := writeMiniProgram(optProj, wechat, optDir, []byte("module")); err != nil {
		t.Fatal(err)
	}
	appRaw, err = os.ReadFile(filepath.Join(optDir, "app.json"))
	if err != nil {
		t.Fatal(err)
	}
	var app struct {
		Plugins map[string]struct {
			Version  string `json:"version"`
			Provider string `json:"provider"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal(appRaw, &app); err != nil {
		t.Fatal(err)
	}
	plugin, ok := app.Plugins["WechatSI"]
	if !ok || plugin.Version != "0.3.5" || plugin.Provider != "wx069ba97219f66d99" {
		t.Fatalf("app.json plugins = %+v", app.Plugins)
	}
	cfgRaw, err = os.ReadFile(filepath.Join(optDir, "nsgo", "config.js"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(cfgRaw), `"speechPlugin": "WechatSI"`) {
		t.Fatalf("config.js = %s", cfgRaw)
	}

	// Non-WeChat platforms never emit WechatSI.
	for _, target := range []string{"miniprogram_qq", "miniprogram_alipay", "miniprogram_douyin"} {
		mp, _ := miniProgramPlatformOf(target)
		dir := t.TempDir()
		if err := writeMiniProgram(optProj, mp, dir, []byte("module")); err != nil {
			t.Fatal(err)
		}
		data, _ := os.ReadFile(filepath.Join(dir, "app.json"))
		if strings.Contains(string(data), "WechatSI") {
			t.Errorf("%s app.json unexpectedly declares WechatSI: %s", mp.name, data)
		}
		cfg, _ := os.ReadFile(filepath.Join(dir, "nsgo", "config.js"))
		if strings.Contains(string(cfg), "speechPlugin") {
			t.Errorf("%s config.js unexpectedly sets speechPlugin: %s", mp.name, cfg)
		}
	}

	// nsgo.yaml parsing accepts a valid plugin version and rejects whitespace.
	yamlDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(yamlDir, "nsgo.yaml"), []byte("name: Demo\nminiprogram:\n  wechat_speech_plugin: \"0.3.5\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := load(yamlDir)
	if err != nil {
		t.Fatalf("load valid wechat_speech_plugin: %v", err)
	}
	if loaded.MiniProgram.WeChatSpeechPlugin != "0.3.5" {
		t.Fatalf("loaded version = %q", loaded.MiniProgram.WeChatSpeechPlugin)
	}
	badDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(badDir, "nsgo.yaml"), []byte("name: Demo\nminiprogram:\n  wechat_speech_plugin: \"0.3.5 beta\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := load(badDir); err == nil {
		t.Fatal("expected invalid wechat_speech_plugin to fail load")
	}
}
