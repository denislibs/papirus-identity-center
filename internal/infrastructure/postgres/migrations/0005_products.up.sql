CREATE TABLE products (
    key  TEXT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE TABLE workspace_products (
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    product_key  TEXT NOT NULL REFERENCES products(key) ON DELETE CASCADE,
    enabled_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (workspace_id, product_key)
);

INSERT INTO products (key, name) VALUES
    ('papyrus', 'Papyrus (СЭД)'),
    ('lite', 'Papyrus Lite')
ON CONFLICT (key) DO NOTHING;
