// Package x509util implements utilities to build X.509 certificates based on
// JSON templates.
package x509util

import (
	"crypto"
	"crypto/rand"
	"crypto/x509"
	"encoding/asn1"
	"encoding/json"

	"github.com/pkg/errors"
)

// Certificate is the JSON representation of a X.509 certificate. It is used to
// build a certificate from a template.
type Certificate struct {
	Version               int                      `json:"version"`
	Subject               Subject                  `json:"subject"`
	Issuer                Issuer                   `json:"issuer"`
	SerialNumber          SerialNumber             `json:"serialNumber"`
	DNSNames              MultiString              `json:"dnsNames"`
	EmailAddresses        MultiString              `json:"emailAddresses"`
	IPAddresses           MultiIP                  `json:"ipAddresses"`
	URIs                  MultiURL                 `json:"uris"`
	SANs                  []SubjectAlternativeName `json:"sans"`
	Extensions            []Extension              `json:"extensions"`
	KeyUsage              KeyUsage                 `json:"keyUsage"`
	ExtKeyUsage           ExtKeyUsage              `json:"extKeyUsage"`
	UnknownExtKeyUsage    UnknownExtKeyUsage       `json:"unknownExtKeyUsage"`
	SubjectKeyID          SubjectKeyID             `json:"subjectKeyId"`
	AuthorityKeyID        AuthorityKeyID           `json:"authorityKeyId"`
	OCSPServer            OCSPServer               `json:"ocspServer"`
	IssuingCertificateURL IssuingCertificateURL    `json:"issuingCertificateURL"`
	CRLDistributionPoints CRLDistributionPoints    `json:"crlDistributionPoints"`
	PolicyIdentifiers     PolicyIdentifiers        `json:"policyIdentifiers"`
	BasicConstraints      *BasicConstraints        `json:"basicConstraints"`
	NameConstraints       *NameConstraints         `json:"nameConstraints"`
	SignatureAlgorithm    SignatureAlgorithm       `json:"signatureAlgorithm"`
	PublicKeyAlgorithm    x509.PublicKeyAlgorithm  `json:"-"`
	PublicKey             interface{}              `json:"-"`
}

// NewCertificate creates a new Certificate from an x509.Certificate request and
// some template options.
func NewCertificate(cr *x509.CertificateRequest, opts ...Option) (*Certificate, error) {
	if err := cr.CheckSignature(); err != nil {
		return nil, errors.Wrap(err, "error validating certificate request")
	}

	o, err := new(Options).apply(cr, opts)
	if err != nil {
		return nil, err
	}

	// If no template use only the certificate request with the default leaf key
	// usages.
	if o.CertBuffer == nil {
		return newCertificateRequest(cr).GetLeafCertificate(), nil
	}

	// With templates
	var cert Certificate
	if err := json.NewDecoder(o.CertBuffer).Decode(&cert); err != nil {
		return nil, errors.Wrap(err, "error unmarshaling certificate")
	}

	// Complete with certificate request
	cert.PublicKey = cr.PublicKey
	cert.PublicKeyAlgorithm = cr.PublicKeyAlgorithm

	return &cert, nil
}

// GetCertificate returns the x509.Certificate representation of the
// certificate.
func (c *Certificate) GetCertificate() *x509.Certificate {
	cert := new(x509.Certificate)
	// Unparsed data
	cert.PublicKey = c.PublicKey
	cert.PublicKeyAlgorithm = c.PublicKeyAlgorithm

	// Subject.
	c.Subject.Set(cert)

	if c.hasExtendedSANs() && !c.hasExtension(oidExtensionSubjectAltName) {
		subjectAltNameExtension, err := createSubjectAltNameExtension(c, subjectIsEmpty(cert.Subject))
		if err != nil {
			panic(err)
		}
		subjectAltNameExtension.Set(cert)
	} else {
		// When we have no extended SANs, use the golang x509 lib to create the extension instead
		cert.DNSNames = c.DNSNames
		cert.EmailAddresses = c.EmailAddresses
		cert.IPAddresses = c.IPAddresses
		cert.URIs = c.URIs

		// SANs slice.
		for _, san := range c.SANs {
			san.Set(cert)
		}
	}

	// Defined extensions.
	c.KeyUsage.Set(cert)
	c.ExtKeyUsage.Set(cert)
	c.UnknownExtKeyUsage.Set(cert)
	c.SubjectKeyID.Set(cert)
	c.AuthorityKeyID.Set(cert)
	c.OCSPServer.Set(cert)
	c.IssuingCertificateURL.Set(cert)
	c.CRLDistributionPoints.Set(cert)
	c.PolicyIdentifiers.Set(cert)
	if c.BasicConstraints != nil {
		c.BasicConstraints.Set(cert)
	}
	if c.NameConstraints != nil {
		c.NameConstraints.Set(cert)
	}

	// Custom Extensions.
	for _, e := range c.Extensions {
		e.Set(cert)
	}

	// Others.
	c.SerialNumber.Set(cert)
	c.SignatureAlgorithm.Set(cert)

	return cert
}

// hasExtendedSANs returns true if the certificate contains any SAN types that
// are not supported by the golang x509 library (i.e. RegisteredID, OtherName,
// DirectoryName, X400Address, or EDIPartyName)
// See also https://datatracker.ietf.org/doc/html/rfc5280.html#section-4.2.1.6
func (c *Certificate) hasExtendedSANs() bool {
	for _, san := range c.SANs {
		if !(san.Type == DNSType || san.Type == IPType || san.Type == URIType || san.Type == AutoType || san.Type == EmailType || san.Type == "") {
			return true
		}
	}
	return false
}

func (c *Certificate) hasExtension(oid ObjectIdentifier) bool {
	for _, e := range c.Extensions {
		if asn1.ObjectIdentifier(e.ID).Equal(asn1.ObjectIdentifier(oid)) {
			return true
		}
	}
	return false
}

// CreateCertificate signs the given template using the parent private key and
// returns it.
func CreateCertificate(template, parent *x509.Certificate, pub crypto.PublicKey, signer crypto.Signer) (*x509.Certificate, error) {
	var err error
	// Complete certificate.
	if template.SerialNumber == nil {
		if template.SerialNumber, err = generateSerialNumber(); err != nil {
			return nil, err
		}
	}
	if template.SubjectKeyId == nil {
		if template.SubjectKeyId, err = generateSubjectKeyID(pub); err != nil {
			return nil, err
		}
	}

	// Sign certificate
	asn1Data, err := x509.CreateCertificate(rand.Reader, template, parent, pub, signer)
	if err != nil {
		return nil, errors.Wrap(err, "error creating certificate")
	}
	cert, err := x509.ParseCertificate(asn1Data)
	if err != nil {
		return nil, errors.Wrap(err, "error parsing certificate")
	}
	return cert, nil
}

// CreateCertificateTemplate creates a X.509 certificate template from the given certificate request.
func CreateCertificateTemplate(cr *x509.CertificateRequest) (*x509.Certificate, error) {
	if err := cr.CheckSignature(); err != nil {
		return nil, errors.Wrap(err, "error validating certificate request")
	}
	// Set SubjectAltName extension as critical if Subject is empty.
	fixSubjectAltName(cr)

	return &x509.Certificate{
		Subject:            cr.Subject,
		DNSNames:           cr.DNSNames,
		EmailAddresses:     cr.EmailAddresses,
		IPAddresses:        cr.IPAddresses,
		URIs:               cr.URIs,
		ExtraExtensions:    cr.Extensions,
		PublicKey:          cr.PublicKey,
		PublicKeyAlgorithm: cr.PublicKeyAlgorithm,
		SignatureAlgorithm: 0,
	}, nil
}
