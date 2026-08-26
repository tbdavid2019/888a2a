-- Organization-scoped Web Widget configuration.
CREATE TABLE IF NOT EXISTS a2a888_web_widget_config (
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    widget_id TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT false,
    session_ttl_seconds INTEGER NOT NULL DEFAULT 900,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, widget_id),
    CONSTRAINT a2a888_web_widget_config_identity_check CHECK (organization_id <> '' AND widget_id <> ''),
    CONSTRAINT a2a888_web_widget_config_ttl_check CHECK (session_ttl_seconds BETWEEN 60 AND 86400)
);
