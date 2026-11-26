package driver

// ScopeCleanup registers fn on the current owner of scope, a *core.Scope, as Scope.Cleanup does, and returns a function that takes it off again, after which the owner never runs fn and keeps nothing of it. core sets it; the gpu package registers each resource's Close with it, so a closed resource leaves nothing on the scope it was created under.
var ScopeCleanup func(scope any, fn func()) (remove func())
