CREATE TABLE IF NOT EXISTS enlaces_externos (
    dashid      INTEGER PRIMARY KEY AUTOINCREMENT,
    descripcion TEXT,
    url         TEXT,
    target      TEXT NOT NULL DEFAULT '_blank',
    is_active   INTEGER NOT NULL DEFAULT 1 CHECK (is_active IN (0, 1))
);
