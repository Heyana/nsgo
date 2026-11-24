//go:build linux && !android

package linux

import (
	"errors"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ebitengine/purego"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Location comes from GeoClue 2 on the system bus. Each request starts a GeoClue client of its own under the app's desktop ID, nsgo.yaml's id or else the executable's name, which is what GeoClue's agent (GNOME Shell, KDE's) asks the user about and remembers, and stops it when the request ends. GeoClue has no way to read an app's authorization without asking, so the permission reads as not asked while the service is there, and as the last answer this process got once a client started or was refused; requesting it starts a client and waits for the agent's answer. Background location is unavailable: a desktop app has no background state to keep a stream going in. NSGO_GEOCLUE_BUS_ADDRESS points the provider at a private bus, as the tests do.

const (
	geoClueName       = "org.freedesktop.GeoClue2"
	geoClueManager    = "/org/freedesktop/GeoClue2/Manager"
	geoClueManagerIF  = "org.freedesktop.GeoClue2.Manager"
	geoClueClientIF   = "org.freedesktop.GeoClue2.Client"
	geoClueLocationIF = "org.freedesktop.GeoClue2.Location"
	propertiesIF      = "org.freedesktop.DBus.Properties"
)

// geoClueStartTimeout bounds Client.Start, which returns only once the agent has the user's answer.
const geoClueStartTimeout = 2 * time.Minute

// geoCluePoll is how often a started client's Location property is read: GeoClue publishes each new fix as a new Location object.
const geoCluePoll = 100 * time.Millisecond

var errGeoClueDenied = errors.New("GeoClue refused the app")

// geoClueGrant is the last authorization a client start answered in this process, SensorNotAsked before one did.
var geoClueGrant atomic.Uint32

type geoClueRunKey struct {
	w  *window
	id uint32
}

type geoClueRun struct {
	key  geoClueRunKey
	req  driver.SensorRequest
	stop chan struct{}
	once sync.Once
}

func (r *geoClueRun) cancel() { r.once.Do(func() { close(r.stop) }) }

func (r *geoClueRun) stopped() bool {
	select {
	case <-r.stop:
		return true
	default:
		return false
	}
}

var geoClueRuns = struct {
	sync.Mutex
	m map[geoClueRunKey]*geoClueRun
}{m: map[geoClueRunKey]*geoClueRun{}}

var bindGeoClue = sync.OnceFunc(func() {
	bindInputSources()
	purego.RegisterLibFunc(&geoVariantNewUint32, glibLib, "g_variant_new_uint32")
	purego.RegisterLibFunc(&geoVariantNewVariant, glibLib, "g_variant_new_variant")
	purego.RegisterLibFunc(&geoVariantGetDouble, glibLib, "g_variant_get_double")
	purego.RegisterLibFunc(&geoVariantGetUint64, glibLib, "g_variant_get_uint64")
})

var (
	geoVariantNewUint32  func(uint32) uintptr
	geoVariantNewVariant func(uintptr) uintptr
	geoVariantGetDouble  func(uintptr) float64
	geoVariantGetUint64  func(uintptr) uint64
)

// geoClueBus is the bus GeoClue owns its name on, 0 without one.
var geoClueBus = sync.OnceValue(func() uintptr {
	bindGeoClue()
	var err uintptr
	var bus uintptr
	if address := os.Getenv("NSGO_GEOCLUE_BUS_ADDRESS"); address != "" {
		const authClient, messageBus = 1 << 0, 1 << 3
		bus = gDBusConnectionNewForAddressSync(cstr(address), authClient|messageBus, 0, 0, &err)
	} else {
		const busSystem = 1
		bus = gBusGetSync(busSystem, 0, &err)
	}
	if err != 0 {
		gErrorFree(err)
		return 0
	}
	return bus
})

// handleGeoClue serves an ItemSensor. Sessions run on goroutines of their own, as GeoClue's calls block, and answer on the main thread.
func handleGeoClue(w *window, it *driver.MountItem) {
	req := *it.Sensor
	key := geoClueRunKey{w: w, id: it.Count}
	if req.Command == driver.SensorStop {
		geoClueRuns.Lock()
		run := geoClueRuns.m[key]
		if run != nil && run.req.Generation == req.Generation {
			delete(geoClueRuns.m, key)
		} else {
			run = nil
		}
		geoClueRuns.Unlock()
		if run != nil {
			run.cancel()
		}
		return
	}
	if req.Kind != driver.SensorLocation || req.Location.Background {
		w.engine.Dispatch(driver.NoSensor(it))
		return
	}
	switch req.Command {
	case driver.SensorReadPermission:
		go func() {
			status := driver.SensorAuthorization(geoClueGrant.Load())
			if !geoClueAvailable() {
				status = driver.SensorUnavailable
			}
			geoClueAnswer(w, it.Count, &driver.SensorReport{Generation: req.Generation, Grant: &driver.SensorGrant{Authorization: status}, End: true})
		}()
	case driver.SensorRequestPermission, driver.SensorLocate, driver.SensorWatchLocation:
		run := &geoClueRun{key: key, req: req, stop: make(chan struct{})}
		geoClueRuns.Lock()
		if old := geoClueRuns.m[key]; old != nil {
			old.cancel()
		}
		geoClueRuns.m[key] = run
		geoClueRuns.Unlock()
		go run.serve()
	default:
		w.engine.Dispatch(driver.NoSensor(it))
	}
}

// stopGeoClue ends the window's sessions as it closes.
func stopGeoClue(w *window) {
	geoClueRuns.Lock()
	for key, run := range geoClueRuns.m {
		if key.w == w {
			delete(geoClueRuns.m, key)
			run.cancel()
		}
	}
	geoClueRuns.Unlock()
}

// geoClueAnswer dispatches r on the main thread unless the window is closing.
func geoClueAnswer(w *window, id uint32, r *driver.SensorReport) {
	w.post(func() {
		if !w.closing {
			w.engine.Dispatch(driver.Event{Kind: driver.EventSensor, Count: id, Data: r})
		}
	})
}

// serve starts the run's client, answers the permission request or delivers its fixes, and stops the client.
func (r *geoClueRun) serve() {
	w, id, gen := r.key.w, r.key.id, r.req.Generation
	defer func() {
		geoClueRuns.Lock()
		if geoClueRuns.m[r.key] == r {
			delete(geoClueRuns.m, r.key)
		}
		geoClueRuns.Unlock()
	}()
	exe, _ := os.Executable()
	c, err := startGeoClueClient(awakeAppID(w.opts.ID, "", exe), r.req.Location)
	if r.req.Command == driver.SensorRequestPermission {
		if c != nil {
			c.stop()
		}
		status := driver.SensorGranted
		switch {
		case errors.Is(err, errGeoClueDenied):
			status = driver.SensorDenied
		case err != nil:
			status = driver.SensorUnavailable
		}
		if !r.stopped() {
			geoClueAnswer(w, id, &driver.SensorReport{Generation: gen, Grant: &driver.SensorGrant{Authorization: status}, End: true})
		}
		return
	}
	if err != nil {
		code := "unavailable"
		if errors.Is(err, errGeoClueDenied) {
			code = "not-allowed"
		}
		if !r.stopped() {
			geoClueAnswer(w, id, &driver.SensorReport{Generation: gen, Error: code, End: true})
		}
		return
	}
	defer c.stop()
	var packet driver.SensorPacket
	last := ""
	tick := time.NewTicker(geoCluePoll)
	defer tick.Stop()
	for {
		select {
		case <-r.stop:
			return
		case <-tick.C:
		}
		path, err := c.locationPath()
		if err != nil {
			if !r.stopped() {
				geoClueAnswer(w, id, &driver.SensorReport{Generation: gen, Error: "unavailable", End: true})
			}
			return
		}
		if path == "" || path == last {
			continue
		}
		last = path
		fix, err := c.fix(path)
		if err != nil {
			continue
		}
		if r.stopped() {
			return
		}
		if r.req.Command == driver.SensorLocate {
			geoClueAnswer(w, id, &driver.SensorReport{Generation: gen, Location: &fix, End: true})
			return
		}
		if packet.Offer(driver.SensorReport{Generation: gen, Location: &fix}) {
			w.post(func() {
				if !w.closing {
					w.engine.Dispatch(driver.Event{Kind: driver.EventSensor, Count: id, Data: &packet})
				}
			})
		}
	}
}

// geoClueAvailable reports whether the bus has GeoClue or can start it.
func geoClueAvailable() bool {
	bus := geoClueBus()
	if bus == 0 {
		return false
	}
	reply, err := geoClueCall(bus, "/org/freedesktop/GeoClue2/Manager", "org.freedesktop.DBus.Peer", "Ping", time.Second)
	gVariantUnrefNonZero(reply)
	return err == nil
}

type geoClueClient struct {
	bus     uintptr
	path    string
	started time.Time
}

// startGeoClueClient gets a client, sets the app's desktop ID, accuracy and thresholds, and starts it.
func startGeoClueClient(desktopID string, opts driver.LocationOptions) (*geoClueClient, error) {
	bus := geoClueBus()
	if bus == 0 {
		return nil, errors.New("GeoClue's bus is unavailable")
	}
	reply, err := geoClueCall(bus, geoClueManager, geoClueManagerIF, "GetClient", 5*time.Second)
	if err != nil {
		return nil, err
	}
	path := geoTupleObjectPath(reply, 0)
	gVariantUnref(reply)
	if path == "" {
		return nil, errors.New("GeoClue returned no client")
	}
	c := &geoClueClient{bus: bus, path: path}
	set := func(name string, value uintptr) error {
		reply, err := geoClueCall(bus, path, propertiesIF, "Set", 5*time.Second, gVariantNewString(cstr(geoClueClientIF)), gVariantNewString(cstr(name)), geoVariantNewVariant(value))
		gVariantUnrefNonZero(reply)
		return err
	}
	if err := set("DesktopId", gVariantNewString(cstr(desktopID))); err != nil {
		return nil, err
	}
	if err := set("RequestedAccuracyLevel", geoVariantNewUint32(geoClueAccuracyLevel(opts.Accuracy))); err != nil {
		return nil, err
	}
	if opts.Distance > 0 {
		if err := set("DistanceThreshold", geoVariantNewUint32(uint32(min(opts.Distance, float64(^uint32(0)))))); err != nil {
			return nil, err
		}
	}
	if seconds := opts.Interval / time.Second; seconds > 0 {
		if err := set("TimeThreshold", geoVariantNewUint32(uint32(min(seconds, time.Duration(^uint32(0)))))); err != nil {
			return nil, err
		}
	}
	c.started = time.Now()
	reply, err = geoClueCall(bus, path, geoClueClientIF, "Start", geoClueStartTimeout)
	if err != nil {
		if strings.Contains(err.Error(), "org.freedesktop.DBus.Error.AccessDenied") {
			geoClueGrant.Store(uint32(driver.SensorDenied))
			return nil, errGeoClueDenied
		}
		return nil, err
	}
	gVariantUnref(reply)
	geoClueGrant.Store(uint32(driver.SensorGranted))
	return c, nil
}

// geoClueAccuracyLevel is the GClueAccuracyLevel for a wanted accuracy in meters, exact for zero.
func geoClueAccuracyLevel(meters float64) uint32 {
	const city, neighborhood, street, exact = 4, 5, 6, 8
	switch {
	case meters <= 0 || meters <= 100:
		return exact
	case meters <= 1000:
		return street
	case meters <= 5000:
		return neighborhood
	}
	return city
}

func (c *geoClueClient) stop() {
	reply, _ := geoClueCall(c.bus, c.path, geoClueClientIF, "Stop", 5*time.Second)
	gVariantUnrefNonZero(reply)
}

// locationPath is the client's current Location object, "" before its first fix.
func (c *geoClueClient) locationPath() (string, error) {
	reply, err := geoClueCall(c.bus, c.path, propertiesIF, "Get", 5*time.Second, gVariantNewString(cstr(geoClueClientIF)), gVariantNewString(cstr("Location")))
	if err != nil {
		return "", err
	}
	defer gVariantUnref(reply)
	value := geoTupleVariant(reply, 0)
	if value == 0 {
		return "", nil
	}
	defer gVariantUnref(value)
	if path := geoVariantObjectPath(value); path != "/" {
		return path, nil
	}
	return "", nil
}

// fix reads the Location object at path. GeoClue reports an unknown speed or heading as -1; its accuracy is always known.
func (c *geoClueClient) fix(path string) (driver.LocationFix, error) {
	get := func(name string) (uintptr, error) {
		reply, err := geoClueCall(c.bus, path, propertiesIF, "Get", 5*time.Second, gVariantNewString(cstr(geoClueLocationIF)), gVariantNewString(cstr(name)))
		if err != nil {
			return 0, err
		}
		defer gVariantUnref(reply)
		value := geoTupleVariant(reply, 0)
		if value == 0 {
			return 0, errors.New("GeoClue location property " + name + " missing")
		}
		return value, nil
	}
	double := func(name string) (float64, error) {
		value, err := get(name)
		if err != nil {
			return 0, err
		}
		defer gVariantUnref(value)
		if goString(gVariantGetTypeString(value)) != "d" {
			return 0, errors.New("GeoClue location property " + name + " is not a double")
		}
		return geoVariantGetDouble(value), nil
	}
	lat, err := double("Latitude")
	if err != nil {
		return driver.LocationFix{}, err
	}
	lon, err := double("Longitude")
	if err != nil {
		return driver.LocationFix{}, err
	}
	f := driver.LocationFix{Latitude: lat, Longitude: lon, Timestamp: time.Now()}
	if accuracy, err := double("Accuracy"); err == nil && accuracy >= 0 {
		f.HorizontalAccuracy = driver.SensorNumber{Value: accuracy, Valid: true}
	}
	if speed, err := double("Speed"); err == nil && speed >= 0 {
		f.Speed = driver.SensorNumber{Value: speed, Valid: true}
	}
	if heading, err := double("Heading"); err == nil && heading >= 0 {
		f.Course = driver.SensorNumber{Value: heading, Valid: true}
	}
	if value, err := get("Timestamp"); err == nil {
		if goString(gVariantGetTypeString(value)) == "(tt)" {
			seconds := gVariantGetChildValue(value, 0)
			micros := gVariantGetChildValue(value, 1)
			f.Timestamp = time.Unix(int64(geoVariantGetUint64(seconds)), int64(geoVariantGetUint64(micros))*1000)
			gVariantUnref(seconds)
			gVariantUnref(micros)
		}
		gVariantUnref(value)
	}
	// GeoClue hands a started client the fix its sources last made, which can predate the client when another app keeps them running; the fix is still GeoClue's current one, so it is delivered and marked cached with its age.
	f.Age = max(0, time.Since(f.Timestamp))
	f.Cached = f.Timestamp.Before(c.started.Add(-time.Second))
	return f, nil
}

// geoClueCall calls a GeoClue method as dbusTry does, with its own timeout.
func geoClueCall(bus uintptr, path, iface, method string, timeout time.Duration, args ...uintptr) (uintptr, error) {
	var params uintptr
	if len(args) > 0 {
		params = gVariantNewTuple(&args[0], uintptr(len(args)))
	} else {
		params = gVariantNewTuple(nil, 0)
	}
	var gerr uintptr
	reply := gDBusConnectionCallSync(bus, cstr(geoClueName), cstr(path), cstr(iface), cstr(method), params, 0, 0, int32(timeout/time.Millisecond), 0, &gerr)
	if gerr != 0 {
		defer gErrorFree(gerr)
		return 0, errors.New(goString((*gError)(cPointer(gerr)).message))
	}
	return reply, nil
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
	defer gVariantUnref(child)
	if goString(gVariantGetTypeString(child)) != "v" {
		return 0
	}
	return gVariantGetVariant(child)
}

func geoVariantObjectPath(value uintptr) string {
	if goString(gVariantGetTypeString(value)) != "o" {
		return ""
	}
	return goString(gVariantGetString(value, 0))
}
