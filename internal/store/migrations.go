package store

import "strconv"

// migrations are applied in order. Each is idempotent-safe via a schema_version
// row tracked in the meta table.
var migrations = []string{
	`CREATE TABLE users (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		is_admin INTEGER NOT NULL DEFAULT 0,
		created_at INTEGER NOT NULL
	);`,
	`CREATE TABLE credentials (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		cred_id BLOB NOT NULL UNIQUE,
		public_key BLOB NOT NULL,
		sign_count INTEGER NOT NULL DEFAULT 0,
		transports TEXT NOT NULL DEFAULT '',
		aaguid BLOB,
		name TEXT NOT NULL DEFAULT '',
		created_at INTEGER NOT NULL
	);`,
	`CREATE TABLE invites (
		token TEXT PRIMARY KEY,
		created_by TEXT REFERENCES users(id) ON DELETE SET NULL,
		is_admin INTEGER NOT NULL DEFAULT 0,
		expires_at INTEGER NOT NULL,
		used_at INTEGER NOT NULL DEFAULT 0,
		created_at INTEGER NOT NULL
	);`,
	`CREATE TABLE sessions (
		token TEXT PRIMARY KEY,
		user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		expires_at INTEGER NOT NULL,
		created_at INTEGER NOT NULL
	);`,
	`CREATE TABLE stacks (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL UNIQUE
	);`,
	`CREATE TABLE managed_containers (
		id TEXT PRIMARY KEY,
		stack_id TEXT NOT NULL REFERENCES stacks(id) ON DELETE CASCADE,
		container_name TEXT NOT NULL,
		snapshot_json TEXT NOT NULL,
		auto_update INTEGER NOT NULL DEFAULT 0,
		created_at INTEGER NOT NULL
	);`,
	`CREATE TABLE settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);`,
}

func (s *Store) migrate() error {
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
		return err
	}
	var current int
	row := s.db.QueryRow(`SELECT value FROM meta WHERE key='schema_version'`)
	var v string
	if err := row.Scan(&v); err == nil {
		if n, err := strconv.Atoi(v); err == nil {
			current = n
		}
	}
	for i := current; i < len(migrations); i++ {
		if _, err := s.db.Exec(migrations[i]); err != nil {
			return err
		}
	}
	_, err := s.db.Exec(`INSERT INTO meta(key,value) VALUES('schema_version',?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, strconv.Itoa(len(migrations)))
	return err
}
