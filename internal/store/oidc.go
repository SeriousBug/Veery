package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"time"
)

// OIDCIdentity links a Veery user to one external OIDC identity. The identity is
// keyed by (issuer, subject) rather than by the user, so a login from an unknown
// provider/subject has to be provisioned before it can be used.
type OIDCIdentity struct {
	ID        string
	UserID    string
	Issuer    string
	Subject   string
	Email     string
	CreatedAt int64
}

// OIDCIdentityBySubject looks up an identity by the provider's issuer and the
// ID token's subject claim.
func (s *Store) OIDCIdentityBySubject(issuer, subject string) (OIDCIdentity, error) {
	var id OIDCIdentity
	err := s.db.QueryRow(`SELECT id,user_id,issuer,subject,email,created_at FROM oidc_identities
		WHERE issuer=? AND subject=?`, issuer, subject).
		Scan(&id.ID, &id.UserID, &id.Issuer, &id.Subject, &id.Email, &id.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return id, ErrNotFound
	}
	return id, err
}

// LinkOIDCIdentity stores a new provider link for a user.
func (s *Store) LinkOIDCIdentity(userID, issuer, subject, email string) error {
	_, err := s.db.Exec(`INSERT INTO oidc_identities(id,user_id,issuer,subject,email,created_at)
		VALUES(?,?,?,?,?,?)`,
		randomID(), userID, issuer, subject, email, time.Now().Unix())
	return err
}

// OIDCIdentitiesByUser returns every provider link a user has.
func (s *Store) OIDCIdentitiesByUser(userID string) ([]OIDCIdentity, error) {
	rows, err := s.db.Query(`SELECT id,user_id,issuer,subject,email,created_at FROM oidc_identities
		WHERE user_id=? ORDER BY created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OIDCIdentity
	for rows.Next() {
		var id OIDCIdentity
		if err := rows.Scan(&id.ID, &id.UserID, &id.Issuer, &id.Subject, &id.Email, &id.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// SetUserAdmin changes a user's admin flag and reports whether the row existed.
func (s *Store) SetUserAdmin(id string, isAdmin bool) error {
	res, err := s.db.Exec(`UPDATE users SET is_admin=? WHERE id=?`, boolInt(isAdmin), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// randomID returns a URL-safe random identifier for a new row.
func randomID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
