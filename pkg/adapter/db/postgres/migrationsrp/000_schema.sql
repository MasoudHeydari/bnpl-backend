SET search_path TO bnpl;

CREATE TABLE migrations
(
    id         BIGSERIAL PRIMARY KEY,
    name       TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);
