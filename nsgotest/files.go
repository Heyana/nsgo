package nsgotest

import (
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

type saveKey struct {
	tree *core.Tree
	id   uint32
}

type saveDestination struct {
	request driver.SaveRequest
	path    string
}

func (a *App) chooseSaveDestination(it driver.MountItem) {
	a.Saves = append(a.Saves, *it.Save)
	if a.target != nil {
		return
	}
	path := ""
	if a.SaveFile != nil {
		path = a.SaveFile(*it.Save)
	}
	switch {
	case path == "":
		a.post(driver.Event{Kind: driver.EventFiles, Count: it.Count})
	case it.Save.Preflight:
		if a.saveDestinations == nil {
			a.saveDestinations = make(map[saveKey]saveDestination)
		}
		a.saveDestinations[saveKey{a.responseTree, it.Count}] = saveDestination{*it.Save, path}
		a.post(driver.Event{Kind: driver.EventSavePreflight, Count: it.Count, Text: path})
	default:
		a.post(it.Save.SaveResult(it.Count, path))
	}
}

func (a *App) answerSavePreflight(it driver.MountItem) {
	key := saveKey{a.responseTree, it.Count}
	destination, ok := a.saveDestinations[key]
	if !ok {
		return
	}
	delete(a.saveDestinations, key)
	if it.On {
		a.post(destination.request.SaveResult(it.Count, destination.path))
	} else {
		a.post(driver.Event{Kind: driver.EventFiles, Count: it.Count, Data: destination.path, Text: it.Value})
	}
}
