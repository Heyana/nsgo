package apk

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/des"
	"crypto/pbkdf2"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"math/big"
	"strings"
	"unicode/utf16"
)

// ReadKeystore reads the signing key alias from a keystore as keytool and
// Android Studio write them: PKCS#12 (keytool's default since JDK 9) or
// the older JKS. keyPassword, when empty, is storePassword; alias, when
// empty, picks the only private key.
func ReadKeystore(data []byte, storePassword, alias, keyPassword string) (*Key, error) {
	if keyPassword == "" {
		keyPassword = storePassword
	}
	var keys []keystoreEntry
	var err error
	if len(data) >= 4 && binary.BigEndian.Uint32(data) == 0xfeedfeed {
		keys, err = readJKS(data, keyPassword)
	} else {
		keys, err = readPKCS12(data, storePassword, keyPassword)
	}
	if err != nil {
		return nil, err
	}
	var found []keystoreEntry
	var names []string
	for _, k := range keys {
		names = append(names, k.alias)
		if alias == "" || strings.EqualFold(k.alias, alias) {
			found = append(found, k)
		}
	}
	switch {
	case len(found) == 0 && alias != "":
		return nil, fmt.Errorf("the keystore has no key %q; it has %s", alias, strings.Join(names, ", "))
	case len(found) == 0:
		return nil, errors.New("the keystore holds no private key")
	case len(found) > 1:
		return nil, fmt.Errorf("the keystore holds several keys (%s): name one", strings.Join(names, ", "))
	}
	k := found[0]
	private, ok := k.key.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("the key %q is %T; NSGo signs APKs with RSA keys", k.alias, k.key)
	}
	if k.cert == nil {
		return nil, fmt.Errorf("the key %q has no certificate", k.alias)
	}
	return &Key{Private: private, Cert: k.cert}, nil
}

type keystoreEntry struct {
	alias string
	key   any
	cert  *x509.Certificate
}

// readJKS reads a Java KeyStore's private keys, which Sun's key protector
// hides with a SHA-1 stream of the password.
func readJKS(data []byte, password string) ([]keystoreEntry, error) {
	r := bytes.NewReader(data)
	var head struct{ Magic, Version, Count uint32 }
	if err := binary.Read(r, binary.BigEndian, &head); err != nil {
		return nil, err
	}
	if head.Version != 1 && head.Version != 2 {
		return nil, fmt.Errorf("JKS version %d", head.Version)
	}
	str := func() (string, error) {
		var n uint16
		if err := binary.Read(r, binary.BigEndian, &n); err != nil {
			return "", err
		}
		b := make([]byte, n)
		_, err := r.Read(b)
		return string(b), err
	}
	blob := func() ([]byte, error) {
		var n uint32
		if err := binary.Read(r, binary.BigEndian, &n); err != nil {
			return nil, err
		}
		if int(n) > r.Len() {
			return nil, errors.New("JKS: truncated")
		}
		b := make([]byte, n)
		_, err := r.Read(b)
		return b, err
	}
	pass := make([]byte, 0, 2*len(password))
	for _, c := range utf16.Encode([]rune(password)) {
		pass = append(pass, byte(c>>8), byte(c))
	}
	var out []keystoreEntry
	for i := uint32(0); i < head.Count; i++ {
		var tag uint32
		if err := binary.Read(r, binary.BigEndian, &tag); err != nil {
			return nil, err
		}
		alias, err := str()
		if err != nil {
			return nil, err
		}
		r.Seek(8, 1) // the entry's date
		switch tag {
		case 1:
			protected, err := blob()
			if err != nil {
				return nil, err
			}
			var count uint32
			if err := binary.Read(r, binary.BigEndian, &count); err != nil {
				return nil, err
			}
			e := keystoreEntry{alias: alias}
			for c := uint32(0); c < count; c++ {
				if head.Version == 2 {
					if _, err := str(); err != nil {
						return nil, err
					}
				}
				der, err := blob()
				if err != nil {
					return nil, err
				}
				if c == 0 {
					if e.cert, err = x509.ParseCertificate(der); err != nil {
						return nil, err
					}
				}
			}
			var info struct {
				Algo pkix.AlgorithmIdentifier
				Data []byte
			}
			if _, err := asn1.Unmarshal(protected, &info); err != nil {
				return nil, fmt.Errorf("JKS key %q: %w", alias, err)
			}
			plain, err := jksUnprotect(info.Data, pass)
			if err != nil {
				return nil, fmt.Errorf("JKS key %q: %w", alias, err)
			}
			if e.key, err = x509.ParsePKCS8PrivateKey(plain); err != nil {
				return nil, fmt.Errorf("JKS key %q: %w", alias, err)
			}
			out = append(out, e)
		case 2:
			if head.Version == 2 {
				if _, err := str(); err != nil {
					return nil, err
				}
			}
			if _, err := blob(); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("JKS entry kind %d (a JCEKS secret key?)", tag)
		}
	}
	return out, nil
}

// jksUnprotect undoes Sun's JKS key protector: a 20-byte salt, the key
// XORed with SHA-1(password || last digest) blocks, and a check digest.
func jksUnprotect(data, pass []byte) ([]byte, error) {
	if len(data) < 40 {
		return nil, errors.New("short protected key")
	}
	salt, enc, check := data[:20], data[20:len(data)-20], data[len(data)-20:]
	plain := make([]byte, len(enc))
	digest := salt
	for i := 0; i < len(enc); i += 20 {
		h := sha1.New()
		h.Write(pass)
		h.Write(digest)
		digest = h.Sum(nil)
		for j := 0; j < 20 && i+j < len(enc); j++ {
			plain[i+j] = enc[i+j] ^ digest[j]
		}
	}
	h := sha1.New()
	h.Write(pass)
	h.Write(plain)
	if !bytes.Equal(h.Sum(nil), check) {
		return nil, errors.New("wrong password")
	}
	return plain, nil
}

var (
	oidData          = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 1}
	oidEncryptedData = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 6}
	oidKeyBag        = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 12, 10, 1, 1}
	oidShroudedKey   = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 12, 10, 1, 2}
	oidCertBag       = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 12, 10, 1, 3}
	oidFriendlyName  = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 20}
	oidLocalKeyID    = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 21}
	oidPBES2         = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 5, 13}
	oidPBKDF2        = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 5, 12}
	oidSHA3DES       = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 12, 1, 3}
	oidSHARC2_40     = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 12, 1, 6}
	oidHMACSHA1      = asn1.ObjectIdentifier{1, 2, 840, 113549, 2, 7}
	oidHMACSHA256    = asn1.ObjectIdentifier{1, 2, 840, 113549, 2, 9}
	oidHMACSHA512    = asn1.ObjectIdentifier{1, 2, 840, 113549, 2, 11}
	oidAES128CBC     = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 1, 2}
	oidAES192CBC     = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 1, 22}
	oidAES256CBC     = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 1, 42}
	oidDESEDE3CBC    = asn1.ObjectIdentifier{1, 2, 840, 113549, 3, 7}
)

type contentInfo struct {
	Type    asn1.ObjectIdentifier
	Content asn1.RawValue `asn1:"tag:0,explicit,optional"`
}

type safeBag struct {
	ID         asn1.ObjectIdentifier
	Value      asn1.RawValue `asn1:"tag:0,explicit"`
	Attributes []struct {
		ID     asn1.ObjectIdentifier
		Values asn1.RawValue `asn1:"set"`
	} `asn1:"set,optional"`
}

// readPKCS12 reads a PKCS#12 keystore's private keys with their
// certificates, matched by local key ID.
func readPKCS12(data []byte, storePassword, keyPassword string) ([]keystoreEntry, error) {
	var pfx struct {
		Version  int
		AuthSafe contentInfo
		MacData  asn1.RawValue `asn1:"optional"`
	}
	if _, err := asn1.Unmarshal(data, &pfx); err != nil {
		return nil, fmt.Errorf("not a PKCS#12 or JKS keystore: %w", err)
	}
	if !pfx.AuthSafe.Type.Equal(oidData) {
		return nil, errors.New("PKCS#12: the keystore is not password protected")
	}
	var authSafe []byte
	if _, err := asn1.Unmarshal(pfx.AuthSafe.Content.Bytes, &authSafe); err != nil {
		return nil, err
	}
	var infos []contentInfo
	if _, err := asn1.Unmarshal(authSafe, &infos); err != nil {
		return nil, err
	}
	type bagged struct {
		alias, id string
		key       any
		cert      *x509.Certificate
	}
	var keys, certs []bagged
	for _, info := range infos {
		var contents []byte
		switch {
		case info.Type.Equal(oidData):
			if _, err := asn1.Unmarshal(info.Content.Bytes, &contents); err != nil {
				return nil, err
			}
		case info.Type.Equal(oidEncryptedData):
			var enc struct {
				Version int
				Info    struct {
					Type      asn1.ObjectIdentifier
					Algorithm pkix.AlgorithmIdentifier
					Content   []byte `asn1:"tag:0,optional"`
				}
			}
			if _, err := asn1.Unmarshal(info.Content.Bytes, &enc); err != nil {
				return nil, err
			}
			var err error
			if contents, err = pbeDecrypt(enc.Info.Algorithm, enc.Info.Content, storePassword); err != nil {
				return nil, fmt.Errorf("PKCS#12: %w", err)
			}
		default:
			continue
		}
		var bags []safeBag
		if _, err := asn1.Unmarshal(contents, &bags); err != nil {
			return nil, err
		}
		for _, bag := range bags {
			var b bagged
			for _, a := range bag.Attributes {
				switch {
				case a.ID.Equal(oidFriendlyName):
					var name asn1.RawValue
					if _, err := asn1.Unmarshal(a.Values.Bytes, &name); err == nil {
						b.alias = bmpString(name.Bytes)
					}
				case a.ID.Equal(oidLocalKeyID):
					var id []byte
					if _, err := asn1.Unmarshal(a.Values.Bytes, &id); err == nil {
						b.id = string(id)
					}
				}
			}
			switch {
			case bag.ID.Equal(oidKeyBag), bag.ID.Equal(oidShroudedKey):
				der := bag.Value.Bytes
				if bag.ID.Equal(oidShroudedKey) {
					var shrouded struct {
						Algorithm pkix.AlgorithmIdentifier
						Data      []byte
					}
					if _, err := asn1.Unmarshal(der, &shrouded); err != nil {
						return nil, err
					}
					var err error
					if der, err = pbeDecrypt(shrouded.Algorithm, shrouded.Data, keyPassword); err != nil {
						return nil, fmt.Errorf("PKCS#12 key: %w", err)
					}
				}
				key, err := x509.ParsePKCS8PrivateKey(der)
				if err != nil {
					return nil, err
				}
				b.key = key
				keys = append(keys, b)
			case bag.ID.Equal(oidCertBag):
				var cb struct {
					ID   asn1.ObjectIdentifier
					Cert []byte `asn1:"tag:0,explicit"`
				}
				if _, err := asn1.Unmarshal(bag.Value.Bytes, &cb); err != nil {
					return nil, err
				}
				cert, err := x509.ParseCertificate(cb.Cert)
				if err != nil {
					return nil, err
				}
				b.cert = cert
				certs = append(certs, b)
			}
		}
	}
	var out []keystoreEntry
	for _, k := range keys {
		e := keystoreEntry{alias: k.alias, key: k.key}
		for _, c := range certs {
			if k.id != "" && c.id == k.id || e.cert == nil && len(keys) == 1 {
				e.cert = c.cert
			}
		}
		out = append(out, e)
	}
	return out, nil
}

func bmpString(b []byte) string {
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = uint16(b[2*i])<<8 | uint16(b[2*i+1])
	}
	return string(utf16.Decode(u))
}

// pbeDecrypt decrypts data with password as algo says: PBES2 with PBKDF2
// and AES or 3DES (keytool's), or PKCS#12's own SHA-1 schemes with 3DES
// or 40-bit RC2 (older keystores').
func pbeDecrypt(algo pkix.AlgorithmIdentifier, data []byte, password string) ([]byte, error) {
	var block cipher.Block
	var iv []byte
	switch {
	case algo.Algorithm.Equal(oidPBES2):
		var params struct {
			KDF    pkix.AlgorithmIdentifier
			Scheme pkix.AlgorithmIdentifier
		}
		if _, err := asn1.Unmarshal(algo.Parameters.FullBytes, &params); err != nil {
			return nil, err
		}
		if !params.KDF.Algorithm.Equal(oidPBKDF2) {
			return nil, fmt.Errorf("key derivation %v", params.KDF.Algorithm)
		}
		var kdf struct {
			Salt       []byte
			Iterations int
			KeyLength  int                      `asn1:"optional"`
			PRF        pkix.AlgorithmIdentifier `asn1:"optional"`
		}
		if _, err := asn1.Unmarshal(params.KDF.Parameters.FullBytes, &kdf); err != nil {
			return nil, err
		}
		prf := func() hash.Hash { return sha1.New() }
		switch {
		case kdf.PRF.Algorithm == nil, kdf.PRF.Algorithm.Equal(oidHMACSHA1):
		case kdf.PRF.Algorithm.Equal(oidHMACSHA256):
			prf = sha256.New
		case kdf.PRF.Algorithm.Equal(oidHMACSHA512):
			prf = sha512.New
		default:
			return nil, fmt.Errorf("PBKDF2 with %v", kdf.PRF.Algorithm)
		}
		if _, err := asn1.Unmarshal(params.Scheme.Parameters.FullBytes, &iv); err != nil {
			return nil, err
		}
		size := map[string]int{oidAES128CBC.String(): 16, oidAES192CBC.String(): 24, oidAES256CBC.String(): 32,
			oidDESEDE3CBC.String(): 24}[params.Scheme.Algorithm.String()]
		if size == 0 {
			return nil, fmt.Errorf("cipher %v", params.Scheme.Algorithm)
		}
		key, err := pbkdf2.Key(prf, password, kdf.Salt, kdf.Iterations, size)
		if err != nil {
			return nil, err
		}
		if size == 24 && params.Scheme.Algorithm.Equal(oidDESEDE3CBC) {
			block, err = des.NewTripleDESCipher(key)
		} else {
			block, err = aes.NewCipher(key)
		}
		if err != nil {
			return nil, err
		}
	case algo.Algorithm.Equal(oidSHA3DES), algo.Algorithm.Equal(oidSHARC2_40):
		var params struct {
			Salt       []byte
			Iterations int
		}
		if _, err := asn1.Unmarshal(algo.Parameters.FullBytes, &params); err != nil {
			return nil, err
		}
		pass := bmpPassword(password)
		if algo.Algorithm.Equal(oidSHA3DES) {
			key := pkcs12KDF(pass, params.Salt, params.Iterations, 1, 24)
			iv = pkcs12KDF(pass, params.Salt, params.Iterations, 2, 8)
			var err error
			if block, err = des.NewTripleDESCipher(key); err != nil {
				return nil, err
			}
		} else {
			key := pkcs12KDF(pass, params.Salt, params.Iterations, 1, 5)
			iv = pkcs12KDF(pass, params.Salt, params.Iterations, 2, 8)
			block = newRC2(key, 40)
		}
	default:
		return nil, fmt.Errorf("encryption %v", algo.Algorithm)
	}
	if len(data)%block.BlockSize() != 0 || len(data) == 0 {
		return nil, errors.New("bad encrypted data")
	}
	plain := make([]byte, len(data))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plain, data)
	n := int(plain[len(plain)-1])
	if n == 0 || n > block.BlockSize() || n > len(plain) || !bytes.Equal(plain[len(plain)-n:], bytes.Repeat([]byte{byte(n)}, n)) {
		return nil, errors.New("wrong password")
	}
	return plain[:len(plain)-n], nil
}

// bmpPassword is a password as PKCS#12's own schemes take it: UTF-16BE
// ending in a NUL.
func bmpPassword(s string) []byte {
	out := []byte{}
	for _, c := range utf16.Encode([]rune(s)) {
		out = append(out, byte(c>>8), byte(c))
	}
	return append(out, 0, 0)
}

// pkcs12KDF derives size bytes for purpose id (1 key, 2 IV, 3 MAC) from
// a password with SHA-1, as RFC 7292 appendix B does.
func pkcs12KDF(pass, salt []byte, iterations int, id byte, size int) []byte {
	const v = 64
	fill := func(b []byte) []byte {
		if len(b) == 0 {
			return nil
		}
		out := make([]byte, v*((len(b)+v-1)/v))
		for i := range out {
			out[i] = b[i%len(b)]
		}
		return out
	}
	d := bytes.Repeat([]byte{id}, v)
	i := append(fill(salt), fill(pass)...)
	var out []byte
	for len(out) < size {
		h := sha1.New()
		h.Write(d)
		h.Write(i)
		a := h.Sum(nil)
		for r := 1; r < iterations; r++ {
			s := sha1.Sum(a)
			a = s[:]
		}
		out = append(out, a...)
		b := fill(a)[:v]
		bn := new(big.Int).SetBytes(b)
		one := big.NewInt(1)
		for j := 0; j < len(i); j += v {
			ij := new(big.Int).SetBytes(i[j : j+v])
			ij.Add(ij, bn).Add(ij, one)
			sum := ij.Bytes()
			if len(sum) > v {
				sum = sum[len(sum)-v:]
			}
			copy(i[j:j+v], make([]byte, v))
			copy(i[j+v-len(sum):j+v], sum)
		}
	}
	return out[:size]
}
