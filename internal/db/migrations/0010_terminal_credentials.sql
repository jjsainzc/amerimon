CREATE TABLE IF NOT EXISTS terminal_credentials (
    target_key     TEXT PRIMARY KEY,
    encrypted_data BLOB NOT NULL,
    updated_at     TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
