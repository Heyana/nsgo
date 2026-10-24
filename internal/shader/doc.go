// Package shader compiles shaders written in Go's syntax into the
// intermediate representation of package shaderir, which its backends
// translate for each platform's GPU API.
//
// It is Ebitengine's Kage compiler (github.com/hajimehoshi/ebiten,
// internal/shader and internal/shaderir at commit 16f18b0), under the
// Apache License 2.0 in LICENSE, as MyGo (github.com/egoist/mygo, v0.6.3,
// MIT License) forked it to compile fragment shaders as libraries. Files
// that came from Ebitengine keep its copyright notice and say where MyGo
// changed them. NSGo's bridge (bridge.go) declares only the view's size
// and scale: shaders read no images yet.
package shader
