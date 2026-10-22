package apk

import (
	"bytes"
	"encoding/binary"
	"unicode/utf16"
)

// The resource table chunk types, from Android's ResourceTypes.h.
const (
	tableHeader     = 0x0002
	tablePackage    = 0x0200
	tableType       = 0x0201
	tableTypeSpec   = 0x0202
	appPackageID    = 0x7f
	configSize      = 64
	densityXXXHigh  = 640
	iconFile        = "res/mipmap-xxxhdpi-v4/ic_launcher.png"
	iconResource    = "@mipmap/ic_launcher"
	iconResourceID  = appPackageID<<24 | 1<<16 // package 0x7f, type 1, entry 0
	utf8PoolFlag    = 1 << 8
	resStringHeader = 28
)

// IconSize is the launcher icon's size in pixels: 48dp at xxxhdpi, the
// density the table declares it for. The system scales it for others.
const IconSize = 192

// iconResources is a resource table with one entry, the launcher icon
// @mipmap/ic_launcher, at xxxhdpi in iconFile. The manifest's
// android:icon refers to it by iconResourceID.
func iconResources(pkg string) []byte {
	var body bytes.Buffer
	body.Write(stringPoolChunk([]string{iconFile}))

	var spec bytes.Buffer
	put := func(b *bytes.Buffer, v any) { binary.Write(b, binary.LittleEndian, v) }
	put(&spec, [2]uint16{tableTypeSpec, 16})
	put(&spec, uint32(16+4))
	put(&spec, [4]uint8{1, 0, 0, 0}) // type 1, mipmap
	put(&spec, uint32(1))            // one entry
	put(&spec, uint32(0x0100))       // that varies by density

	var typ bytes.Buffer
	const typeHeader = 20 + configSize
	put(&typ, [2]uint16{tableType, typeHeader})
	put(&typ, uint32(typeHeader+4+16))
	put(&typ, [4]uint8{1, 0, 0, 0})
	put(&typ, uint32(1))            // entries
	put(&typ, uint32(typeHeader+4)) // where they start
	config := make([]byte, configSize)
	binary.LittleEndian.PutUint32(config, configSize)
	binary.LittleEndian.PutUint16(config[14:], densityXXXHigh)
	binary.LittleEndian.PutUint16(config[24:], 4) // sdkVersion, as aapt2 adds for a density
	typ.Write(config)
	put(&typ, uint32(0))               // the entry's offset
	put(&typ, [2]uint16{8, 0})         // entry header, no flags
	put(&typ, uint32(0))               // key: ic_launcher
	put(&typ, uint16(8))               // value size
	put(&typ, [2]uint8{0, typeString}) // a string,
	put(&typ, uint32(0))               // iconFile in the global pool

	types := stringPoolChunk([]string{"mipmap"})
	keys := stringPoolChunk([]string{"ic_launcher"})
	const packageHeader = 288
	var p bytes.Buffer
	put(&p, [2]uint16{tablePackage, packageHeader})
	put(&p, uint32(packageHeader+len(types)+len(keys)+spec.Len()+typ.Len()))
	put(&p, uint32(appPackageID))
	name := make([]uint16, 128)
	copy(name[:127], utf16.Encode([]rune(pkg)))
	put(&p, name)
	put(&p, uint32(packageHeader))            // type strings
	put(&p, uint32(1))                        // last public type
	put(&p, uint32(packageHeader+len(types))) // key strings
	put(&p, uint32(1))                        // last public key
	put(&p, uint32(0))                        // type ID offset
	p.Write(types)
	p.Write(keys)
	p.Write(spec.Bytes())
	p.Write(typ.Bytes())
	body.Write(p.Bytes())

	var out bytes.Buffer
	put(&out, [2]uint16{tableHeader, 12})
	put(&out, uint32(12+body.Len()))
	put(&out, uint32(1)) // packages
	out.Write(body.Bytes())
	return out.Bytes()
}

// stringPoolChunk is a UTF-8 string pool of short ASCII strings, each
// with its character and byte lengths and a terminating zero.
func stringPoolChunk(strs []string) []byte {
	var data bytes.Buffer
	offsets := make([]uint32, len(strs))
	for i, s := range strs {
		offsets[i] = uint32(data.Len())
		data.Write([]byte{byte(len(s)), byte(len(s))})
		data.WriteString(s)
		data.WriteByte(0)
	}
	for data.Len()%4 != 0 {
		data.WriteByte(0)
	}
	start := resStringHeader + 4*len(strs)
	var out bytes.Buffer
	put := func(v any) { binary.Write(&out, binary.LittleEndian, v) }
	put([2]uint16{stringPool, resStringHeader})
	put(uint32(start + data.Len()))
	put(uint32(len(strs)))
	put(uint32(0)) // styles
	put(uint32(utf8PoolFlag))
	put(uint32(start))
	put(uint32(0)) // styles start
	put(offsets)
	out.Write(data.Bytes())
	return out.Bytes()
}
