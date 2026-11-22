//go:build linux && !android

package linux

import (
	"errors"
	"os"
	"sync"
	"time"

	"github.com/ebitengine/purego"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

type geoClueRunKey struct {
	w  *window
	id uint32
}

type geoClueRun struct {
	key  geoClueRunKey
	req  driver.SensorRequest
	stop chan struct{}
}

var geoClueRuns = struct {
	sync.Mutex
	m map[geoClueRunKey]*geoClueRun
}{m: map[geoClueRunKey]*geoClueRun{}}

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
	purego.RegisterLibFunc(&geoVariantGetInt64, glibLib, "g_variant_get_int64")
})

var geoClueBus = sync.OnceValue(func() uintptr {
	if address := os.Getenv("NSGO_GEOCLUE_BUS_ADDRESS"); address != "" {
		var err uintptr
		bus := gDBusConnectionNewForAddressSync(cstr(address), 1<<0|1<<3, 0, 0, &err)
		if err != 0 {
			gErrorFree(err)
			return 0
		}
		return bus
	}
	var err uintptr
	bus := gBusGetSync(1, 0, &err) // GeoClue2 owns a system-bus name.
	if err != 0 {
		gErrorFree(err)
		return 0
	}
	return bus
})

var (
	geoVariantNewUint32  func(uint32) uintptr
	geoVariantNewVariant func(uintptr) uintptr
	geoVariantGetDouble  func(uintptr) float64
	geoVariantGetInt64   func(uintptr) int64
)

func handleGeoClue(w *window, it *driver.MountItem) {
	req := *it.Sensor
	key := geoClueRunKey{w: w, id: it.Count}
	if req.Command == driver.SensorStop {
		geoClueRuns.Lock()
		run := geoClueRuns.m[key]
		delete(geoClueRuns.m, key)
		geoClueRuns.Unlock()
		if run != nil {
			close(run.stop)
		}
		return
	}
	if (req.Kind != driver.SensorLocation && req.Kind != driver.SensorBackgroundLocation) ||
		(req.Command != driver.SensorReadPermission && req.Command != driver.SensorRequestPermission && req.Command != driver.SensorLocate && req.Command != driver.SensorWatchLocation) {
		w.engine.Dispatch(driver.NoSensor(it))
		return
	}
	if req.Kind == driver.SensorMotion {
		w.engine.Dispatch(driver.NoSensor(it))
		return
	}
	if req.Command == driver.SensorWatchLocation {
		if req.Location.Background {
			w.engine.Dispatch(driver.NoSensor(it))
			return
		}
		run := &geoClueRun{key: key, req: req, stop: make(chan struct{})}
		geoClueRuns.Lock()
		geoClueRuns.m[key] = run
		geoClueRuns.Unlock()
		geoClueNext(run)
		return
	}
	portalQueue() <- func() {
		if req.Command <= driver.SensorRequestPermission {
			err := geoClueAvailable()
			w.post(func() {
				if !w.closing {
					status := driver.SensorGranted
					if err != nil {
						status = driver.SensorUnavailable
					}
					w.engine.Dispatch(driver.Event{Kind: driver.EventSensor, Count: it.Count, Data: &driver.SensorReport{Generation: req.Generation, Grant: &driver.SensorGrant{Authorization: status}, End: true}})
				}
			})
			return
		}
		var grant driver.SensorAuthorization
		fix, err := geoClueLocate(w.opts.ID)
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

func geoClueNext(run *geoClueRun) {
	portalQueue() <- func() {
		fix, err := geoClueLocate(run.key.w.opts.ID)
		run.key.w.post(func() {
			geoClueRuns.Lock()
			_, active := geoClueRuns.m[run.key]
			geoClueRuns.Unlock()
			if !active || run.key.w.closing {
				return
			}
			if err != nil {
				geoClueRuns.Lock()
				delete(geoClueRuns.m, run.key)
				geoClueRuns.Unlock()
				run.key.w.engine.Dispatch(driver.Event{Kind: driver.EventSensor, Count: run.key.id, Data: &driver.SensorReport{Generation: run.req.Generation, Error: "unavailable", End: true}})
				close(run.stop)
				return
			}
			if run.key.w.engine != nil {
				run.key.w.engine.Dispatch(driver.Event{Kind: driver.EventSensor, Count: run.key.id, Data: &driver.SensorReport{Generation: run.req.Generation, Location: &fix}})
			}
			interval := run.req.Location.Interval
			if interval < time.Second {
				interval = time.Second
			}
			time.AfterFunc(interval, func() {
				select {
				case <-run.stop:
				default:
					geoClueNext(run)
				}
			})
		})
	}
}

func geoClueAvailable() error {
	bindGeoClue()
	bus := geoClueBus()
	if bus == 0 {
		return errors.New("system bus unavailable")
	}
	reply, err := dbusTry(bus, geoClueName, geoClueManager, geoClueManagerIF, "GetClient")
	gVariantUnrefNonZero(reply)
	return err
}

func geoClueLocate(desktopID string) (driver.LocationFix, error) {
	bindGeoClue()
	bus := geoClueBus()
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
	if desktopID == "" {
		desktopID = "nsgo"
	}
	defer func() { gVariantUnrefNonZero(dbusCall(bus, geoClueName, client, geoClueClientIF, "Stop")) }()
	if _, err = dbusTry(bus, geoClueName, client, propertiesIF, "Set", gVariantNewString(cstr(geoClueClientIF)), gVariantNewString(cstr("DesktopId")), geoVariantNewVariant(gVariantNewString(cstr(desktopID)))); err != nil {
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
	var path string
	for attempt := 0; attempt < 20 && path == ""; attempt++ {
		locationReply, getErr := dbusTry(bus, geoClueName, client, propertiesIF, "Get", gVariantNewString(cstr(geoClueClientIF)), gVariantNewString(cstr("Location")))
		if getErr != nil {
			return driver.LocationFix{}, getErr
		}
		location := geoTupleVariant(locationReply, 0)
		gVariantUnref(locationReply)
		if location != 0 {
			path = geoVariantObjectPath(location)
			gVariantUnref(location)
		}
		if path == "" || path == "/" {
			path = ""
			time.Sleep(100 * time.Millisecond)
		}
	}
	if path == "" {
		return driver.LocationFix{}, errors.New("GeoClue did not publish a location")
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
	timestamp := time.Now()
	if raw, timestampErr := readInt64(bus, geoClueName, path, "Timestamp"); timestampErr == nil {
		const unixToWindows100ns = int64(11644473600) * 10000000
		if raw > unixToWindows100ns {
			timestamp = time.Unix(0, (raw-unixToWindows100ns)*100)
		}
	}
	return driver.LocationFix{Latitude: lat, Longitude: lon, HorizontalAccuracy: driver.SensorNumber{Value: accuracy, Valid: accuracy >= 0}, Timestamp: timestamp}, nil
}

func readInt64(bus uintptr, name, path, property string) (int64, error) {
	reply, err := dbusTry(bus, name, path, propertiesIF, "Get", gVariantNewString(cstr(geoClueLocationIF)), gVariantNewString(cstr(property)))
	if err != nil {
		return 0, err
	}
	defer gVariantUnref(reply)
	value := geoTupleVariant(reply, 0)
	if value == 0 {
		return 0, errors.New("GeoClue property missing")
	}
	defer gVariantUnref(value)
	return geoVariantGetInt64(value), nil
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
