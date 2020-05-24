package main

import (
	"crypto/x509"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/ucarion/saml"
)

func main() {
	var issuer string
	var cert *x509.Certificate
	var redirectURL *url.URL

	http.HandleFunc("/setup", func(w http.ResponseWriter, r *http.Request) {
		var metadata saml.EntityDescriptor
		if err := xml.NewDecoder(r.Body).Decode(&metadata); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "%s", err)
			return
		}

		inputIssuer, inputCert, inputRedirectURL, err := metadata.GetEntityIDCertificateAndRedirectURL()
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "%s", err)
			return
		}

		issuer = inputIssuer
		cert = inputCert
		redirectURL = inputRedirectURL
	})

	http.HandleFunc("/acs", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "%s", err)
			return
		}

		samlResponse, err := saml.Verify(
			r.FormValue("SAMLResponse"),
			issuer,
			cert,
			"http://localhost:8080/acs",
			time.Now(),
		)

		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "%s", err)
			return
		}

		response := map[string]interface{}{
			"assertion":   samlResponse.Assertion,
			"relay_state": r.FormValue("RelayState"),
		}

		w.Header().Set("content-type", "application/json")
		json.NewEncoder(w).Encode(response)
	})

	http.HandleFunc("/initiate", func(w http.ResponseWriter, r *http.Request) {
		redirectURL := *redirectURL
		redirectURL.Query().Add("RelayState", r.URL.Query().Get("relay_state"))
		http.Redirect(w, r, redirectURL.String(), http.StatusFound)
	})

	http.ListenAndServe("localhost:8080", nil)
}
