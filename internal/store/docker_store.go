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
	AutoUpdate    bool
	CreatedAt     int64
}

// UpsertStack inserts or leaves a stack (id == name), returning the stack.
func (s *Store) UpsertStack(name string) (Stack, error) {
	_, err := s.db.Exec(`INSERT INTO stacks(id,name) VALUES(?,?)
		ON CONFLICT(id) DO NOTHING`, name, name)
	if err != nil {
		return Stack{}, err
	}
	return Stack{ID: name, Name: name}, nil
}

// AddManagedContainer persists (or replaces) a managed container by name.
func (s *Store) AddManagedContainer(mc ManagedContainer) error {
	if mc.CreatedAt == 0 {
		mc.CreatedAt = time.Now().Unix()
	}
	_, err := s.db.Exec(`INSERT INTO managed_containers(id,stack_id,container_name,snapshot_json,auto_update,created_at)
		VALUES(?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET stack_id=excluded.stack_id, snapshot_json=excluded.snapshot_json`,
		mc.ID, mc.StackID, mc.ContainerName, mc.SnapshotJSON, boolInt(mc.AutoUpdate), mc.CreatedAt)
	return err
}

// ManagedByName looks up a managed container by its container name.
func (s *Store) ManagedByName(name string) (ManagedContainer, error) {
	var mc ManagedContainer
	var au int
	err := s.db.QueryRow(`SELECT id,stack_id,container_name,snapshot_json,auto_update,created_at
		FROM managed_containers WHERE container_name=?`, name).
		Scan(&mc.ID, &mc.StackID, &mc.ContainerName, &mc.SnapshotJSON, &au, &mc.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return mc, ErrNotFound
	}
	mc.AutoUpdate = au != 0
	return mc, err
}

// ManagedByID looks up a managed container by its id.
func (s *Store) ManagedByID(id string) (ManagedContainer, error) {
	var mc ManagedContainer
	var au int
	err := s.db.QueryRow(`SELECT id,stack_id,container_name,snapshot_json,auto_update,created_at
		FROM managed_containers WHERE id=?`, id).
		Scan(&mc.ID, &mc.StackID, &mc.ContainerName, &mc.SnapshotJSON, &au, &mc.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return mc, ErrNotFound
	}
	mc.AutoUpdate = au != 0
	return mc, err
}

// ManagedByStack returns all managed containers in a stack.
func (s *Store) ManagedByStack(stackID string) ([]ManagedContainer, error) {
	rows, err := s.db.Query(`SELECT id,stack_id,container_name,snapshot_json,auto_update,created_at
		FROM managed_containers WHERE stack_id=? ORDER BY container_name`, stackID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanManaged(rows)
}

// AllManaged returns every managed container.
func (s *Store) AllManaged() ([]ManagedContainer, error) {
	rows, err := s.db.Query(`SELECT id,stack_id,container_name,snapshot_json,auto_update,created_at
		FROM managed_containers`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanManaged(rows)
}

// AutoUpdateContainers returns managed containers with auto-update enabled.
func (s *Store) AutoUpdateContainers() ([]ManagedContainer, error) {
	rows, err := s.db.Query(`SELECT id,stack_id,container_name,snapshot_json,auto_update,created_at
		FROM managed_containers WHERE auto_update=1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanManaged(rows)
}

func scanManaged(rows *sql.Rows) ([]ManagedContainer, error) {
	var out []ManagedContainer
	for rows.Next() {
		var mc ManagedContainer
		var au int
		if err := rows.Scan(&mc.ID, &mc.StackID, &mc.ContainerName, &mc.SnapshotJSON, &au, &mc.CreatedAt); err != nil {
			return nil, err
		}
		mc.AutoUpdate = au != 0
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

// UpdateSnapshot refreshes the stored create-spec for a managed container.
func (s *Store) UpdateSnapshot(id, snapshotJSON string) error {
	_, err := s.db.Exec(`UPDATE managed_containers SET snapshot_json=? WHERE id=?`, snapshotJSON, id)
	return err
}

// Unadopt removes a stack and its managed containers.
func (s *Store) Unadopt(stackID string) error {
	_, err := s.db.Exec(`DELETE FROM stacks WHERE id=?`, stackID)
	return err
}
