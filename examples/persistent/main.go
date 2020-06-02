package main

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/julienschmidt/httprouter"
	_ "github.com/lib/pq"
	"github.com/ucarion/saml"
)

type connection struct {
	ID          uuid.UUID `json:"id" db:"id"`
	Issuer      string    `json:"issuer" db:"issuer"`
	X509        []byte    `json:"x509" db:"x509"`
	RedirectURL string    `json:"redirect_url" db:"redirect_url"`
}

type store struct {
	DB *sqlx.DB
}

func (s *store) getConnection(ctx context.Context, id uuid.UUID) (connection, error) {
	var c connection
	err := s.DB.GetContext(ctx, &c, `select * from connections where id = $1`, id)
	return c, err
}

func (s *store) createConnection(ctx context.Context, c connection) error {
	_, err := s.DB.ExecContext(ctx, `insert into connections (id) values ($1)`, c.ID)
	return err
}

func (s *store) updateConnection(ctx context.Context, c connection) error {
	_, err := s.DB.ExecContext(ctx, `
		update
			connections
		set
			issuer = $1, x509 = $2, redirect_url = $3
		where
			id = $4
	`, c.Issuer, c.X509, c.RedirectURL, c.ID)

	return err
}

func main() {
	db, err := sqlx.Open("postgres", "postgres://postgres:password@localhost?sslmode=disable")
	if err != nil {
		panic(err)
	}

	store := store{DB: db}
	router := httprouter.New()

	router.GET("/connections/:id", func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		id, err := uuid.Parse(p.ByName("id"))
		if err != nil {
			fmt.Fprintf(w, err.Error())
			return
		}

		c, err := store.getConnection(r.Context(), id)
		if err != nil {
			fmt.Fprintf(w, err.Error())
			return
		}

		json.NewEncoder(w).Encode(c)
	})

	router.POST("/connections", func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		c := connection{ID: uuid.New()}
		err := store.createConnection(r.Context(), c)
		if err != nil {
			fmt.Fprintf(w, err.Error())
			return
		}

		json.NewEncoder(w).Encode(c)
	})

	router.PATCH("/connections/:id/metadata", func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		defer r.Body.Close()

		id, err := uuid.Parse(p.ByName("id"))
		if err != nil {
			fmt.Fprintf(w, err.Error())
			return
		}

		var metadata saml.EntityDescriptor
		if err := xml.NewDecoder(r.Body).Decode(&metadata); err != nil {
			fmt.Fprintf(w, err.Error())
			return
		}

		issuer, cert, redirectURL, err := metadata.GetEntityIDCertificateAndRedirectURL()
		if err != nil {
			fmt.Fprintf(w, err.Error())
			return
		}

		c := connection{ID: id, Issuer: issuer, X509: cert.Raw, RedirectURL: redirectURL.String()}
		err = store.updateConnection(r.Context(), c)
		if err != nil {
			fmt.Fprintf(w, err.Error())
			return
		}

		json.NewEncoder(w).Encode(c)
	})

	router.GET("/connections/:id/initiate", func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		id, err := uuid.Parse(p.ByName("id"))
		if err != nil {
			fmt.Fprintf(w, err.Error())
			return
		}

		c, err := store.getConnection(r.Context(), id)
		if err != nil {
			fmt.Fprintf(w, err.Error())
			return
		}

		redirectURL, err := url.Parse(c.RedirectURL)
		if err != nil {
			fmt.Fprintf(w, err.Error())
			return
		}

		relayState := r.URL.Query().Get("relay_state")
		query := redirectURL.Query()
		query.Set(saml.ParamRelayState, relayState)
		redirectURL.RawQuery = query.Encode()

		http.Redirect(w, r, redirectURL.String(), http.StatusFound)
	})

	router.POST("/connections/:id/acs", func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		id, err := uuid.Parse(p.ByName("id"))
		if err != nil {
			fmt.Fprintf(w, err.Error())
			return
		}

		c, err := store.getConnection(r.Context(), id)
		if err != nil {
			fmt.Fprintf(w, err.Error())
			return
		}

		if err := r.ParseForm(); err != nil {
			fmt.Fprintf(w, err.Error())
			return
		}

		cert, err := x509.ParseCertificate(c.X509)
		if err != nil {
			fmt.Fprintf(w, err.Error())
			return
		}

		rawSAMLResponse := r.FormValue(saml.ParamSAMLResponse)
		samlResponse, err := saml.Verify(rawSAMLResponse, c.Issuer, cert, fmt.Sprintf("http://localhost:8080/connections/%s", id), time.Now())
		if err != nil {
			fmt.Fprintf(w, err.Error())
			return
		}

		relayState := r.FormValue(saml.ParamRelayState)

		// In real life, at this point you would need to integrate samlResponse into
		// your existing authentication / authorization system. There's no
		// one-size-fits-all way of doing that, you'll need to do some critical
		// thinking.
		//
		// See the README of github.com/ucarion/saml for some discussion around what
		// SAML does and does not guarantee you, and for some suggestions around how
		// to integrate SAML into your existing model.
		//
		// For now, just as a demo, let's write the saml response back out as JSON.
		json.NewEncoder(w).Encode(map[string]interface{}{
			"saml_response": samlResponse,
			"relay_state":   relayState,
		})
	})

	if err := http.ListenAndServe("localhost:8080", router); err != nil {
		panic(err)
	}
}
