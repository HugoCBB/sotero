CREATE TABLE IF NOT EXISTS articles (
    id              bigserial   PRIMARY KEY,
    url             text        NOT NULL,
    url_hash        bytea       GENERATED ALWAYS AS (sha256(url::bytea)) STORED,
    source          text        NOT NULL,
    title           text        NOT NULL DEFAULT '',
    published_at    timestamptz,
    first_seen_at   timestamptz NOT NULL DEFAULT now(),
    lang            text        NOT NULL DEFAULT '',
    meta            jsonb       NOT NULL DEFAULT '{}',
    body            text,
    status          text        NOT NULL DEFAULT 'pending',
    attempts        int         NOT NULL DEFAULT 0,
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    last_error      text,
    fetched_at      timestamptz
);

CREATE UNIQUE INDEX IF NOT EXISTS articles_url_hash_idx ON articles (url_hash);
CREATE INDEX IF NOT EXISTS articles_queue_idx ON articles (next_attempt_at) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS articles_classify_queue_idx ON articles (next_attempt_at) WHERE status = 'fetched';
CREATE INDEX IF NOT EXISTS articles_first_seen_idx ON articles (first_seen_at);

CREATE TABLE IF NOT EXISTS source_cursors (
    source     text        PRIMARY KEY,
    cursor     text        NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS companies (
    id        text             PRIMARY KEY,
    name      text             NOT NULL,
    country   text             NOT NULL DEFAULT '',
    sector    text             NOT NULL DEFAULT '',
    aliases   text[]           NOT NULL DEFAULT '{}',
    keywords  text[]           NOT NULL DEFAULT '{}',
    valuation double precision NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS edges (
    id         bigserial   PRIMARY KEY,
    src        text        NOT NULL REFERENCES companies (id) ON DELETE CASCADE,
    dst        text        NOT NULL REFERENCES companies (id) ON DELETE CASCADE,
    product    text        NOT NULL DEFAULT '',
    weight     double precision NOT NULL DEFAULT 0.5,
    exclusive  boolean     NOT NULL DEFAULT false,
    status     text        NOT NULL DEFAULT 'proposed',
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (src, dst)
);

CREATE TABLE IF NOT EXISTS evidence (
    edge_id    bigint NOT NULL REFERENCES edges (id) ON DELETE CASCADE,
    article_id bigint NOT NULL REFERENCES articles (id) ON DELETE CASCADE,
    PRIMARY KEY (edge_id, article_id)
);

CREATE TABLE IF NOT EXISTS classifications (
    id         bigserial   PRIMARY KEY,
    article_id bigint      NOT NULL REFERENCES articles (id) ON DELETE CASCADE,
    labeler    text        NOT NULL,
    relevant   boolean     NOT NULL,
    factual    boolean     NOT NULL,
    event_type text        NOT NULL DEFAULT '',
    reason     text        NOT NULL DEFAULT '',
    result     jsonb       NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS classifications_article_idx ON classifications (article_id, id DESC);

CREATE TABLE IF NOT EXISTS events (
    article_id bigint      NOT NULL REFERENCES articles (id) ON DELETE CASCADE,
    company_id text        NOT NULL REFERENCES companies (id) ON DELETE CASCADE,
    direction  smallint    NOT NULL,
    severity   smallint    NOT NULL,
    factual    boolean     NOT NULL,
    event_type text        NOT NULL DEFAULT '',
    domain     text        NOT NULL,
    seen_at    timestamptz NOT NULL,
    PRIMARY KEY (article_id, company_id)
);

CREATE INDEX IF NOT EXISTS events_company_seen_idx ON events (company_id, seen_at DESC);

CREATE OR REPLACE FUNCTION notify_graph() RETURNS trigger AS $$
BEGIN
    PERFORM pg_notify('graph', TG_TABLE_NAME);
    RETURN NULL;
END
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER companies_notify AFTER INSERT OR UPDATE OR DELETE ON companies
    FOR EACH STATEMENT EXECUTE FUNCTION notify_graph();
CREATE OR REPLACE TRIGGER edges_notify AFTER INSERT OR UPDATE OR DELETE ON edges
    FOR EACH STATEMENT EXECUTE FUNCTION notify_graph();
CREATE OR REPLACE TRIGGER events_notify AFTER INSERT OR UPDATE OR DELETE ON events
    FOR EACH STATEMENT EXECUTE FUNCTION notify_graph();
