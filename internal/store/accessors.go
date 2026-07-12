package store

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/SeriousBug/Veery/internal/api"
)

// ErrNotFound is returned when a lookup finds no row.
var ErrNotFound = errors.New("not found")

// --- Users ---

// CountUsers returns the number of registered users.
func (s *Store) CountUsers() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

// CreateUser inserts a user.
func (s *Store) CreateUser(id, name string, isAdmin bool) (api.User, error) {
	now := time.Now().Unix()
	_, err := s.db.Exec(`INSERT INTO users(id,name,is_admin,created_at) VALUES(?,?,?,?)`,
		id, name, boolInt(isAdmin), now)
	if err != nil {
		return api.User{}, err
	}
	return api.User{ID: id, Name: name, IsAdmin: isAdmin, CreatedAt: now}, nil
}

// GetUser looks up a user by id.
func (s *Store) GetUser(id string) (api.User, error) {
	var u api.User
	var admin int
	err := s.db.QueryRow(`SELECT id,name,is_admin,created_at FROM users WHERE id=?`, id).
		Scan(&u.ID, &u.Name, &admin, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return u, ErrNotFound
	}
	u.IsAdmin = admin != 0
	return u, err
}

// ListUsers returns all users ordered by creation.
func (s *Store) ListUsers() ([]api.User, error) {
	rows, err := s.db.Query(`SELECT id,name,is_admin,created_at FROM users ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []api.User
	for rows.Next() {
		var u api.User
		var admin int
		if err := rows.Scan(&u.ID, &u.Name, &admin, &u.CreatedAt); err != nil {
			return nil, err
		}
		u.IsAdmin = admin != 0
		out = append(out, u)
	}
	return out, rows.Err()
}

// CountAdmins returns the number of admin users.
func (s *Store) CountAdmins() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM users WHERE is_admin=1`).Scan(&n)
	return n, err
}

// DeleteUser removes a user; credentials and sessions cascade via FK.
func (s *Store) DeleteUser(id string) error {
	res, err := s.db.Exec(`DELETE FROM users WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// --- Credentials ---

// StoredCredential is a passkey row, decoded from the DB.
type StoredCredential struct {
	ID         string
	UserID     string
	CredID     []byte
	PublicKey  []byte
	SignCount  uint32
	Transports []string
	AAGUID     []byte
	Name       string
	CreatedAt  int64
}

// AddCredential stores a new passkey for a user.
func (s *Store) AddCredential(c StoredCredential) error {
	_, err := s.db.Exec(`INSERT INTO credentials(id,user_id,cred_id,public_key,sign_count,transports,aaguid,name,created_at)
		VALUES(?,?,?,?,?,?,?,?,?)`,
		c.ID, c.UserID, c.CredID, c.PublicKey, c.SignCount,
		strings.Join(c.Transports, ","), c.AAGUID, c.Name, c.CreatedAt)
	return err
}

// CredentialsByUser returns all passkeys for a user.
func (s *Store) CredentialsByUser(userID string) ([]StoredCredential, error) {
	rows, err := s.db.Query(`SELECT id,user_id,cred_id,public_key,sign_count,transports,aaguid,name,created_at
		FROM credentials WHERE user_id=? ORDER BY created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCreds(rows)
}

// CredentialByCredID looks up a passkey by its raw credential id.
func (s *Store) CredentialByCredID(credID []byte) (StoredCredential, error) {
	rows, err := s.db.Query(`SELECT id,user_id,cred_id,public_key,sign_count,transports,aaguid,name,created_at
		FROM credentials WHERE cred_id=?`, credID)
	if err != nil {
		return StoredCredential{}, err
	}
	defer rows.Close()
	creds, err := scanCreds(rows)
	if err != nil {
		return StoredCredential{}, err
	}
	if len(creds) == 0 {
		return StoredCredential{}, ErrNotFound
	}
	return creds[0], nil
}

// UpdateSignCount persists an incremented authenticator sign counter.
func (s *Store) UpdateSignCount(credID []byte, count uint32) error {
	_, err := s.db.Exec(`UPDATE credentials SET sign_count=? WHERE cred_id=?`, count, credID)
	return err
}

func scanCreds(rows *sql.Rows) ([]StoredCredential, error) {
	var out []StoredCredential
	for rows.Next() {
		var c StoredCredential
		var transports string
		if err := rows.Scan(&c.ID, &c.UserID, &c.CredID, &c.PublicKey, &c.SignCount,
			&transports, &c.AAGUID, &c.Name, &c.CreatedAt); err != nil {
			return nil, err
		}
		if transports != "" {
			c.Transports = strings.Split(transports, ",")
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// --- Invites ---

// CreateInvite stores a single-use invite.
func (s *Store) CreateInvite(token, createdBy string, isAdmin bool, expiresAt int64) error {
	var cb any
	if createdBy != "" {
		cb = createdBy
	}
	_, err := s.db.Exec(`INSERT INTO invites(token,created_by,is_admin,expires_at,used_at,created_at)
		VALUES(?,?,?,?,0,?)`, token, cb, boolInt(isAdmin), expiresAt, time.Now().Unix())
	return err
}

// InviteRow is a stored invite.
type InviteRow struct {
	Token     string
	IsAdmin   bool
	ExpiresAt int64
	UsedAt    int64
	CreatedAt int64
}

// GetInvite returns an invite by token.
func (s *Store) GetInvite(token string) (InviteRow, error) {
	var r InviteRow
	var admin int
	err := s.db.QueryRow(`SELECT token,is_admin,expires_at,used_at,created_at FROM invites WHERE token=?`, token).
		Scan(&r.Token, &admin, &r.ExpiresAt, &r.UsedAt, &r.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return r, ErrNotFound
	}
	r.IsAdmin = admin != 0
	return r, err
}

// ConsumeInvite marks an invite used only if currently unused and unexpired.
// Returns ErrNotFound if it cannot be consumed.
func (s *Store) ConsumeInvite(token string) error {
	now := time.Now().Unix()
	res, err := s.db.Exec(`UPDATE invites SET used_at=? WHERE token=? AND used_at=0 AND expires_at>?`,
		now, token, now)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteInvite removes an invite by token (used to revoke a pending invite).
func (s *Store) DeleteInvite(token string) error {
	res, err := s.db.Exec(`DELETE FROM invites WHERE token=?`, token)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListPendingInvites returns unused, unexpired invites.
func (s *Store) ListPendingInvites() ([]InviteRow, error) {
	now := time.Now().Unix()
	rows, err := s.db.Query(`SELECT token,is_admin,expires_at,used_at,created_at FROM invites
		WHERE used_at=0 AND expires_at>? ORDER BY created_at DESC`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []InviteRow
	for rows.Next() {
		var r InviteRow
		var admin int
		if err := rows.Scan(&r.Token, &admin, &r.ExpiresAt, &r.UsedAt, &r.CreatedAt); err != nil {
			return nil, err
		}
		r.IsAdmin = admin != 0
		out = append(out, r)
	}
	return out, rows.Err()
}

// --- Sessions ---

// CreateSession stores a session token.
func (s *Store) CreateSession(token, userID string, expiresAt int64) error {
	_, err := s.db.Exec(`INSERT INTO sessions(token,user_id,expires_at,created_at) VALUES(?,?,?,?)`,
		token, userID, expiresAt, time.Now().Unix())
	return err
}

// SessionUser returns the user for a valid (unexpired) session token.
func (s *Store) SessionUser(token string) (api.User, error) {
	var uid string
	var exp int64
	err := s.db.QueryRow(`SELECT user_id,expires_at FROM sessions WHERE token=?`, token).Scan(&uid, &exp)
	if errors.Is(err, sql.ErrNoRows) {
		return api.User{}, ErrNotFound
	}
	if err != nil {
		return api.User{}, err
	}
	if exp < time.Now().Unix() {
		s.DeleteSession(token)
		return api.User{}, ErrNotFound
	}
	return s.GetUser(uid)
}

// DeleteSession removes a session.
func (s *Store) DeleteSession(token string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE token=?`, token)
	return err
}

// --- Settings ---

// GetSetting returns a setting value or ("", ErrNotFound).
func (s *Store) GetSetting(key string) (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return v, err
}

// SetSetting upserts a setting.
func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO settings(key,value) VALUES(?,?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
