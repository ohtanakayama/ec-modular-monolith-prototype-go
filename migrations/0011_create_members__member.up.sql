CREATE TABLE members.member (
    id          text        PRIMARY KEY,
    email       text        NOT NULL UNIQUE,
    name        text        NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);
