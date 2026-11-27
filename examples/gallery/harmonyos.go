//go:build nsgoharmony

package main

// omitted are the demos this build leaves out: a Shader view fills itself with
// WGSL on the GPU, which ArkTS has no WebGL or WGSL surface for yet
// (docs/architecture/harmonyos.md#constraints).
var omitted = map[string]bool{"Shader": true}
