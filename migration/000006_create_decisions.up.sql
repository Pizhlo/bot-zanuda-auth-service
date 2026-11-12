CREATE TABLE auth.decisions (
    decision_id UUID PRIMARY KEY,
    request_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    client_id TEXT NOT NULL,
    subject_id UUID NOT NULL,
    action TEXT NOT NULL,
    action_attributes JSON NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id UUID NOT NULL,
    space_id UUID,
    checked_object TEXT,
    checked_relation TEXT,
    result TEXT NOT NULL CHECK (result IN ('allow', 'deny')),
    reasons JSON NOT NULL,
    policy_version TEXT NOT NULL,
    authorization_model_id TEXT NOT NULL,
    space_type TEXT CHECK (space_type IN ('shared', 'personal'))
);

CREATE INDEX decisions_request_id_idx ON auth.decisions (request_id);
