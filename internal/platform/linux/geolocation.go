//go:build linux && !android

package linux

import (
	"errors"
	"sync"
	"time"

	"github.com/ebitengine/purego"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

const (
	geoClueName       = "org.freedesktop.GeoClue2"
	geoClueManager    = "/org/freedesktop/GeoClue2/Manager"
	geoClueManagerIF  = "org.freedesktop.GeoClue2.Manager"
	geoClueClientIF   = "org.freedesktop.GeoClue2.Client"
	geoClueLocationIF = "org.freedesktop.GeoClue2.Location"
	propertiesIF      = "org.freedesktop.DBus.Properties"
)

var bindGeoClue = sync.OnceFunc(func() {
	bindInputSources()
	purego.RegisterLibFunc(&geoVariantNewUint32, glibLib, "g_variant_new_uint32")
	purego.RegisterLibFunc(&geoVariantNewVariant, glibLib, "g_variant_new_variant")
	purego.RegisterLibFunc(&geoVariantGetDouble, glibLib, "g_variant_get_double")
})

var (
	geoVariantNewUint32  func(uint32) uintptr
	geoVariantNewVariant func(uintptr) uintptr
	geoVariantGetDouble  func(uintptr) float64
)

func handleGeoClue(w *window, it *driver.MountItem) {
	req := *it.Sensor
	if (req.Kind != driver.SensorLocation && req.Kind != driver.SensorBackgroundLocation) ||
		(req.Command != driver.SensorReadPermission && req.Command != driver.SensorRequestPermission && req.Command != driver.SensorLocate && req.Command != driver.SensorWatchLocation) {
		w.engine.Dispatch(driver.NoSensor(it))
		return
	}
	if req.Command == driver.SensorWatchLocation || req.Kind == driver.SensorMotion {
		w.engine.Dispatch(driver.NoSensor(it))
		return
	}
	portalQueue() <- func() {
		var grant driver.SensorAuthorization
		fix, err := geoClueLocate()
		if err == nil {
			grant = driver.SensorGranted
		} else {
			grant = driver.SensorUnavailable
		}
		w.post(func() {
			if w.closing {
				return
			}
			if req.Command <= driver.SensorRequestPermission {
				w.engine.Dispatch(driver.Event{Kind: driver.EventSensor, Count: it.Count, Data: &driver.SensorReport{Generation: req.Generation, Grant: &driver.SensorGrant{Authorization: grant}, End: true}})
				return
			}
			if err != nil {
				w.engine.Dispatch(driver.Event{Kind: driver.EventSensor, Count: it.Count, Data: &driver.SensorReport{Generation: req.Generation, Error: "unavailable", End: true}})
				return
			}
			w.engine.Dispatch(driver.Event{Kind: driver.EventSensor, Count: it.Count, Data: &driver.SensorReport{Generation: req.Generation, Location: &fix, End: true}})
		})
	}
}

func geoClueLocate() (driver.LocationFix, error) {
	bindGeoClue()
	bus := sessionBus()
	if bus == 0 {
		return driver.LocationFix{}, errors.New("session bus unavailable")
	}
	manager, err := dbusTry(bus, geoClueName, geoClueManager, geoClueManagerIF, "GetClient")
	if err != nil {
		return driver.LocationFix{}, err
	}
	defer gVariantUnref(manager)
	client := geoTupleObjectPath(manager, 0)
	if client == "" {
		return driver.LocationFix{}, errors.New("GeoClue returned no client")
	}
	if _, err = dbusTry(bus, geoClueName, client, propertiesIF, "Set", gVariantNewString(cstr(geoClueClientIF)), gVariantNewString(cstr("DesktopId")), geoVariantNewVariant(gVariantNewString(cstr("nsgo")))); err != nil {
		return driver.LocationFix{}, err
	}
	if _, err = dbusTry(bus, geoClueName, client, propertiesIF, "Set", gVariantNewString(cstr(geoClueClientIF)), gVariantNewString(cstr("RequestedAccuracyLevel")), geoVariantNewVariant(geoVariantNewUint32(6))); err != nil {
		return driver.LocationFix{}, err
	}
	if reply, callErr := dbusTry(bus, geoClueName, client, geoClueClientIF, "Start"); callErr != nil {
		return driver.LocationFix{}, callErr
	} else {
		gVariantUnref(reply)
	}
	locationReply, err := dbusTry(bus, geoClueName, client, propertiesIF, "Get", gVariantNewString(cstr(geoClueClientIF)), gVariantNewString(cstr("Location")))
	if err != nil {
		return driver.LocationFix{}, err
	}
	defer gVariantUnref(locationReply)
	location := geoTupleVariant(locationReply, 0)
	if location == 0 {
		return driver.LocationFix{}, errors.New("GeoClue returned no location")
	}
	defer gVariantUnref(location)
	path := geoVariantObjectPath(location)
	if path == "" {
		return driver.LocationFix{}, errors.New("GeoClue location path is empty")
	}
	read := func(name string) (float64, error) {
		reply, err := dbusTry(bus, geoClueName, path, propertiesIF, "Get", gVariantNewString(cstr(geoClueLocationIF)), gVariantNewString(cstr(name)))
		if err != nil {
			return 0, err
		}
		defer gVariantUnref(reply)
		value := geoTupleVariant(reply, 0)
		if value == 0 {
			return 0, errors.New("GeoClue property missing")
		}
		defer gVariantUnref(value)
		return geoVariantGetDouble(value), nil
	}
	lat, err := read("Latitude")
	if err != nil {
		return driver.LocationFix{}, err
	}
	lon, err := read("Longitude")
	if err != nil {
		return driver.LocationFix{}, err
	}
	accuracy, _ := read("Accuracy")
	return driver.LocationFix{Latitude: lat, Longitude: lon, HorizontalAccuracy: driver.SensorNumber{Value: accuracy, Valid: accuracy >= 0}, Timestamp: time.Now()}, nil
}

func geoTupleObjectPath(tuple uintptr, index int) string {
	child := gVariantGetChildValue(tuple, uintptr(index))
	defer gVariantUnref(child)
	return geoVariantObjectPath(child)
}

func geoTupleVariant(tuple uintptr, index int) uintptr {
	child := gVariantGetChildValue(tuple, uintptr(index))
	if child == 0 {
		return 0
	}
	value := gVariantGetVariant(child)
	gVariantUnref(child)
	return value
}

func geoVariantObjectPath(value uintptr) string {
	if goString(gVariantGetTypeString(value)) != "o" {
		return ""
	}
	return goString(gVariantGetString(value, 0))
}
