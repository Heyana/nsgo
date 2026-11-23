//go:build android || nsgoandroidhost

package android

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"log"
	"math"
	"sort"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/imagedecode"
)

// nativeView is what Go keeps of a native view: its component, or for
// one the embedding host registered in Java, host.
type nativeView struct {
	component driver.NativeComponent
	host      bool
}

// nativeKey names a native view: its node in the tree of its surface, 0 for the activity's view.
type nativeKey struct {
	surface uint32
	id      driver.NodeID
}

// hostClass prefixes the name of a component Go does not know, for Java to
// look it up among the host's (NsgoView.registerComponent).
const hostClass = "host:"

// closeNatives closes the components still showing once the app stopped.
func (s *surface) closeNatives() {
	s.cmu.Lock()
	defer s.cmu.Unlock()
	for key, v := range s.natives {
		if v.component != nil {
			v.component.Close()
		}
		delete(s.natives, key)
	}
}

// closeSurfaceNatives closes the components of surface id's tree, which closed without deleting its views.
func (s *surface) closeSurfaceNatives(id uint32) {
	s.cmu.Lock()
	defer s.cmu.Unlock()
	for key, v := range s.natives {
		if key.surface != id {
			continue
		}
		if v.component != nil {
			v.component.Close()
		}
		delete(s.natives, key)
	}
}

// nativeItem is what an ItemNative sends Java: the class to build, empty
// for none or to keep the one it has, and the setter calls.
type nativeItem struct {
	class string
	props []driver.AndroidViewProp
}

// trackNatives makes and updates the components of the native views of surface's batch b and returns what each ItemNative in b sends Java, in order.
func (s *surface) trackNatives(surface uint32, b *driver.MountBatch) []nativeItem {
	var items []nativeItem
	s.cmu.Lock()
	defer s.cmu.Unlock()
	for i := range b.Items {
		it := &b.Items[i]
		key := nativeKey{surface, it.ID}
		switch it.Kind {
		case driver.ItemCreate, driver.ItemDelete:
			if v := s.natives[key]; v != nil {
				if v.component != nil {
					v.component.Close()
				}
				delete(s.natives, key)
			}
		case driver.ItemNative:
			items = append(items, s.configureNative(key, it.Native))
		case driver.ItemNativeCommand:
			if v := s.natives[key]; v != nil && v.component != nil {
				v.component.Command(it.Value, it.Data)
			}
		}
	}
	return items
}

// configureNative makes key's component on its first item and passes it
// the props that changed.
func (s *surface) configureNative(key nativeKey, c *driver.NativeConfig) nativeItem {
	var item nativeItem
	v := s.natives[key]
	if v == nil {
		v = &nativeView{}
		s.natives[key] = v
		factory := s.components[c.Component]
		if factory == nil && c.Component == driver.TabBarComponent {
			factory = newTabBar
		}
		if factory == nil && c.Component == driver.SwitchComponent {
			factory = newSystemSwitch
		}
		if factory == nil {
			// The host's, if Java has one of the name.
			v.host = true
			item.class = hostClass + c.Component
		} else {
			surface, id := key.surface, key.id
			v.component = factory(driver.NewNativeContext(
				func(name string, data any) {
					s.dispatchTo(surface, driver.Event{Kind: driver.EventNative, ID: id, Text: name, Data: data})
				},
				func(size driver.Size) {
					s.dispatchTo(surface, driver.Event{Kind: driver.EventNativeSize, ID: id, Size: size})
				},
			))
		}
		if v.component != nil {
			if class, ok := v.component.View().(string); ok {
				item.class = class
			} else {
				log.Printf("nsgo: native component %q: View must return a View class name on Android", c.Component)
			}
		}
	}
	if v.host {
		// The host's component takes the props as they are.
		for name, value := range c.Props {
			item.props = append(item.props, driver.AndroidViewProp{Name: name, Value: value})
		}
		sort.Slice(item.props, func(i, j int) bool { return item.props[i].Name < item.props[j].Name })
		return item
	}
	if v.component == nil {
		return item
	}
	names := make([]string, 0, len(c.Props))
	for name := range c.Props {
		names = append(names, name)
	}
	sort.Strings(names)
	mapper, _ := v.component.(driver.AndroidPropMapper)
	for _, name := range names {
		value := c.Props[name]
		v.component.SetProp(name, value)
		if mapper != nil {
			item.props = append(item.props, mapper.ViewProps(name, value)...)
		} else {
			item.props = append(item.props, driver.AndroidViewProp{Name: name, Value: value})
		}
	}
	return item
}

// nativeEvent hands a listener callback of the View of surface's native view id to its component, or emits it.
func (s *surface) nativeEvent(surface uint32, id driver.NodeID, name, data string) {
	e := s.target(surface)
	if e == nil {
		return
	}
	if name == driver.PictureInPictureEventName {
		if event, ok := readPictureInPictureEvent(data); ok {
			e.Dispatch(driver.Event{Kind: driver.EventNative, ID: id, Text: name, Data: event})
		}
		return
	}
	s.cmu.Lock()
	v := s.natives[nativeKey{surface, id}]
	if v != nil && v.host {
		// A host's component emits one value.
		s.cmu.Unlock()
		var value any
		json.Unmarshal([]byte(data), &value)
		e.Dispatch(driver.Event{Kind: driver.EventNative, ID: id, Text: name, Data: value})
		return
	}
	var args []any
	if data != "" {
		json.Unmarshal([]byte(data), &args)
	}
	var mapper driver.AndroidEventMapper
	if v != nil && v.component != nil {
		mapper, _ = v.component.(driver.AndroidEventMapper)
	}
	if mapper != nil {
		mapper.ViewEvent(name, args)
	}
	s.cmu.Unlock()
	if mapper == nil && v != nil {
		e.Dispatch(driver.Event{Kind: driver.EventNative, ID: id, Text: name, Data: args})
	}
}

// jsonValue is v as JSON, null for what JSON cannot hold.
func jsonValue(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "null"
	}
	return string(b)
}

// decodedImage is what Java gets for an image's bytes: those to decode,
// their pixels per point and the size EventLoad reports, zero for the
// decoded bitmap's own.
type decodedImage struct {
	data  []byte
	scale float32
	size  driver.Size
}

// prepareImage passes bitmaps through for Java's decoders, which decode
// one at a time, and rasterizes an SVG into a PNG, which Android cannot
// decode, in one of imagedecode's slots: covering view, the view's size
// in points, when it is known, else oversampled. An SVG reports its
// intrinsic size in whole points.
func (s *surface) prepareImage(ctx context.Context, data []byte, uri string, view driver.Size) (decodedImage, error) {
	if imagedecode.Sniff(data) != imagedecode.SVG {
		return decodedImage{data: data, scale: imageScale(uri)}, nil
	}
	done, err := imagedecode.Wait(ctx)
	if err != nil {
		return decodedImage{}, err
	}
	defer done()
	opts := imagedecode.Options{Layout: imagedecode.RGBA}
	if ratio := math.Float32frombits(s.ratio.Load()); ratio > 0 && view.Width > 0 && view.Height > 0 {
		opts.Cover = int(math.Ceil(float64(max(view.Width, view.Height) * driver.Points(ratio))))
	}
	img, err := imagedecode.Decode(data, opts)
	if err != nil {
		return decodedImage{}, err
	}
	m := &image.RGBA{Pix: img.Frames[0].Pix, Stride: 4 * img.Width, Rect: image.Rect(0, 0, img.Width, img.Height)}
	var b bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := enc.Encode(&b, m); err != nil {
		return decodedImage{}, err
	}
	scale := float32(1)
	if img.Size.Width > 0 {
		scale = float32(img.Width) / float32(img.Size.Width)
	}
	round := func(p driver.Points) driver.Points { return driver.Points(math.Round(float64(p))) }
	return decodedImage{data: b.Bytes(), scale: scale,
		size: driver.Size{Width: round(img.Size.Width), Height: round(img.Size.Height)}}, nil
}
