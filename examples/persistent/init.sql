create table connections (
  id uuid not null primary key,
  issuer varchar,
  x509 bytea,
  redirect_url varchar
);
