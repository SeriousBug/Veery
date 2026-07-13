package store

import (
	"database/sql"
	"errors"
	"time"
)

// UpdateJob is a persisted record of an update in flight. It exists because the
// process that starts an update is not always the one that finishes it: a
// self-update hands off to a helper container and the original Veery exits, and
// a crash leaves the row unfinished for startup recovery to reconcile.
type UpdateJob struct {
	ID            string
	ContainerName string
	Image         string
	Phase         string
	Message       string
	Error         string
	Self          bool
	Done          bool
	StartedAt     int64
	FinishedAt    int64
}

// StartUpdateJob records a new in-flight update.
func (s *Store) StartUpdateJob(j UpdateJob) error {
	if j.StartedAt == 0 {
		j.StartedAt = time.Now().Unix()
	}
	_, err := s.db.Exec(`INSERT INTO update_jobs(id,container_name,image,phase,message,is_self,done,started_at)
		VALUES(?,?,?,?,?,?,0,?)
		ON CONFLICT(id) DO UPDATE SET phase=excluded.phase, message=excluded.message`,
		j.ID, j.ContainerName, j.Image, j.Phase, j.Message, boolInt(j.Self), j.StartedAt)
	return err
}

// SetUpdateJobPhase advances an in-flight update. Unknown ids are ignored so a
// helper container can report progress for a job it did not create.
func (s *Store) SetUpdateJobPhase(id, phase, message string) error {
	_, err := s.db.Exec(`UPDATE update_jobs SET phase=?, message=? WHERE id=? AND done=0`, phase, message, id)
	return err
}

// MarkUpdateJobSelf flags a job as an update of Veery's own container, which is
// carried out by a helper container rather than in-process.
func (s *Store) MarkUpdateJobSelf(id string) error {
	_, err := s.db.Exec(`UPDATE update_jobs SET is_self=1 WHERE id=?`, id)
	return err
}

// FinishUpdateJob marks an update finished. errMsg is empty on success.
func (s *Store) FinishUpdateJob(id, phase, message, errMsg string) error {
	_, err := s.db.Exec(`UPDATE update_jobs SET phase=?, message=?, error=?, done=1, finished_at=?
		WHERE id=?`, phase, message, errMsg, time.Now().Unix(), id)
	return err
}

// UpdateJobByID returns one update job.
func (s *Store) UpdateJobByID(id string) (UpdateJob, error) {
	row := s.db.QueryRow(`SELECT id,container_name,image,phase,message,error,is_self,done,started_at,finished_at
		FROM update_jobs WHERE id=?`, id)
	j, err := scanUpdateJob(row)
	if errors.Is(err, sql.ErrNoRows) {
		return j, ErrNotFound
	}
	return j, err
}

// ActiveUpdateJobs returns every update that has not reported a final state.
func (s *Store) ActiveUpdateJobs() ([]UpdateJob, error) {
	rows, err := s.db.Query(`SELECT id,container_name,image,phase,message,error,is_self,done,started_at,finished_at
		FROM update_jobs WHERE done=0 ORDER BY started_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UpdateJob
	for rows.Next() {
		j, err := scanUpdateJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// RecentUpdateJobs returns updates that finished since a unix timestamp. A
// self-update finishes while every client is disconnected (Veery is restarting),
// so the outcome has to be readable after the fact or the UI is left showing a
// spinner for an update that is long done.
func (s *Store) RecentUpdateJobs(since int64) ([]UpdateJob, error) {
	rows, err := s.db.Query(`SELECT id,container_name,image,phase,message,error,is_self,done,started_at,finished_at
		FROM update_jobs WHERE done=1 AND finished_at>=? ORDER BY finished_at`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UpdateJob
	for rows.Next() {
		j, err := scanUpdateJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanUpdateJob(row scanner) (UpdateJob, error) {
	var j UpdateJob
	var isSelf, done int
	err := row.Scan(&j.ID, &j.ContainerName, &j.Image, &j.Phase, &j.Message, &j.Error,
		&isSelf, &done, &j.StartedAt, &j.FinishedAt)
	j.Self = isSelf != 0
	j.Done = done != 0
	return j, err
}
