package main

import "slices"

// MacOSCategory returns the bundle's category, with Games as the default
// for a project marked as a game. apple.info may provide the category
// directly when macos.category is unset.
func (p *project) MacOSCategory() string {
	_, custom := p.Apple.Info["LSApplicationCategoryType"]
	if p.MacOS.Category == "" && p.Game && !custom {
		return "public.app-category.games"
	}
	return p.MacOS.Category
}

func (p *project) linuxCategories() []string {
	categories := p.Linux.Categories
	if p.Game && !slices.Contains(categories, "Game") {
		return append(slices.Clone(categories), "Game")
	}
	return categories
}

// gamePlist writes the app's game entries. Game Mode is available on
// macOS and iOS. The legacy key opts in on older OS versions.
func (p *project) gamePlist(gameMode bool) string {
	info := map[string]any{}
	if gameMode && p.Game {
		info["GCSupportsGameMode"] = true
		info["LSSupportsGameMode"] = true
	}
	if p.Gamepad {
		info["GCSupportsControllerUserInteraction"] = true
		info["GCSupportedGameControllers"] = []any{map[string]any{"ProfileName": "ExtendedGamepad"}}
	}
	for key := range p.Apple.Info {
		delete(info, key)
	}
	if len(info) == 0 {
		return ""
	}
	return plistEntries(string(encodePlist(info)))
}
