package main

import (
	"context"
	"crypto/x509"
	"database/sql"
	"encoding/pem"
	"encoding/xml"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/julienschmidt/httprouter"
	_ "github.com/lib/pq"
	"github.com/ucarion/saml"
	"golang.org/x/crypto/bcrypt"
)

type account struct {
	ID              uuid.UUID `db:"id"`
	SAMLIssuer      *string   `db:"saml_issuer"`
	SAMLX509        []byte    `db:"saml_x509"`
	SAMLRedirectURL *string   `db:"saml_redirect_url"`
}

type user struct {
	ID           uuid.UUID `db:"id"`
	AccountID    uuid.UUID `db:"account_id"`
	SAMLID       *string   `db:"saml_id"`
	DisplayName  string    `db:"display_name"`
	PasswordHash []byte    `db:"password_hash"`
}

type session struct {
	ID        uuid.UUID `db:"id"`
	UserID    uuid.UUID `db:"user_id"`
	ExpiresAt time.Time `db:"expires_at"`
}

type todo struct {
	ID       uuid.UUID `db:"id"`
	AuthorID uuid.UUID `db:"author_id"`
	Body     string    `db:"body"`
}

type store struct {
	DB *sqlx.DB
}

func (s *store) getAccount(ctx context.Context, id uuid.UUID) (account, error) {
	var a account
	err := s.DB.GetContext(ctx, &a, `
		select
			id, saml_issuer, saml_x509, saml_redirect_url
		from
			accounts
		where
			id = $1
	`, id)
	return a, err
}

func (s *store) createAccount(ctx context.Context, a account) error {
	_, err := s.DB.ExecContext(ctx, `insert into accounts (id) values ($1)`, a.ID)
	return err
}

func (s *store) updateAccount(ctx context.Context, a account) error {
	_, err := s.DB.ExecContext(ctx, `
		update
			accounts
		set
			saml_issuer = $1,
			saml_x509 = $2,
			saml_redirect_url = $3
		where
			id = $4
	`, a.SAMLIssuer, a.SAMLX509, a.SAMLRedirectURL, a.ID)
	return err
}

func (s *store) listUsers(ctx context.Context, accountID uuid.UUID) ([]user, error) {
	var users []user
	err := s.DB.SelectContext(ctx, &users, `
		select
			id, account_id, display_name, password_hash
		from
			users
		where
			account_id = $1
	`, accountID)
	return users, err
}

func (s *store) getUser(ctx context.Context, id uuid.UUID) (user, error) {
	var u user
	err := s.DB.GetContext(ctx, &u, `
		select
			id, account_id, saml_id, display_name, password_hash
		from
			users
		where
			id = $1
	`, id)
	return u, err
}

func (s *store) getUserBySAMLID(ctx context.Context, accountID uuid.UUID, samlID string) (user, error) {
	var u user
	err := s.DB.GetContext(ctx, &u, `
		select
			id, account_id, saml_id, display_name, password_hash
		from
			users
		where
			account_id = $1 and saml_id = $2
	`, accountID, samlID)
	return u, err
}

func (s *store) createUser(ctx context.Context, u user) error {
	_, err := s.DB.ExecContext(ctx, `
		insert into users
			(id, account_id, saml_id, display_name, password_hash)
		values
			($1, $2, $3, $4, $5)
	`, u.ID, u.AccountID, u.SAMLID, u.DisplayName, u.PasswordHash)
	return err
}

func (s *store) createSession(ctx context.Context, sess session) error {
	_, err := s.DB.ExecContext(ctx, `
		insert into sessions
			(id, user_id, expires_at)
		values
			($1, $2, $3)
	`, sess.ID, sess.UserID, sess.ExpiresAt)
	return err
}

func (s *store) getSession(ctx context.Context, id uuid.UUID) (session, error) {
	var sess session
	err := s.DB.GetContext(ctx, &sess, `
		select
			id, user_id, expires_at
		from
			sessions
		where
			sessions.id = $1
	`, id)
	return sess, err
}

func (s *store) listTodos(ctx context.Context, accountID uuid.UUID) ([]todo, error) {
	var todos []todo
	err := s.DB.SelectContext(ctx, &todos, `
		select
			todos.id, todos.author_id, todos.body
		from
			todos
		join
			users on todos.author_id = users.id
		where
			users.account_id = $1
	`, accountID)
	return todos, err
}

func (s *store) createTodo(ctx context.Context, t todo) error {
	_, err := s.DB.ExecContext(ctx, `
		insert into todos
			(id, author_id, body)
		values
			($1, $2, $3)
	`, t.ID, t.AuthorID, t.Body)
	return err
}

var indexTemplate = template.Must(template.New("index").Parse(`
	<h1>
		TodoApp, Enterprise Edition
	</h1>

	<p>
		Obviously, this is a pretty ugly application. This app is meant to make it
		clearer how to integrate SAML into an application. It's not meant to be a
		production-ready system.
	</p>

	<form action="/login" method="post">
		<label>
			User ID (not username)

			<input type="text" name="id" />
		</label>

		<label>
			Password

			<input type="password" name="password" />
		</label>

		<button>Log into an existing user</button>
	</form>

	<form action="/accounts" method="post">
		<label>
			Username

			<input type="text" name="root_display_name" />
		</label>

		<label>
			Password

			<input type="password" name="root_password" />
		</label>

		<button>Create a new account</button>
	</form>
`))

type getAccountData struct {
	ID                string
	SAMLACS           string
	SAMLDestinationID string
	SAMLIssuerID      string
	SAMLIssuerX509    string
	SAMLRedirectURL   string
}

var getAccountTemplate = template.Must(template.New("get_account").Parse(`
	<p>Account ID {{ .ID }}</p>

	<a href="/accounts/{{ .ID }}/saml/initiate">Initiate SAML Login Flow</a>

	<p>SAML Connnection Details</p>

	<hr />

	<p><b>Data you need to put into your Identity Provider</b></p>

	<p>
		SAML ACS ("Assertion Consumer Service") URL: <code>{{ .SAMLACS }}</code>
	</p>

	<p>
		SAML Destination Entity ID: <code>{{ .SAMLDestinationID }}</code>
	</p>

	<hr />

	<p><b>Data from your Identity Provider you need to give us</b></p>

	<p>
		SAML Issuer Entity ID: <code>{{ .SAMLIssuerID }}</code>
	</p>

	<p>
		SAML Issuer x509 Certificate:

		<code><pre>{{ .SAMLIssuerX509 }}</pre></code>
	</p>

	<p>
		SAML SP-Initiated Redirect URL: <code>{{ .SAMLRedirectURL }}</code>
	</p>

	<form action="{{ .ID }}/metadata" method="post" enctype="multipart/form-data">
		<input type="file" accept=".xml" name="metadata" />

		<button>Upload Identity Provider SAML Metadata</button>
	</form>
`))

var listUsersTemplate = template.Must(template.New("list_users").Parse(`
	There are {{ len . }} users:

	<ul>
		{{ range . }}
			<li>
				{{ .DisplayName }} (id = {{ .ID }})
			</li>
		{{ end }}
	</ul>

	<form method="post">
		<label>
			Display Name
			<input type="text" name="display_name" />
		</label>

		<label>
			Password
			<input type="password" name="password" />
		</label>

		<button>Create a user</button>
	</form>
`))

type todoWithAuthor struct {
	Todo todo
	User user
}

var listTodosTemplate = template.Must(template.New("list_todos").Parse(`
	There are {{ len . }} todos:

	<ul>
		{{ range . }}
			<li>
				{{ .User.DisplayName }}: {{ .Todo.Body }} (id = {{ .Todo.ID }})
			</li>
		{{ end }}
	</ul>

	<form method="post">
		<label>
			Body
			<input type="text" name="body" />
		</label>

		<button>Create a todo</button>
	</form>
`))

func authorize(s *store, w http.ResponseWriter, r *http.Request, accountID string) (user, error) {
	fmt.Println(r.Cookies())

	sessionCookie, err := r.Cookie("session_token")
	if err != nil {
		return user{}, err
	}

	sessionID, err := uuid.Parse(sessionCookie.Value)
	if err != nil {
		return user{}, err
	}

	sess, err := s.getSession(r.Context(), sessionID)
	if err != nil {
		return user{}, err
	}

	u, err := s.getUser(r.Context(), sess.UserID)
	if err != nil {
		return user{}, err
	}

	if u.AccountID.String() != accountID {
		return user{}, errors.New("unauthorized")
	}

	return u, nil
}

func main() {
	db, err := sqlx.Open("postgres", "postgres://postgres:password@localhost?sslmode=disable")
	if err != nil {
		panic(err)
	}

	store := store{DB: db}
	router := httprouter.New()

	router.GET("/", func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		w.Header().Add("content-type", "text/html")
		indexTemplate.Execute(w, nil)
	})

	router.POST("/accounts", func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		if err := r.ParseForm(); err != nil {
			fmt.Fprintf(w, err.Error())
			return
		}

		displayName := r.FormValue("root_display_name")
		password := r.FormValue("root_password")

		a := account{ID: uuid.New()}
		if err := store.createAccount(r.Context(), a); err != nil {
			fmt.Fprintf(w, err.Error())
			return
		}

		passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			fmt.Fprintf(w, err.Error())
			return
		}

		u := user{ID: uuid.New(), AccountID: a.ID, DisplayName: displayName, PasswordHash: passwordHash}
		if err := store.createUser(r.Context(), u); err != nil {
			fmt.Fprintf(w, err.Error())
			return
		}

		s := session{ID: uuid.New(), UserID: u.ID, ExpiresAt: time.Now().Add(time.Hour * 24)}
		if err := store.createSession(r.Context(), s); err != nil {
			fmt.Fprintf(w, err.Error())
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:    "session_token",
			Path:    "/",
			Expires: s.ExpiresAt,
			Value:   s.ID.String(),
		})

		http.Redirect(w, r, fmt.Sprintf("/accounts/%s/todos", a.ID.String()), http.StatusFound)
	})

	router.POST("/login", func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		if err := r.ParseForm(); err != nil {
			fmt.Fprintf(w, err.Error())
			return
		}

		userID := r.FormValue("id")
		password := r.FormValue("password")

		userUUID, err := uuid.Parse(userID)
		if err != nil {
			fmt.Fprintf(w, err.Error())
			return
		}

		user, err := store.getUser(r.Context(), userUUID)
		if err != nil {
			fmt.Fprintf(w, err.Error())
			return
		}

		if err := bcrypt.CompareHashAndPassword(user.PasswordHash, []byte(password)); err != nil {
			fmt.Fprintf(w, err.Error())
			return
		}

		s := session{ID: uuid.New(), UserID: userUUID, ExpiresAt: time.Now().Add(time.Hour * 24)}
		if err := store.createSession(r.Context(), s); err != nil {
			fmt.Fprintf(w, err.Error())
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:    "session_token",
			Path:    "/",
			Expires: s.ExpiresAt,
			Value:   s.ID.String(),
		})

		http.Redirect(w, r, fmt.Sprintf("/accounts/%s/todos", user.AccountID.String()), http.StatusFound)
	})

	router.GET("/accounts/:account_id", func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		accountID := p.ByName("account_id")
		if _, err := authorize(&store, w, r, accountID); err != nil {
			fmt.Fprintf(w, err.Error())
			return
		}

		accountUUID, err := uuid.Parse(accountID)
		if err != nil {
			fmt.Fprintf(w, err.Error())
			return
		}

		account, err := store.getAccount(r.Context(), accountUUID)
		if err != nil {
			fmt.Fprintf(w, err.Error())
			return
		}

		issuer := ""
		if account.SAMLIssuer != nil {
			issuer = *account.SAMLIssuer
		}

		issuerX509 := ""
		if len(account.SAMLX509) != 0 {
			issuerX509 = string(pem.EncodeToMemory(&pem.Block{
				Type:  "CERTIFICATE",
				Bytes: account.SAMLX509,
			}))
		}

		redirectURL := ""
		if account.SAMLRedirectURL != nil {
			redirectURL = *account.SAMLRedirectURL
		}

		w.Header().Add("content-type", "text/html")
		getAccountTemplate.Execute(w, getAccountData{
			ID:                accountID,
			SAMLACS:           fmt.Sprintf("http://localhost:8080/accounts/%s/saml/acs", accountID),
			SAMLDestinationID: fmt.Sprintf("http://localhost:8080/accounts/%s/saml", accountID),
			SAMLIssuerID:      issuer,
			SAMLIssuerX509:    issuerX509,
			SAMLRedirectURL:   redirectURL,
		})
	})

	router.POST("/accounts/:account_id/metadata", func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		accountID := p.ByName("account_id")
		if _, err := authorize(&store, w, r, accountID); err != nil {
			fmt.Fprintf(w, err.Error())
			return
		}

		accountUUID, err := uuid.Parse(accountID)
		if err != nil {
			fmt.Fprintf(w, err.Error())
			return
		}

		if err := r.ParseMultipartForm(16 * 1024); err != nil {
			fmt.Fprintf(w, err.Error())
			return
		}

		file, _, err := r.FormFile("metadata")
		if err != nil {
			fmt.Fprintf(w, err.Error())
			return
		}

		defer file.Close()
		var metadata saml.EntityDescriptor
		if err := xml.NewDecoder(file).Decode(&metadata); err != nil {
			fmt.Fprintf(w, err.Error())
			return
		}

		entityID, cert, redirectURL, err := metadata.GetEntityIDCertificateAndRedirectURL()
		if err != nil {
			fmt.Fprintf(w, err.Error())
			return
		}

		samlRedirectURL := redirectURL.String()

		store.updateAccount(r.Context(), account{
			ID:              accountUUID,
			SAMLIssuer:      &entityID,
			SAMLX509:        cert.Raw,
			SAMLRedirectURL: &samlRedirectURL,
		})

		http.Redirect(w, r, fmt.Sprintf("/accounts/%s", accountID), http.StatusFound)
	})

	router.GET("/accounts/:account_id/saml/initiate", func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		// This endpoint is intentionally not checking for authentication /
		// authorization. Think of this endpoint as a customizable login page, where
		// we redirect the user to a SAML identity provider of the account's
		// choosing.
		accountID := p.ByName("account_id")

		accountUUID, err := uuid.Parse(accountID)
		if err != nil {
			fmt.Fprintf(w, err.Error())
			return
		}

		account, err := store.getAccount(r.Context(), accountUUID)
		if err != nil {
			fmt.Fprintf(w, err.Error())
			return
		}

		http.Redirect(w, r, *account.SAMLRedirectURL, http.StatusFound)
	})

	router.POST("/accounts/:account_id/saml/acs", func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		// This is the endpoint that users get redirected to from /saml/initiate
		// above, or when log into our app directly from their Identity Provider.
		accountID := p.ByName("account_id")

		accountUUID, err := uuid.Parse(accountID)
		if err != nil {
			fmt.Fprintf(w, err.Error())
			return
		}

		account, err := store.getAccount(r.Context(), accountUUID)
		if err != nil {
			fmt.Fprintf(w, err.Error())
			return
		}

		cert, err := x509.ParseCertificate(account.SAMLX509)
		if err != nil {
			fmt.Fprintf(w, err.Error())
			return
		}

		expectedDestinationID := fmt.Sprintf("http://localhost:8080/accounts/%s/saml", accountID)
		rawSAMLResponse := r.FormValue(saml.ParamSAMLResponse)
		samlResponse, err := saml.Verify(rawSAMLResponse, *account.SAMLIssuer, cert, expectedDestinationID, time.Now())
		if err != nil {
			fmt.Fprintf(w, err.Error())
			return
		}

		samlUserID := samlResponse.Assertion.Subject.NameID.Value
		existingUser, err := store.getUserBySAMLID(r.Context(), accountUUID, samlUserID)

		var loginUser user
		if err == nil {
			loginUser = existingUser
		} else if err == sql.ErrNoRows {
			provisionedUser := user{
				AccountID:   accountUUID,
				ID:          uuid.New(),
				SAMLID:      &samlUserID,
				DisplayName: samlUserID,
			}

			err := store.createUser(r.Context(), provisionedUser)

			if err != nil {
				fmt.Fprintf(w, err.Error())
				return
			}

			loginUser = provisionedUser
		} else {
			fmt.Fprintf(w, err.Error())
			return
		}

		s := session{ID: uuid.New(), UserID: loginUser.ID, ExpiresAt: time.Now().Add(time.Hour * 24)}
		if err := store.createSession(r.Context(), s); err != nil {
			fmt.Fprintf(w, err.Error())
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:    "session_token",
			Path:    "/",
			Expires: s.ExpiresAt,
			Value:   s.ID.String(),
		})

		http.Redirect(w, r, fmt.Sprintf("/accounts/%s/todos", accountID), http.StatusFound)
	})

	router.GET("/accounts/:account_id/users", func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		accountID := p.ByName("account_id")
		if _, err := authorize(&store, w, r, accountID); err != nil {
			fmt.Fprintf(w, err.Error())
			return
		}

		accountUUID, err := uuid.Parse(accountID)
		if err != nil {
			fmt.Fprintf(w, err.Error())
			return
		}

		users, err := store.listUsers(r.Context(), accountUUID)
		if err != nil {
			fmt.Fprintf(w, err.Error())
			return
		}

		w.Header().Add("content-type", "text/html")
		listUsersTemplate.Execute(w, users)
	})

	router.POST("/accounts/:account_id/users", func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		accountID := p.ByName("account_id")
		if _, err := authorize(&store, w, r, accountID); err != nil {
			fmt.Fprintf(w, err.Error())
			return
		}

		accountUUID, err := uuid.Parse(accountID)
		if err != nil {
			fmt.Fprintf(w, err.Error())
			return
		}

		if err := r.ParseForm(); err != nil {
			fmt.Fprintf(w, err.Error())
			return
		}

		displayName := r.FormValue("display_name")
		password := r.FormValue("password")

		passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			fmt.Fprintf(w, err.Error())
			return
		}

		err = store.createUser(r.Context(), user{
			AccountID:    accountUUID,
			ID:           uuid.New(),
			DisplayName:  displayName,
			PasswordHash: passwordHash,
		})

		if err != nil {
			fmt.Fprintf(w, err.Error())
			return
		}

		http.Redirect(w, r, "users", http.StatusFound)
	})

	router.GET("/accounts/:account_id/todos", func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		accountID := p.ByName("account_id")
		if _, err := authorize(&store, w, r, accountID); err != nil {
			fmt.Fprintf(w, err.Error())
			return
		}

		accountUUID, err := uuid.Parse(accountID)
		if err != nil {
			fmt.Fprintf(w, err.Error())
			return
		}

		todos, err := store.listTodos(r.Context(), accountUUID)
		if err != nil {
			fmt.Fprintf(w, err.Error())
			return
		}

		users, err := store.listUsers(r.Context(), accountUUID)
		if err != nil {
			fmt.Fprintf(w, err.Error())
			return
		}

		templateData := []todoWithAuthor{}
		for _, todo := range todos {
			for _, user := range users {
				if user.ID == todo.AuthorID {
					templateData = append(templateData, todoWithAuthor{Todo: todo, User: user})
				}
			}
		}

		w.Header().Add("content-type", "text/html")
		listTodosTemplate.Execute(w, templateData)
	})

	router.POST("/accounts/:account_id/todos", func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		accountID := p.ByName("account_id")
		author, err := authorize(&store, w, r, accountID)
		if err != nil {
			fmt.Fprintf(w, err.Error())
			return
		}

		if err := r.ParseForm(); err != nil {
			fmt.Fprintf(w, err.Error())
			return
		}

		body := r.FormValue("body")

		err = store.createTodo(r.Context(), todo{
			ID:       uuid.New(),
			AuthorID: author.ID,
			Body:     body,
		})

		if err != nil {
			fmt.Fprintf(w, err.Error())
			return
		}

		http.Redirect(w, r, "todos", http.StatusFound)
	})

	if err := http.ListenAndServe("localhost:8080", router); err != nil {
		panic(err)
	}
}
