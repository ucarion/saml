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
		SAML TodoApp
	</h1>

	<h2>Sign up</h2>

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

	<h2>Log in</h2>

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
`))

type getAccountData struct {
	ID              string
	CurrentUser     user
	SAMLACS         string
	SAMLRecipientID string
	SAMLIssuerID    string
	SAMLIssuerX509  string
	SAMLRedirectURL string
	Users           []user
	Todos           []todoWithAuthor
}

type todoWithAuthor struct {
	Todo todo
	User user
}

var getAccountTemplate = template.Must(template.New("get_account").Parse(`
	<!DOCTYPE html>
	<head>
		<style>
		table, th, td { border: 1px solid black; }
		</style>
	</head>
	<body>
		<h1>SAML TodoApp</h1>

		<p>You are logged in as: {{ .CurrentUser.DisplayName }} (id = {{ .CurrentUser.ID }}) </p>

		<h2>SAML Configuration</h2>

		<a href="/accounts/{{ .ID }}/saml/initiate">Initiate SAML Login Flow</a>

		<table>
			<caption>
				Data you need to put into your Identity Provider
			</caption>

			<tr>
				<td>SAML Assertion Consumer Service ("ACS") URL</td>
				<td><code>{{ .SAMLACS }}</code></td>
			</tr>
			<tr>
				<td>SAML Recipient ID</td>
				<td><code>{{ .SAMLRecipientID }}</code></td>
			</tr>
		</table>

		<table>
			<caption>
				Data from your Identity Provider you need to give us
			</caption>

			<tr>
				<td>SAML Issuer Entity ID</td>
				<td><code>{{ .SAMLIssuerID }}</code></td>
			</tr>

			<tr>
				<td>SAML Issuer x509 Certificate</td>
				<td><code><pre>{{ .SAMLIssuerX509 }}</pre></code></td>
			</tr>

			<tr>
				<td>SAML Redirect URL (aka "HTTP-Redirect Binding URL")</td>
				<td><code><pre>{{ .SAMLRedirectURL }}</pre></code></td>
			</tr>
		</table>

		<form action="/accounts/{{ .ID }}/metadata" method="post" enctype="multipart/form-data">
			<input type="file" accept=".xml" name="metadata" />
			<button>Upload Identity Provider SAML Metadata</button>
		</form>

		<h2>Users</h2>

		<p>There are {{ len .Users }} users:</p>

		<ul>
			{{ range .Users }}
				<li>
					{{ .DisplayName }} (id = {{ .ID }})
				</li>
			{{ end }}
		</ul>

		<form action="/accounts/{{ .ID }}/users" method="post">
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

		<h2>Todos</h2>

		There are {{ len .Todos }} todos:

		<ul>
			{{ range .Todos }}
				<li>
					{{ .User.DisplayName }}: {{ .Todo.Body }} (id = {{ .Todo.ID }})
				</li>
			{{ end }}
		</ul>

		<form action="/accounts/{{ .ID }}/todos" method="post">
			<label>
				Body
				<input type="text" name="body" />
			</label>

			<button>Create a todo</button>
		</form>
	</body>
`))

func with500(f func(w http.ResponseWriter, r *http.Request, p httprouter.Params) error) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		if err := f(w, r, p); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "internal server error: %s", err.Error())
		}
	}
}

func issueSession(ctx context.Context, s *store, w http.ResponseWriter, u user) error {
	sess := session{ID: uuid.New(), UserID: u.ID, ExpiresAt: time.Now().Add(time.Hour * 24)}
	if err := s.createSession(ctx, sess); err != nil {
		return err
	}

	http.SetCookie(w, &http.Cookie{
		Name:    "session_token",
		Path:    "/",
		Expires: sess.ExpiresAt,
		Value:   sess.ID.String(),
	})

	return nil
}

var errUnauthorized = errors.New("unauthorized")

func authorize(s *store, w http.ResponseWriter, r *http.Request, accountID string) (user, error) {
	sessionCookie, err := r.Cookie("session_token")
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return user{}, errUnauthorized
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
		w.WriteHeader(http.StatusForbidden)
		return user{}, errUnauthorized
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

	router.POST("/accounts", with500(func(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
		displayName := r.FormValue("root_display_name")
		password := r.FormValue("root_password")

		a := account{ID: uuid.New()}
		if err := store.createAccount(r.Context(), a); err != nil {
			return err
		}

		passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}

		u := user{ID: uuid.New(), AccountID: a.ID, DisplayName: displayName, PasswordHash: passwordHash}
		if err := store.createUser(r.Context(), u); err != nil {
			return err
		}

		if err := issueSession(r.Context(), &store, w, u); err != nil {
			return err
		}

		http.Redirect(w, r, fmt.Sprintf("/accounts/%s", a.ID.String()), http.StatusFound)
		return nil
	}))

	router.POST("/login", with500(func(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
		userID := r.FormValue("id")
		password := r.FormValue("password")

		userUUID, err := uuid.Parse(userID)
		if err != nil {
			return err
		}

		user, err := store.getUser(r.Context(), userUUID)
		if err != nil {
			return err
		}

		if err := bcrypt.CompareHashAndPassword(user.PasswordHash, []byte(password)); err != nil {
			return err
		}

		if err := issueSession(r.Context(), &store, w, user); err != nil {
			return err
		}

		http.Redirect(w, r, fmt.Sprintf("/accounts/%s", user.AccountID.String()), http.StatusFound)
		return nil
	}))

	router.GET("/accounts/:account_id", with500(func(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
		accountID := p.ByName("account_id")
		currentUser, err := authorize(&store, w, r, accountID)
		if err != nil {
			return nil
		}

		accountUUID, err := uuid.Parse(accountID)
		if err != nil {
			return err
		}

		account, err := store.getAccount(r.Context(), accountUUID)
		if err != nil {
			return err
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

		todos, err := store.listTodos(r.Context(), accountUUID)
		if err != nil {
			return err
		}

		users, err := store.listUsers(r.Context(), accountUUID)
		if err != nil {
			return err
		}

		todosWithAuthors := []todoWithAuthor{}
		for _, todo := range todos {
			for _, user := range users {
				if user.ID == todo.AuthorID {
					todosWithAuthors = append(todosWithAuthors, todoWithAuthor{Todo: todo, User: user})
				}
			}
		}

		w.Header().Add("content-type", "text/html")
		getAccountTemplate.Execute(w, getAccountData{
			ID:              accountID,
			CurrentUser:     currentUser,
			SAMLACS:         fmt.Sprintf("http://localhost:8080/accounts/%s/saml/acs", accountID),
			SAMLRecipientID: fmt.Sprintf("http://localhost:8080/accounts/%s/saml", accountID),
			SAMLIssuerID:    issuer,
			SAMLIssuerX509:  issuerX509,
			SAMLRedirectURL: redirectURL,
			Users:           users,
			Todos:           todosWithAuthors,
		})

		return nil
	}))

	router.POST("/accounts/:account_id/metadata", with500(func(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
		accountID := p.ByName("account_id")
		if _, err := authorize(&store, w, r, accountID); err != nil {
			return nil
		}

		accountUUID, err := uuid.Parse(accountID)
		if err != nil {
			return err
		}

		file, _, err := r.FormFile("metadata")
		if err != nil {
			return err
		}

		defer file.Close()
		var metadata saml.EntityDescriptor
		if err := xml.NewDecoder(file).Decode(&metadata); err != nil {
			return err
		}

		entityID, cert, redirectURL, err := metadata.GetEntityIDCertificateAndRedirectURL()
		if err != nil {
			return err
		}

		samlRedirectURL := redirectURL.String()
		store.updateAccount(r.Context(), account{
			ID:              accountUUID,
			SAMLIssuer:      &entityID,
			SAMLX509:        cert.Raw,
			SAMLRedirectURL: &samlRedirectURL,
		})

		http.Redirect(w, r, fmt.Sprintf("/accounts/%s", accountID), http.StatusFound)
		return nil
	}))

	router.GET("/accounts/:account_id/saml/initiate", with500(func(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
		// This endpoint is intentionally not checking for authentication /
		// authorization. Think of this endpoint as a customizable login page, where
		// we redirect the user to a SAML identity provider of the account's
		// choosing.
		accountID := p.ByName("account_id")

		accountUUID, err := uuid.Parse(accountID)
		if err != nil {
			return err
		}

		account, err := store.getAccount(r.Context(), accountUUID)
		if err != nil {
			return err
		}

		http.Redirect(w, r, *account.SAMLRedirectURL, http.StatusFound)
		return nil
	}))

	router.POST("/accounts/:account_id/saml/acs", with500(func(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
		// This is the endpoint that users get redirected to from /saml/initiate
		// above, or when log into our app directly from their Identity Provider.
		accountID := p.ByName("account_id")

		accountUUID, err := uuid.Parse(accountID)
		if err != nil {
			return err
		}

		account, err := store.getAccount(r.Context(), accountUUID)
		if err != nil {
			return err
		}

		cert, err := x509.ParseCertificate(account.SAMLX509)
		if err != nil {
			return err
		}

		// This is the destination ID we expect to see in the SAML assertion. We
		// verify this to make sure that this SAML assertion is meant for us, and
		// not some other SAML application in the identity provider.
		expectedDestinationID := fmt.Sprintf("http://localhost:8080/accounts/%s/saml", accountID)

		// Get the raw SAML response, and verify it.
		rawSAMLResponse := r.FormValue(saml.ParamSAMLResponse)
		samlResponse, err := saml.Verify(rawSAMLResponse, *account.SAMLIssuer, cert, expectedDestinationID, time.Now())
		if err != nil {
			return err
		}

		// samlUserID will contain the user ID from the identity provider.
		//
		// If a user with that saml_id already exists in our database, we'll log the
		// user in as them. If no such user already exists, we'll create one first.
		samlUserID := samlResponse.Assertion.Subject.NameID.Value
		existingUser, err := store.getUserBySAMLID(r.Context(), accountUUID, samlUserID)

		// loginUser will contain the user we should create a session for.
		var loginUser user
		if err == nil {
			// A user with the given saml_id in this account already exists. Log into
			// that user.
			loginUser = existingUser
		} else if err == sql.ErrNoRows {
			// No such user already exists. Create one now.
			//
			// This practice of creating a user like this is often called
			// "just-in-time" provisioning.
			provisionedUser := user{
				AccountID:   accountUUID,
				ID:          uuid.New(),
				SAMLID:      &samlUserID,
				DisplayName: samlUserID,
			}

			if err := store.createUser(r.Context(), provisionedUser); err != nil {
				return err
			}

			loginUser = provisionedUser
		} else {
			return err
		}

		if err := issueSession(r.Context(), &store, w, loginUser); err != nil {
			return err
		}

		http.Redirect(w, r, fmt.Sprintf("/accounts/%s", accountID), http.StatusFound)
		return nil
	}))

	router.POST("/accounts/:account_id/users", with500(func(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
		accountID := p.ByName("account_id")
		if _, err := authorize(&store, w, r, accountID); err != nil {
			return err
		}

		accountUUID, err := uuid.Parse(accountID)
		if err != nil {
			return err
		}

		displayName := r.FormValue("display_name")
		password := r.FormValue("password")

		passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}

		if err := store.createUser(r.Context(), user{
			AccountID:    accountUUID,
			ID:           uuid.New(),
			DisplayName:  displayName,
			PasswordHash: passwordHash,
		}); err != nil {
			return err
		}

		http.Redirect(w, r, fmt.Sprintf("/accounts/%s", accountID), http.StatusFound)
		return nil
	}))

	router.POST("/accounts/:account_id/todos", with500(func(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
		accountID := p.ByName("account_id")
		author, err := authorize(&store, w, r, accountID)
		if err != nil {
			return err
		}

		body := r.FormValue("body")

		if err := store.createTodo(r.Context(), todo{
			ID:       uuid.New(),
			AuthorID: author.ID,
			Body:     body,
		}); err != nil {
			return err
		}

		http.Redirect(w, r, fmt.Sprintf("/accounts/%s", accountID), http.StatusFound)
		return nil
	}))

	if err := http.ListenAndServe("localhost:8080", router); err != nil {
		panic(err)
	}
}
