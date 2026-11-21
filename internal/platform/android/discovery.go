//go:build android

package android

import "github.com/nsgo-dev/nsgo/internal/driver"

func (s *surface) discovery(from, count uint32, req *driver.DiscoveryRequest) {
	if req == nil {
		return
	}
	var txt []byte
	var err error
	if req.Op == driver.DiscoveryAdvertise {
		txt, err = driver.DiscoveryTXT(req.Service.TXT)
	}
	if err == nil {
		err = s.sendExtended(extDiscovery, func(w *writer) {
			w.u32(from)
			w.u32(count)
			w.byte(byte(req.Op))
			switch req.Op {
			case driver.DiscoveryAdvertise:
				w.str(req.Service.Type)
				w.str(req.Service.Name)
				w.u32(uint32(req.Service.Port))
				w.u32(uint32(len(txt)))
				w.buf = append(w.buf, txt...)
			case driver.DiscoveryBrowse:
				w.str(req.Service.Type)
			}
		})
	}
	if err != nil && req.Op != driver.DiscoveryStop {
		go s.dispatchTo(from, driver.Event{Kind: driver.EventDiscovery, Count: count, Data: &driver.DiscoveryReply{Error: err.Error()}})
	}
}

func (s *surface) discoveryReply(r *reader) {
	from, count, failure := r.u32(), r.u32(), r.str()
	reply := driver.DiscoveryReply{Error: failure}
	if failure == "" {
		reply.Removed = r.byte() != 0
		reply.Service = driver.LocalService{
			Name: r.str(), Type: r.str(), Host: r.str(), Port: int(r.u32()), TXT: driver.ParseDiscoveryTXT(r.bytes()),
		}
	}
	s.dispatchTo(from, driver.Event{Kind: driver.EventDiscovery, Count: count, Data: &reply})
}
