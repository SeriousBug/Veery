package store

import (
	"database/sql"
	"errors"
	"time"
)

// Stack is a persisted managed stack.
type Stack struct {
	ID   string
	Name string
}

// ManagedContainer is a persisted managed container with its create-spec
// snapshot.
type ManagedContainer struct {
	ID            string
	StackID       string
	ContainerName string
	SnapshotJSON  string
	// ContainerID is the Docker container the snapshot was taken from. It stops
	// matching the live container when something outside Veery recreates it,
	// which is how a stale snapshot is spotted.
	ContainerID string
	AutoUpdate  bool
	CreatedAt   int64
}

const managedCols = `id,stack_id,container_name,snapshot_json,container_id,auto_update,created_at`

// UpsertStack inserts or leaves a stack (id == name), returning the stack.
func (s *Store) UpsertStack(name string) (Stack, error) {
	_, err := s.db.Exec(`INSERT INTO stacks(id,name) VALUES(?,?)
		ON CONFLICT(id) DO NOTHING`, name, name)
	if err != nil {
		return Stack{}, err
	}
	return Stack{ID: name, Name: name}, nil
}

// StackExists reports whether a stack has been adopted.
func (s *Store) StackExists(id string) (bool, error) {
	var one int
	err := s.db.QueryRow(`SELECT 1 FROM stacks WHERE id=?`, id).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// AddManagedContainer persists (or replaces) a managed container by name.
func (s *Store) AddManagedContainer(mc ManagedContainer) error {
	if mc.CreatedAt == 0 {
		mc.CreatedAt = time.Now().Unix()
	}
	_, err := s.db.Exec(`INSERT INTO managed_containers(`+managedCols+`)
		VALUES(?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET stack_id=excluded.stack_id, snapshot_json=excluded.snapshot_json,
			container_id=excluded.container_id`,
		mc.ID, mc.StackID, mc.ContainerName, mc.SnapshotJSON, mc.ContainerID, boolInt(mc.AutoUpdate), mc.CreatedAt)
	return err
}

// ManagedByName looks up a managed container by its container name.
func (s *Store) ManagedByName(name string) (ManagedContainer, error) {
	row := s.db.QueryRow(`SELECT `+managedCols+` FROM managed_containers WHERE container_name=?`, name)
	mc, err := scanOneManaged(row)
	if errors.Is(err, sql.ErrNoRows) {
		return mc, ErrNotFound
	}
	return mc, err
}

// ManagedByID looks up a managed container by its id.
func (s *Store) ManagedByID(id string) (ManagedContainer, error) {
	row := s.db.QueryRow(`SELECT `+managedCols+` FROM managed_containers WHERE id=?`, id)
	mc, err := scanOneManaged(row)
	if errors.Is(err, sql.ErrNoRows) {
		return mc, ErrNotFound
	}
	return mc, err
}

// ResolveManaged looks up a managed container by its managed id, falling back
// to its container name. Both are stable identifiers the UI may send.
func (s *Store) ResolveManaged(idOrName string) (ManagedContainer, error) {
	if mc, err := s.ManagedByID(idOrName); err == nil {
		return mc, nil
	}
	return s.ManagedByName(idOrName)
}

// ManagedByStack returns all managed containers in a stack.
func (s *Store) ManagedByStack(stackID string) ([]ManagedContainer, error) {
	rows, err := s.db.Query(`SELECT `+managedCols+`
		FROM managed_containers WHERE stack_id=? ORDER BY container_name`, stackID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanManaged(rows)
}

// AllManaged returns every managed container.
func (s *Store) AllManaged() ([]ManagedContainer, error) {
	rows, err := s.db.Query(`SELECT ` + managedCols + ` FROM managed_containers`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanManaged(rows)
}

// AutoUpdateContainers returns managed containers with auto-update enabled.
func (s *Store) AutoUpdateContainers() ([]ManagedContainer, error) {
	rows, err := s.db.Query(`SELECT ` + managedCols + ` FROM managed_containers WHERE auto_update=1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanManaged(rows)
}

func scanOneManaged(row scanner) (ManagedContainer, error) {
	var mc ManagedContainer
	var au int
	err := row.Scan(&mc.ID, &mc.StackID, &mc.ContainerName, &mc.SnapshotJSON, &mc.ContainerID, &au, &mc.CreatedAt)
	mc.AutoUpdate = au != 0
	return mc, err
}

func scanManaged(rows *sql.Rows) ([]ManagedContainer, error) {
	var out []ManagedContainer
	for rows.Next() {
		mc, err := scanOneManaged(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, mc)
	}
	return out, rows.Err()
}

// SetAutoUpdate toggles auto-update for a managed container by id.
func (s *Store) SetAutoUpdate(id string, on bool) error {
	res, err := s.db.Exec(`UPDATE managed_containers SET auto_update=? WHERE id=?`, boolInt(on), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateSnapshot refreshes the stored create-spec for a managed container, and
// with it the container the spec was captured from.
func (s *Store) UpdateSnapshot(id, snapshotJSON, containerID string) error {
	_, err := s.db.Exec(`UPDATE managed_containers SET snapshot_json=?, container_id=? WHERE id=?`,
		snapshotJSON, containerID, id)
	return err
}

// DeleteManagedContainer drops one managed container, and the stack with it
// when it was the last one. A stack with no containers left is not something
// the UI can act on, and leaving it behind would keep offering to bring up a
// service that no longer has any parts.
func (s *Store) DeleteManagedContainer(id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var stackID string
	err = tx.QueryRow(`SELECT stack_id FROM managed_containers WHERE id=?`, id).Scan(&stackID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM managed_containers WHERE id=?`, id); err != nil {
		return err
	}
	var left int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM managed_containers WHERE stack_id=?`, stackID).Scan(&left); err != nil {
		return err
	}
	if left == 0 {
		if _, err := tx.Exec(`DELETE FROM stacks WHERE id=?`, stackID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Unadopt removes a stack and its managed containers.
func (s *Store) Unadopt(stackID string) error {
	_, err := s.db.Exec(`DELETE FROM stacks WHERE id=?`, stackID)
	return err
}
