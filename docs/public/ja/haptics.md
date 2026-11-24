---
title: ハプティクス
description: プリセットの触覚フィードバックや、タイミングを指定した連続パルスを再生します。
group: プラットフォーム機能
order: 46
---

## 単発で再生する

`mobile.PlayHaptic` は iOS と Android でユーザーの指先にフィードバックを再生し、その他のプラットフォームでは何もしないため、画面上の視覚的な変化と組み合わせて使います。`HapticSelection` は値の変化やピッカー、セグメント、スライダーのステップを示し、`HapticLight` から `HapticRigid` は要素同士の衝突を重さや硬さに応じて示します。そして `HapticSuccess`、`HapticWarning`、`HapticError` はタスクがそのように終了したことを示します。

```go
mobile.PlayHaptic(cx, mobile.HapticSelection)
```

## パターンを再生する

`cx.Env().CanPlayHapticPattern()` はタイミング指定のハプティクスが利用可能かどうかを返します。パターンには、パターン開始からの相対時刻 `At` を持つ複数のパルスを含めます。`Duration` が 0 の場合は瞬間的な振動（トランジェント）になり、正の値を指定すると最大 30 秒間パルスが持続します。`Intensity` と `Sharpness` は 0 から 1 の範囲で指定します。新しいパターンを再生すると前のパターンが置き換わります。空のパターンを渡すと再生を停止します。アプリがバックグラウンドに移行するとパターンは停止します。

```go
mobile.PlayHapticPattern(cx, mobile.HapticPattern{
    {Intensity: 0.25, Sharpness: 0.4},
    {At: 100 * time.Millisecond, Intensity: 0.55, Sharpness: 0.6},
    {At: 200 * time.Millisecond, Intensity: 1, Sharpness: 0.8},
})
```

iOS では対応ハードウェア上で Core Haptics を使ってイベントを再生します。Android ではタイミングに従い、バイブレータが振幅制御に対応している場合は強さも反映します。`navigator.vibrate` を備えた Android ブラウザでは、ユーザー操作後にオン・オフのタイミングに従って動作します。Android と Web ではトランジェントを 20 ms のパルスとして再生し、シャープネスは iOS でのみ利用できます。非対応プラットフォームでは、この呼び出しは何の効果も持ちません。

テストでは、`nsgotest.Environment` を通じて `HapticPatterns: true` を設定し、`app.HapticPatterns` を読み取ります。アプリが再生をキャンセルした場合、履歴には nil エントリが含まれます。

## いつ再生するか

結果が届いたときではなく、ジェスチャーが起きた瞬間に再生します。値が通過したときの選択、ビューが着地したときの衝撃、タスクが終わったときの結果に対して鳴らしてください。ユーザーが引き起こした変化に対してのみ再生し、アプリが自律的に行った変化では再生しないでください。

## 次のステップ

- [通知](notifications.md): 同じ知らせを伝える通知とバッジ。
- [プラットフォームサービス](services.md): システムが提供するその他の機能。
- [アクセシビリティ](accessibility.md): 振動を感じ取れないユーザーに届く音声案内。
