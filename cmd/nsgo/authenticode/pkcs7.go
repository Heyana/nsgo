package authenticode

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"errors"
	"time"
)

// Authenticode's structures, as CMS encodes them with Microsoft's own
// object identifiers:
//
//	ContentInfo ::= SEQUENCE {
//	  contentType            1.3.6.1.4.1.311.2.1.4, SPC_INDIRECT_DATA_OBJID
//	  content            [0] EXPLICIT SpcIndirectDataContent }
//	SpcIndirectDataContent ::= SEQUENCE {
//	  data                SpcAttributeTypeAndOptionalValue,
//	  messageDigest       DigestInfo }
//	SpcAttributeTypeAndOptionalValue ::= SEQUENCE {
//	  type                   1.3.6.1.4.1.311.2.1.15, SPC_PE_IMAGE_DATA_OBJID
//	  value                   SpcPeImageData, untagged }
//	SpcPeImageData ::= SEQUENCE {
//	  flags               SpcPeImageFlags, a BIT STRING, and
//	  file                SpcLink, a name readers ignore: the format's own
//	                      "<<<Obsolete>>>" in a BMPSTRING under [0] and [2]. }
var (
	oidSpcIndirectData  = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 311, 2, 1, 4}
	oidSpcPeImageData   = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 311, 2, 1, 15}
	oidSpcStatementType = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 311, 2, 1, 11}
	oidSpcIndividual    = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 311, 2, 1, 21}
	oidSHA256           = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 1}
	oidRSA              = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 1}
	oidSignedData       = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 2}
	oidContentType      = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 3}
	oidMessageDigest    = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 4}
	oidSigningTime      = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 5}
	oidTimestampToken   = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 16, 2, 14}
)

// Key is the identity a Windows file is signed with.
type Key struct {
	Private *rsa.PrivateKey
	Cert    *x509.Certificate
	// Chain holds the certificates above Cert that the signature carries,
	// so that Windows need not fetch them.
	Chain []*x509.Certificate
}

// Stamper asks a timestamp server to sign the digest of a signature's
// value. RFC 3161's TimeStampReq asks, and the token of its TimeStampResp
// answer is the signature's unsigned attribute.
type Stamper interface {
	Token(digest []byte) ([]byte, error)
}

// spcPeImageFlags is SpcPeImageFlags with includeResources, its only bit,
// set, which the format's writers always set.
var spcPeImageFlags = []byte{0x03, 0x02, 0x07, 0x80}

// indirectData is the content a signature states: the digest of a PE
// image, and the format's obsolete name for it.
func indirectData(sum [sha256.Size]byte) []byte {
	name := explicit(2, implicitPrimitive(0, utf16BE("<<<Obsolete>>>")))
	image := sequence(spcPeImageFlags, explicit(0, name))
	data := sequence(oid(oidSpcPeImageData), image)
	digestInfo := sequence(algorithmIdentifier(oidSHA256), octetString(sum[:]))
	return sequence(data, digestInfo)
}

// contentValue is the octets a signature's message digest covers:
// SpcIndirectDataContent's contents, without the SEQUENCE header that
// wraps them, as Authenticode's readers check it.
func contentValue(content []byte) []byte {
	return content[1+lengthSize(content[1:]):]
}

// lengthSize is how many bytes a DER length at the start of b takes.
func lengthSize(b []byte) int {
	if len(b) == 0 {
		return 0
	}
	if b[0]&0x80 == 0 {
		return 1
	}
	return 1 + int(b[0]&0x7f)
}

// signedData is a PKCS #7 SignedData over sum, signed by key, carrying its
// certificate chain and, with a stamper, a timestamp token.
func signedData(sum [sha256.Size]byte, key *Key, stamp Stamper) ([]byte, error) {
	return signedContent(indirectData(sum), key, stamp)
}

// signedContent signs a format's SpcIndirectDataContent with the same CMS attributes and timestamp flow.
func signedContent(content []byte, key *Key, stamp Stamper) ([]byte, error) {
	if key == nil || key.Private == nil || key.Cert == nil {
		return nil, errors.New("a Windows signature needs a certificate and its key")
	}
	public, ok := key.Cert.PublicKey.(*rsa.PublicKey)
	if !ok || !public.Equal(&key.Private.PublicKey) {
		return nil, errors.New("the certificate's key is not the one that signs")
	}
	// The attributes the signature covers, in the order a SET OF holds
	// them, since the signature covers their encoding.
	attrs := setAttributes(signatureAttributes(content, time.Now()))
	hash := sha256.Sum256(der(0x31, attrs...))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key.Private, crypto.SHA256, hash[:])
	if err != nil {
		return nil, err
	}
	issuer, err := asn1.Marshal(asn1.RawValue{FullBytes: key.Cert.RawIssuer})
	if err != nil {
		return nil, err
	}
	signer := [][]byte{
		smallInt(1),
		sequence(issuer, integer(key.Cert.SerialNumber)),
		algorithmIdentifier(oidSHA256),
		// [0] IMPLICIT, the attributes the signature covers: their own
		// contents under that tag.
		implicit(0, attrs...),
		algorithmIdentifier(oidRSA),
		octetString(signature),
	}
	if stamp != nil {
		digest := sha256.Sum256(signature)
		token, err := stamp.Token(digest[:])
		if err != nil {
			return nil, err
		}
		// [1] IMPLICIT, the signer's unsigned attributes: the timestamp
		// token's attribute, whose value set holds the token's
		// ContentInfo.
		signer = append(signer, implicit(1, sequence(oid(oidTimestampToken), set(token))))
	}
	var certificates []byte
	for _, c := range append([]*x509.Certificate{key.Cert}, key.Chain...) {
		certificates = append(certificates, c.Raw...)
	}
	sd := sequence(
		smallInt(1),
		set(algorithmIdentifier(oidSHA256)),
		sequence(oid(oidSpcIndirectData), explicit(0, content)),
		implicit(0, certificates),
		set(sequence(signer...)),
	)
	return sequence(oid(oidSignedData), explicit(0, sd)), nil
}

// signatureAttributes are the attributes a signature covers: the content's
// type, the time of signing, that the signer signs as an individual, and
// the digest of the content's value.
func signatureAttributes(content []byte, now time.Time) [][]byte {
	attribute := func(id asn1.ObjectIdentifier, value []byte) []byte {
		return sequence(oid(id), set(value))
	}
	sum := sha256.Sum256(contentValue(content))
	return [][]byte{
		attribute(oidContentType, oid(oidSpcIndirectData)),
		attribute(oidSigningTime, utcTime(now)),
		attribute(oidSpcStatementType, sequence(oid(oidSpcIndividual))),
		attribute(oidMessageDigest, octetString(sum[:])),
	}
}
