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

var ErrResponseNotSigned = errors.New("saml: response not signed")
var ErrAssertionExpired = errors.New("saml: assertion expired")
var ErrInvalidIssuer = errors.New("saml: invalid issuer")
var ErrInvalidRecipient = errors.New("saml: invalid recipient")

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

type EntityDescriptor struct {
	XMLName          xml.Name         `xml:"urn:oasis:names:tc:SAML:2.0:metadata EntityDescriptor"`
	EntityID         string           `xml:"entityID,attr"`
	IDPSSODescriptor IDPSSODescriptor `xml:"IDPSSODescriptor"`
}

var ErrNoRedirectBinding = errors.New("saml: no HTTP redirect binding in IdP metadata")

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

type IDPSSODescriptor struct {
	XMLName              xml.Name              `xml:"urn:oasis:names:tc:SAML:2.0:metadata IDPSSODescriptor"`
	KeyDescriptor        KeyDescriptor         `xml:"KeyDescriptor"`
	SingleSignOnServices []SingleSignOnService `xml:"SingleSignOnService"`
}

var SingleSignOnServiceBindingHTTPRedirect = "urn:oasis:names:tc:SAML:2.0:bindings:HTTP-Redirect"

type SingleSignOnService struct {
	XMLName  xml.Name `xml:"urn:oasis:names:tc:SAML:2.0:metadata SingleSignOnService"`
	Binding  string   `xml:"Binding,attr"`
	Location string   `xml:"Location,attr"`
}

type KeyDescriptor struct {
	XMLName xml.Name `xml:"urn:oasis:names:tc:SAML:2.0:metadata KeyDescriptor"`
	KeyInfo KeyInfo  `xml:"KeyInfo"`
}

type KeyInfo struct {
	XMLName  xml.Name `xml:"http://www.w3.org/2000/09/xmldsig# KeyInfo"`
	X509Data X509Data `xml:"X509Data"`
}

type X509Data struct {
	XMLName         xml.Name        `xml:"http://www.w3.org/2000/09/xmldsig# X509Data"`
	X509Certificate X509Certificate `xml:"X509Certificate"`
}

type X509Certificate struct {
	XMLName xml.Name `xml:"http://www.w3.org/2000/09/xmldsig# X509Certificate"`
	Value   string   `xml:",chardata"`
}

type Response struct {
	XMLName   xml.Name       `xml:"urn:oasis:names:tc:SAML:2.0:protocol Response"`
	Signature dsig.Signature `xml:"Signature"`
	Assertion Assertion      `xml:"Assertion"`
}

type Issuer struct {
	XMLName xml.Name `xml:"urn:oasis:names:tc:SAML:2.0:assertion Issuer"`
	Name    string   `xml:",chardata"`
}

type Status struct {
	XMLName    xml.Name   `xml:"urn:oasis:names:tc:SAML:2.0:protocol Status"`
	StatusCode StatusCode `xml:"StatusCode"`
}

type StatusCode struct {
	XMLName xml.Name `xml:"urn:oasis:names:tc:SAML:2.0:protocol StatusCode"`
	Value   string   `xml:"Value,attr"`
}

type Assertion struct {
	XMLName            xml.Name           `xml:"urn:oasis:names:tc:SAML:2.0:assertion Assertion"`
	Issuer             Issuer             `xml:"Issuer"`
	Subject            Subject            `xml:"Subject"`
	Conditions         Conditions         `xml:"Conditions"`
	AttributeStatement AttributeStatement `xml:"AttributeStatement"`
}

type Subject struct {
	XMLName             xml.Name            `xml:"urn:oasis:names:tc:SAML:2.0:assertion Subject"`
	NameID              NameID              `xml:"NameID"`
	SubjectConfirmation SubjectConfirmation `xml:"SubjectConfirmation"`
}

type NameID struct {
	XMLName xml.Name `xml:"urn:oasis:names:tc:SAML:2.0:assertion NameID"`
	Format  string   `xml:"Format,attr"`
	Value   string   `xml:",chardata"`
}

type SubjectConfirmation struct {
	XMLName                 xml.Name                `xml:"urn:oasis:names:tc:SAML:2.0:assertion SubjectConfirmation"`
	Method                  string                  `xml:"Method,attr"`
	SubjectConfirmationData SubjectConfirmationData `xml:"SubjectConfirmationData"`
}

type SubjectConfirmationData struct {
	XMLName      xml.Name  `xml:"urn:oasis:names:tc:SAML:2.0:assertion SubjectConfirmationData"`
	NotOnOrAfter time.Time `xml:"NotOnOrAfter,attr"`
	Recipient    string    `xml:"Recipient,attr"`
}

type Conditions struct {
	XMLName      xml.Name  `xml:"urn:oasis:names:tc:SAML:2.0:assertion Conditions"`
	NotBefore    time.Time `xml:"NotBefore,attr"`
	NotOnOrAfter time.Time `xml:"NotOnOrAfter,attr"`
}

type AttributeStatement struct {
	XMLName    xml.Name    `xml:"urn:oasis:names:tc:SAML:2.0:assertion AttributeStatement"`
	Attributes []Attribute `xml:"Attribute"`
}

type Attribute struct {
	XMLName    xml.Name `xml:"urn:oasis:names:tc:SAML:2.0:assertion Attribute"`
	Name       string   `xml:"Name,attr"`
	NameFormat string   `xml:"NameFormat,attr"`
	Value      string   `xml:"AttributeValue"`
}
