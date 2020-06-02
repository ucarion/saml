package saml

import (
	"bytes"
	"crypto/x509"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"net/url"
	"time"

	"github.com/ucarion/dsig"
)

// ParamSAMLResponse is the name of the HTTP POST parameter where SAML puts
// responses.
//
// Usually, you want to pass ParamSAMLResponse to `r.FormValue` when writing
// HTTP handlers that are responding to SAML logins.
const ParamSAMLResponse = "SAMLResponse"

// ParamRelayState is the name of the HTTP POST parameter where SAML puts relay
// states. It's also the name of the URL query parameter you should put your
// relay state in when initiating the SAML flow.
//
// Usually, you want to pass ParamRelayState to `r.FormValue` when writing HTTP
// handlers that are responding to SAML logins.
//
// Usually, you want to use ParamRelayState as the URL parameter "key" when
// writing HTTP handlers that are initiating SAML flows.
const ParamRelayState = "RelayState"

// ErrResponseNotSigned indicates that the SAML response was not signed.
//
// Verify does not support handling unsigned SAML responses. Note that some
// Identity Providers support signing either the full SAML response, or only the
// SAML assertion: Verify only supports having the full SAML response signed,
// and will ignore any additional interior signatures.
var ErrResponseNotSigned = errors.New("saml: response not signed")

// ErrAssertionExpired indicates that the SAML response is expired, or not yet
// valid.
var ErrAssertionExpired = errors.New("saml: assertion expired")

// ErrInvalidIssuer indicates that the SAML response did not have the expected
// issuer.
//
// This error may indicate that an attacker is attempting to replay a SAML
// assertion issed by their own identity provider instead of the authorized
// identity provider.
var ErrInvalidIssuer = errors.New("saml: invalid issuer")

// ErrInvalidRecipient indicates that the SAML response did not have the
// expected recipient.
//
// This error may indicates that an attacker is attempting to replay a SAML
// assertion meant for a different service provider.
var ErrInvalidRecipient = errors.New("saml: invalid recipient")

// Verify parses and verifies a SAML response.
//
// samlResponse should be the HTTP POST body parameter. Consider using
// ParamSAMLResponse to fetch this.
//
// issuer is the expected issuer of the SAML assertion. If samlResponse was
// issued by a different entity, Verify returns ErrInvalidIssuer.
//
// cert is the x509 certificate that the issuer is expected to have signed
// samlResponse with. If samlResponse was not signed at all, Verify returns
// ErrResponseNotSigned. If samlResponse was incorrectly signed, Verify will
// return an error from Verify in github.com/ucarion/dsig.
//
// recipient is the expected recipient of the SAML assertion. If samlResponse
// was issued for a different entity, Verify returns ErrInvalidRecipient.
//
// now should be the current time in production systems, although you may want
// to use a hard-coded time in unit tests. It is used to verify whether
// samlResponse is expired. If samlResponse is expired, Verify returns
// ErrAssertionExpired.
//
// Verify does not check if cert is expired.
func Verify(samlResponse, issuer string, cert *x509.Certificate, recipient string, now time.Time) (Response, error) {
	data, err := base64.StdEncoding.DecodeString(samlResponse)
	if err != nil {
		return Response{}, err
	}

	var response Response
	if err := xml.Unmarshal(data, &response); err != nil {
		return Response{}, err
	}

	if response.Signature.SignatureValue == "" {
		return Response{}, ErrResponseNotSigned
	}

	decoder := xml.NewDecoder(bytes.NewReader(data))
	if err := response.Signature.Verify(cert, decoder); err != nil {
		return Response{}, err
	}

	if response.Assertion.Issuer.Name != issuer {
		return Response{}, ErrInvalidIssuer
	}

	if response.Assertion.Subject.SubjectConfirmation.SubjectConfirmationData.Recipient != recipient {
		return Response{}, ErrInvalidRecipient
	}

	if now.Before(response.Assertion.Conditions.NotBefore) {
		return Response{}, ErrAssertionExpired
	}

	if now.After(response.Assertion.Conditions.NotOnOrAfter) {
		return Response{}, ErrAssertionExpired
	}

	if now.After(response.Assertion.Subject.SubjectConfirmation.SubjectConfirmationData.NotOnOrAfter) {
		return Response{}, ErrAssertionExpired
	}

	return response, nil
}

// Response represents a SAML response.
//
// Verify can construct and verify a Response from an HTTP body parameter.
type Response struct {
	XMLName   xml.Name       `xml:"urn:oasis:names:tc:SAML:2.0:protocol Response"`
	Signature dsig.Signature `xml:"Signature"`
	Assertion Assertion      `xml:"Assertion"`
}

// Assertion represents a SAML assertion.
//
// An assertion is a set of facts that one entity (usually an Identity Provider)
// passes to another entity (usually a Service Provider). These facts are
// usually information about a particular user, called a subject.
type Assertion struct {
	XMLName            xml.Name           `xml:"urn:oasis:names:tc:SAML:2.0:assertion Assertion"`
	Issuer             Issuer             `xml:"Issuer"`
	Subject            Subject            `xml:"Subject"`
	Conditions         Conditions         `xml:"Conditions"`
	AttributeStatement AttributeStatement `xml:"AttributeStatement"`
}

// Issuer indicates the entity that issued a SAML assertion.
type Issuer struct {
	XMLName xml.Name `xml:"urn:oasis:names:tc:SAML:2.0:assertion Issuer"`
	Name    string   `xml:",chardata"`
}

// Subject indicates the user the SAML assertion is about.
type Subject struct {
	XMLName             xml.Name            `xml:"urn:oasis:names:tc:SAML:2.0:assertion Subject"`
	NameID              NameID              `xml:"NameID"`
	SubjectConfirmation SubjectConfirmation `xml:"SubjectConfirmation"`
}

// NameID describes the primary identifier of the user.
type NameID struct {
	XMLName xml.Name `xml:"urn:oasis:names:tc:SAML:2.0:assertion NameID"`
	Format  string   `xml:"Format,attr"`
	Value   string   `xml:",chardata"`
}

// SubjectConfirmation is a set of information that indicates how, and under
// what conditions, the user's identity was confirmed.
type SubjectConfirmation struct {
	XMLName                 xml.Name                `xml:"urn:oasis:names:tc:SAML:2.0:assertion SubjectConfirmation"`
	SubjectConfirmationData SubjectConfirmationData `xml:"SubjectConfirmationData"`
}

// SubjectConfirmationData is a set of constraints about what entities should
// accept this subject, and when the assertion should no longer be considered
// valid.
type SubjectConfirmationData struct {
	XMLName      xml.Name  `xml:"urn:oasis:names:tc:SAML:2.0:assertion SubjectConfirmationData"`
	NotOnOrAfter time.Time `xml:"NotOnOrAfter,attr"`
	Recipient    string    `xml:"Recipient,attr"`
}

// Conditions is a set of constraints that limit under what conditions an
// assertion is valid.
type Conditions struct {
	XMLName      xml.Name  `xml:"urn:oasis:names:tc:SAML:2.0:assertion Conditions"`
	NotBefore    time.Time `xml:"NotBefore,attr"`
	NotOnOrAfter time.Time `xml:"NotOnOrAfter,attr"`
}

// AttributeStatement is a set of user attributes.
type AttributeStatement struct {
	XMLName    xml.Name    `xml:"urn:oasis:names:tc:SAML:2.0:assertion AttributeStatement"`
	Attributes []Attribute `xml:"Attribute"`
}

// Attribute is a particular key-value attribute of the user in an assertion.
type Attribute struct {
	XMLName    xml.Name `xml:"urn:oasis:names:tc:SAML:2.0:assertion Attribute"`
	Name       string   `xml:"Name,attr"`
	NameFormat string   `xml:"NameFormat,attr"`
	Value      string   `xml:"AttributeValue"`
}

// EntityDescriptor describes a SAML entity. This is often referred to as
// "metadata".
//
// This struct is meant to store "Identity Provider metadata"; it's meant to
// store the description of a SAML Identity Provider.
type EntityDescriptor struct {
	XMLName          xml.Name         `xml:"urn:oasis:names:tc:SAML:2.0:metadata EntityDescriptor"`
	EntityID         string           `xml:"entityID,attr"`
	IDPSSODescriptor IDPSSODescriptor `xml:"IDPSSODescriptor"`
}

// ErrNoRedirectBinding indicates that an EntityDescriptor did not declare an
// HTTP-Redirect binding.
var ErrNoRedirectBinding = errors.New("saml: no HTTP redirect binding in IdP metadata")

// SingleSignOnServiceBindingHTTPRedirect is the URI for a SAML HTTP-Redirect
// Binding.
const SingleSignOnServiceBindingHTTPRedirect = "urn:oasis:names:tc:SAML:2.0:bindings:HTTP-Redirect"

// GetEntityIDCertificateAndRedirectURL extracts an issuer entity ID, a x509
// certificate, and a redirect URL from a set of Identity Provider metadata.
//
// Returns an error if the x509 certificate or redirect URL are malformed. If
// there is no redirect URL at all, returns ErrNoRedirectBinding.
func (d *EntityDescriptor) GetEntityIDCertificateAndRedirectURL() (string, *x509.Certificate, *url.URL, error) {
	asn1Data, err := base64.StdEncoding.DecodeString(d.IDPSSODescriptor.KeyDescriptor.KeyInfo.X509Data.X509Certificate.Value)
	if err != nil {
		return "", nil, nil, err
	}

	cert, err := x509.ParseCertificate(asn1Data)
	if err != nil {
		return "", nil, nil, err
	}

	for _, s := range d.IDPSSODescriptor.SingleSignOnServices {
		if s.Binding == SingleSignOnServiceBindingHTTPRedirect {
			location, err := url.Parse(s.Location)
			return d.EntityID, cert, location, err
		}
	}

	return "", nil, nil, ErrNoRedirectBinding
}

// IDPSSODescriptor describes the single-sign-on offerings of an identity
// provider.
type IDPSSODescriptor struct {
	XMLName              xml.Name              `xml:"urn:oasis:names:tc:SAML:2.0:metadata IDPSSODescriptor"`
	KeyDescriptor        KeyDescriptor         `xml:"KeyDescriptor"`
	SingleSignOnServices []SingleSignOnService `xml:"SingleSignOnService"`
}

// KeyDescriptor describes the key an identity provider uses to sign data.
type KeyDescriptor struct {
	XMLName xml.Name `xml:"urn:oasis:names:tc:SAML:2.0:metadata KeyDescriptor"`
	KeyInfo KeyInfo  `xml:"KeyInfo"`
}

// KeyInfo is a XML-DSig description of a x509 key.
type KeyInfo struct {
	XMLName  xml.Name `xml:"http://www.w3.org/2000/09/xmldsig# KeyInfo"`
	X509Data X509Data `xml:"X509Data"`
}

// X509Data contains an x509 certificate.
type X509Data struct {
	XMLName         xml.Name        `xml:"http://www.w3.org/2000/09/xmldsig# X509Data"`
	X509Certificate X509Certificate `xml:"X509Certificate"`
}

// X509Certificate contains the base64-encoded ASN.1 data of a x509 certificate.
type X509Certificate struct {
	XMLName xml.Name `xml:"http://www.w3.org/2000/09/xmldsig# X509Certificate"`
	Value   string   `xml:",chardata"`
}

// SingleSignOnService describes a single binding of an identity provider, and
// the URL where it can be reached.
type SingleSignOnService struct {
	XMLName  xml.Name `xml:"urn:oasis:names:tc:SAML:2.0:metadata SingleSignOnService"`
	Binding  string   `xml:"Binding,attr"`
	Location string   `xml:"Location,attr"`
}
