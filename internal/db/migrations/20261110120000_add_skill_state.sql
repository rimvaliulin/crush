-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS skill_state (
    session_id TEXT PRIMARY KEY,
    state TEXT NOT NULL,
    updated_at INTEGER NOT NULL,
    FOREIGN KEY (session_id) REFERENCES sessions (id) ON DELETE CASCADE
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS skill_state;
-- +goose StatementEnd
