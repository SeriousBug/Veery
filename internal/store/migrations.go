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
		backup_eligible INTEGER NOT NULL DEFAULT 0,
		backup_state INTEGER NOT NULL DEFAULT 0,
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
	// for_user binds a recovery invite to an existing user: enrolling on it adds a
	// fresh passkey to that user rather than creating a new one.
	`ALTER TABLE invites ADD COLUMN for_user TEXT REFERENCES users(id) ON DELETE CASCADE;`,
	// update_jobs outlives the process running the update. A self-update replaces
	// the Veery container mid-flight, so the process that reports the outcome is
	// never the one that started the update; a crash leaves a row unfinished for
	// startup recovery to reconcile.
	`CREATE TABLE update_jobs (
		id TEXT PRIMARY KEY,
		container_name TEXT NOT NULL,
		image TEXT NOT NULL DEFAULT '',
		phase TEXT NOT NULL DEFAULT '',
		message TEXT NOT NULL DEFAULT '',
		error TEXT NOT NULL DEFAULT '',
		is_self INTEGER NOT NULL DEFAULT 0,
		done INTEGER NOT NULL DEFAULT 0,
		started_at INTEGER NOT NULL,
		finished_at INTEGER NOT NULL DEFAULT 0
	);`,
	// container_id is the container the snapshot was taken from. Docker gives a
	// recreated container a new id, so an id that no longer matches the live
	// container means something outside Veery (a compose file edit, a manual
	// docker run) recreated it and the snapshot is stale. Rows written before
	// this column existed carry an empty id and are backfilled on the first
	// reconcile sweep.
	`ALTER TABLE managed_containers ADD COLUMN container_id TEXT NOT NULL DEFAULT '';`,
	// events is the recorded, searchable history of everything the notifier sees.
	// A row is written for every event, muted for delivery or not, so the log is
	// a complete record rather than an archive of only what was sent. It is
	// pruned by age, so no foreign keys tie it to containers or stacks: a row
	// outlives the service it names, and container_name/stack_id are kept as
	// plain text for linking and filtering.
	`CREATE TABLE events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		event TEXT NOT NULL,
		title TEXT NOT NULL,
		body TEXT NOT NULL DEFAULT '',
		container_name TEXT NOT NULL DEFAULT '',
		stack_id TEXT NOT NULL DEFAULT '',
		created_at INTEGER NOT NULL
	);`,
	// Paging walks (created_at, id) descending; filtering narrows by event type
	// or by the service a row names.
	`CREATE INDEX idx_events_created_at ON events(created_at DESC, id DESC);`,
	`CREATE INDEX idx_events_container ON events(container_name);`,
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
