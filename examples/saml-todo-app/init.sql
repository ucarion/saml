create table accounts (
  id uuid not null primary key,
  saml_issuer varchar,
  saml_x509 bytea,
  saml_redirect_url varchar
);

create table users (
  id uuid not null primary key,
  account_id uuid not null references accounts (id),
  saml_id varchar,
  display_name varchar not null,
  password_hash varchar not null,

  unique (account_id, saml_id)
);

create table sessions (
  id uuid not null primary key,
  user_id uuid not null references users (id),
  expires_at timestamptz not null
);

create table todos (
  id uuid not null primary key,
  author_id uuid not null references users (id),
  body varchar not null
);
