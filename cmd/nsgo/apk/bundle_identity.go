package apk

import (
	"archive/zip"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
)

// BundleIdentity is the package name and version code in a bundle's base manifest.
type BundleIdentity struct {
	Package     string
	VersionCode int64
}

// ReadBundleIdentity reads only the protobuf manifest in an Android App Bundle.
func ReadBundleIdentity(r io.ReaderAt, size int64) (BundleIdentity, error) {
	const path = "base/manifest/AndroidManifest.xml"
	bundle, err := zip.NewReader(r, size)
	if err != nil {
		return BundleIdentity{}, err
	}
	manifest, err := bundle.Open(path)
	if err != nil {
		return BundleIdentity{}, err
	}
	defer manifest.Close()
	const maxManifest = 4 << 20
	data, err := io.ReadAll(io.LimitReader(manifest, maxManifest+1))
	if err != nil {
		return BundleIdentity{}, err
	}
	if len(data) > maxManifest {
		return BundleIdentity{}, fmt.Errorf("%s exceeds %d bytes", path, maxManifest)
	}
	return bundleIdentity(data)
}

func bundleIdentity(data []byte) (BundleIdentity, error) {
	node, err := parseBundleFields(data)
	if err != nil {
		return BundleIdentity{}, err
	}
	element, err := parseBundleFields(node.bytes(1))
	if err != nil {
		return BundleIdentity{}, err
	}
	if string(element.bytes(3)) != "manifest" {
		return BundleIdentity{}, errors.New("bundle's base XML has no manifest element")
	}
	var identity BundleIdentity
	for _, field := range element {
		if field.tag != 4<<3|2 { // XmlElement.attribute
			continue
		}
		attr, err := parseBundleFields(field.data)
		if err != nil {
			return BundleIdentity{}, err
		}
		namespace, name := string(attr.bytes(1)), string(attr.bytes(2))
		switch {
		case namespace == "" && name == "package":
			identity.Package = string(attr.bytes(3))
		case namespace == androidNS && name == "versionCode":
			identity.VersionCode, err = bundleVersionCode(attr)
			if err != nil {
				return BundleIdentity{}, err
			}
		}
	}
	if err := validPackage(identity.Package); err != nil {
		return BundleIdentity{}, err
	}
	if identity.VersionCode <= 0 || identity.VersionCode > math.MaxInt32 {
		return BundleIdentity{}, fmt.Errorf("bundle's base manifest has an invalid Android versionCode: %d", identity.VersionCode)
	}
	return identity, nil
}

func bundleVersionCode(attr bundleFields) (int64, error) {
	if compiled := attr.bytes(6); compiled != nil {
		// XmlAttribute.compiled_item → Item.prim → Primitive.int_decimal_value.
		item, err := parseBundleFields(compiled)
		if err != nil {
			return 0, err
		}
		primitive, err := parseBundleFields(item.bytes(7))
		if err != nil {
			return 0, err
		}
		for _, field := range primitive {
			if field.tag == 6<<3 || field.tag == 7<<3 {
				return int64(field.number), nil
			}
		}
		return 0, errors.New("bundle's compiled Android versionCode is not an integer")
	}
	code, err := strconv.ParseInt(string(attr.bytes(3)), 10, 32)
	if err != nil {
		return 0, fmt.Errorf("bundle's Android versionCode: %w", err)
	}
	return code, nil
}

type bundleField struct {
	tag    uint64
	data   []byte
	number uint64
}

type bundleFields []bundleField

func (fields bundleFields) bytes(number uint64) []byte {
	for i := len(fields) - 1; i >= 0; i-- {
		if fields[i].tag == number<<3|2 {
			return fields[i].data
		}
	}
	return nil
}

// parseBundleFields reads one message from aapt2's protobuf XML format.
func parseBundleFields(data []byte) (bundleFields, error) {
	var fields bundleFields
	invalid := errors.New("malformed protobuf in the bundle's base manifest")
	for len(data) > 0 {
		tag, n := binary.Uvarint(data)
		if n <= 0 || tag>>3 == 0 {
			return nil, invalid
		}
		data = data[n:]
		field := bundleField{tag: tag}
		var size uint64
		switch tag & 7 {
		case 0, 2:
			value, n := binary.Uvarint(data)
			if n <= 0 {
				return nil, invalid
			}
			data = data[n:]
			if tag&7 == 2 {
				size = value
			} else {
				field.number = value
			}
		case 1:
			size = 8
		case 5:
			size = 4
		default:
			return nil, invalid
		}
		if size > uint64(len(data)) {
			return nil, invalid
		}
		field.data, data = data[:size], data[size:]
		fields = append(fields, field)
	}
	return fields, nil
}
