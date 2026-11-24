---
title: Resposta tátil
description: Reproduza feedbacks táteis predefinidos ou uma sequência temporizada de pulsos.
group: Recursos de plataforma
order: 46
---

## Reproduzindo um feedback

`mobile.PlayHaptic` reproduz resposta tátil sob o dedo do usuário no iOS e no Android, e não faz nada em outras plataformas, portanto combine-a com o que o usuário vê na tela. `HapticSelection` marca uma mudança de valor, o passo de um seletor, segmento ou controle deslizante; `HapticLight` até `HapticRigid` marcam elementos colidindo, de acordo com seu peso e elasticidade; e `HapticSuccess`, `HapticWarning` e `HapticError` indicam o desfecho de uma tarefa.

```go
mobile.PlayHaptic(cx, mobile.HapticSelection)
```

## Reproduzindo um padrão

`cx.Env().CanPlayHapticPattern()` informa se a resposta tátil temporizada está disponível. Um padrão contém pulsos com um tempo de início `At` relativo ao padrão. Um `Duration` zero gera um pulso transiente, e uma duração positiva sustenta o pulso por até 30 segundos. `Intensity` e `Sharpness` variam de 0 a 1. Um novo padrão substitui o anterior. Passe um padrão vazio para interromper a reprodução. O padrão é interrompido quando o app vai para segundo plano.

```go
mobile.PlayHapticPattern(cx, mobile.HapticPattern{
    {Intensity: 0.25, Sharpness: 0.4},
    {At: 100 * time.Millisecond, Intensity: 0.55, Sharpness: 0.6},
    {At: 200 * time.Millisecond, Intensity: 1, Sharpness: 0.8},
})
```

O iOS reproduz os eventos via Core Haptics em hardware compatível. O Android segue a temporização e usa a intensidade quando o motor de vibração suporta controle de amplitude. Navegadores no Android com `navigator.vibrate` seguem os tempos ligado/desligado após uma interação do usuário. O Android e a web reproduzem um pulso transiente como um pulso de 20 ms, e `Sharpness` está disponível apenas no iOS. Em uma plataforma sem suporte, a chamada não tem efeito.

Nos testes, defina `HapticPatterns: true` por meio de `nsgotest.Environment` e leia `app.HapticPatterns`. O histórico inclui uma entrada `nil` quando o app cancela a reprodução.

## Quando reproduzir

Reproduza no momento em que o gesto acontece, não quando a resposta chega: uma seleção quando o valor muda, um impacto quando uma view pousa, um desfecho quando a tarefa termina. Reproduza para mudanças causadas pelo usuário, e nunca para o que o app fez por conta própria.

## Próximos passos

- [Notificações](notifications.md): as notificações e os badges que levam a mesma informação.
- [Serviços de plataforma](services.md): o restante do que o sistema oferece.
- [Acessibilidade](accessibility.md): o anúncio que um usuário que não sente o toque ouve.
