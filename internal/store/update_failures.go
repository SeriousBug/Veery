package store

import "time"

// UpdateFailure counts the auto-update attempts that have failed to install one
// version on one container. Target is the digest the registry served for the
// container's image tag at the time of the attempt, so each published version
// is counted on its own: a release that cannot be installed is the usual case,
// and it must not poison the count of the release that comes after it.
type UpdateFailure struct {
	ContainerName string
	Target        string
	Failures      int
	LastError     string
	FirstAt       int64
	LastAt        int64
}

const failureCols = `container_name,target,failures,last_error,first_at,last_at`

// RecordUpdateFailure counts one more failed attempt at installing target on a
// container and returns the running total for that version.
func (s *Store) RecordUpdateFailure(containerName, target, errMsg string) (UpdateFailure, error) {
	now := time.Now().Unix()
	_, err := s.db.Exec(`INSERT INTO update_failures(`+failureCols+`)
		VALUES(?,?,1,?,?,?)
		ON CONFLICT(container_name,target) DO UPDATE SET
			failures=failures+1, last_error=excluded.last_error, last_at=excluded.last_at`,
		containerName, target, errMsg, now, now)
	if err != nil {
		return UpdateFailure{}, err
	}
	row := s.db.QueryRow(`SELECT `+failureCols+`
		FROM update_failures WHERE container_name=? AND target=?`, containerName, target)
	var f UpdateFailure
	err = row.Scan(&f.ContainerName, &f.Target, &f.Failures, &f.LastError, &f.FirstAt, &f.LastAt)
	return f, err
}

// UpdateFailures returns the per-version failure counts recorded for a
// container, newest first.
func (s *Store) UpdateFailures(containerName string) ([]UpdateFailure, error) {
	rows, err := s.db.Query(`SELECT `+failureCols+`
		FROM update_failures WHERE container_name=? ORDER BY last_at DESC`, containerName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UpdateFailure
	for rows.Next() {
		var f UpdateFailure
		if err := rows.Scan(&f.ContainerName, &f.Target, &f.Failures, &f.LastError, &f.FirstAt, &f.LastAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// ClearUpdateFailures forgets every recorded failure for a container. An update
// that finally lands means the container is no longer stuck, whatever failed
// before it, and re-enabling auto-update is the user saying to start over.
func (s *Store) ClearUpdateFailures(containerName string) error {
	_, err := s.db.Exec(`DELETE FROM update_failures WHERE container_name=?`, containerName)
	return err
}
