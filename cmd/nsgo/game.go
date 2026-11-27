package main

func (p *project) linuxCategories() []string { return p.Config.LinuxCategories() }

// gamePlist writes the app's game entries. Game Mode is available on macOS and iOS, and the legacy key opts in on older OS versions. remote adds the Micro Gamepad profile of tvOS's Siri Remote, which package gamepad reads there as a controller.
func (p *project) gamePlist(gameMode, remote bool) string {
	info := map[string]any{}
	if gameMode && p.Game {
		info["GCSupportsGameMode"] = true
		info["LSSupportsGameMode"] = true
	}
	if p.Gamepad {
		info["GCSupportsControllerUserInteraction"] = true
		profiles := []any{map[string]any{"ProfileName": "ExtendedGamepad"}}
		if remote {
			profiles = append(profiles, map[string]any{"ProfileName": "MicroGamepad"})
		}
		info["GCSupportedGameControllers"] = profiles
	}
	for key := range p.Apple.Info {
		delete(info, key)
	}
	if len(info) == 0 {
		return ""
	}
	return plistEntries(string(encodePlist(info)))
}
