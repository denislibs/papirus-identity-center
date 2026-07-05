CREATE TABLE org_units (
    id           UUID PRIMARY KEY,
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    parent_id    UUID REFERENCES org_units(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    sort_order   INT NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_org_units_ws ON org_units (workspace_id);

CREATE TABLE positions (
    id           UUID PRIMARY KEY,
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    title        TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_positions_ws ON positions (workspace_id);

ALTER TABLE workspace_members
    ADD COLUMN org_unit_id UUID REFERENCES org_units(id) ON DELETE SET NULL,
    ADD COLUMN position_id UUID REFERENCES positions(id) ON DELETE SET NULL;
