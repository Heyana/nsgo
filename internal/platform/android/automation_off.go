//go:build (android || nsgoandroidhost) && !nsgoautomation

package android

func setLiveSurface(*surface)   {}
func clearLiveSurface(*surface) {}
