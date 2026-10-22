package apk

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"time"
)

// Key is the identity an APK is signed with.
type Key struct {
	Private *rsa.PrivateKey
	Cert    *x509.Certificate
}

// NewKey makes a self-signed signing identity, as a debug keystore holds.
func NewKey() (*Key, error) {
	private, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "Android Debug", Organization: []string{"Android"}, Country: []string{"US"}},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.AddDate(100, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &private.PublicKey, private)
	if err != nil {
		return nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	return &Key{Private: private, Cert: cert}, nil
}

// The APK Signature Scheme v2 records, from
// source.android.com/docs/security/features/apksigning/v2.
const (
	signingBlockID = 0x7109871a
	rsaPKCS1SHA256 = 0x0103
	blockMagic     = "APK Sig Block 42"
	// Content is hashed in 1 MiB chunks, one digest per chunk.
	chunkSize = 1 << 20
)

// sign inserts an APK Signing Block holding a v2 signature before the
// central directory. The digests cover the file as if the end of
// directory record named the block's start; the record written for other
// ZIP readers names the directory's real position instead.
func sign(zip []byte, key *Key) ([]byte, error) {
	eocd, err := findEndOfDirectory(zip)
	if err != nil {
		return nil, err
	}
	cdOffset := int(binary.LittleEndian.Uint32(zip[eocd+16:]))
	cdSize := int(binary.LittleEndian.Uint32(zip[eocd+12:]))
	if cdOffset+cdSize != eocd {
		return nil, fmt.Errorf("sign: central directory %d+%d does not end at the end of directory record %d", cdOffset, cdSize, eocd)
	}

	digest := contentDigest(zip[:cdOffset], zip[cdOffset:eocd], zip[eocd:])
	signedData := join(
		// Digests: a sequence of pairs of algorithm and length-prefixed
		// digest.
		lenBytes(lenBytes(join(uint32Bytes(rsaPKCS1SHA256), lenBytes(digest)))),
		// Certificates: a sequence of length-prefixed certificates.
		lenBytes(lenBytes(key.Cert.Raw)),
		// Additional attributes: none.
		lenBytes(nil),
	)
	hash := sha256.Sum256(signedData)
	signature, err := rsa.SignPKCS1v15(rand.Reader, key.Private, crypto.SHA256, hash[:])
	if err != nil {
		return nil, err
	}
	public, err := x509.MarshalPKIXPublicKey(&key.Private.PublicKey)
	if err != nil {
		return nil, err
	}
	signer := join(
		lenBytes(signedData),
		// The signatures, one sequence of algorithm and signature.
		lenBytes(lenBytes(join(uint32Bytes(rsaPKCS1SHA256), lenBytes(signature)))),
		lenBytes(public),
	)
	// The v2 block is a length-prefixed sequence of length-prefixed signers.
	value := lenBytes(lenBytes(signer))

	pair := join(uint64Bytes(uint64(4+len(value))), uint32Bytes(signingBlockID), value)
	size := uint64(len(pair) + 8 + len(blockMagic))
	block := join(uint64Bytes(size), pair, uint64Bytes(size), []byte(blockMagic))

	out := make([]byte, 0, len(zip)+len(block))
	out = append(out, zip[:cdOffset]...)
	out = append(out, block...)
	out = append(out, zip[cdOffset:]...)
	// The digests cover the end of directory record with the directory's
	// offset naming the block; the file itself names the directory's real
	// position, so other ZIP readers find it.
	eocd += len(block)
	binary.LittleEndian.PutUint32(out[eocd+16:], uint32(cdOffset+len(block)))
	return out, nil
}

// contentDigest hashes each 1 MiB chunk of the sources, then the count and
// the chunk digests together, as v2 signatures specify. A chunk never
// spans two sources; the segments are the archive before the signing
// block, the central directory and the end of directory record.
func contentDigest(sources ...[]byte) []byte {
	chunks := 0
	for _, source := range sources {
		chunks += (len(source) + chunkSize - 1) / chunkSize
	}
	combined := append([]byte{0x5a}, uint32Bytes(uint32(chunks))...)
	for _, source := range sources {
		for len(source) > 0 {
			n := min(len(source), chunkSize)
			chunk := sha256.New()
			chunk.Write([]byte{0xa5})
			chunk.Write(uint32Bytes(uint32(n)))
			chunk.Write(source[:n])
			combined = append(combined, chunk.Sum(nil)...)
			source = source[n:]
		}
	}
	digest := sha256.Sum256(combined)
	return digest[:]
}

// lenBytes prefixes b with its length, the coding v2 uses throughout.
func lenBytes(b []byte) []byte {
	return join(uint32Bytes(uint32(len(b))), b)
}

func join(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func uint32Bytes(v uint32) []byte {
	return binary.LittleEndian.AppendUint32(nil, v)
}

func uint64Bytes(v uint64) []byte {
	return binary.LittleEndian.AppendUint64(nil, v)
}

// findEndOfDirectory returns the offset of the ZIP end of directory record.
func findEndOfDirectory(zip []byte) (int, error) {
	const record = 22
	for i := len(zip) - record; i >= 0 && i >= len(zip)-record-0xffff; i-- {
		if binary.LittleEndian.Uint32(zip[i:]) != endOfDirectory {
			continue
		}
		if int(binary.LittleEndian.Uint16(zip[i+20:])) == len(zip)-i-record {
			return i, nil
		}
	}
	return 0, errors.New("sign: no ZIP end of directory record")
}
