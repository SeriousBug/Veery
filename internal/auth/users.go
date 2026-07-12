// Package auth implements passkey-only authentication: WebAuthn registration
// and login, server-side sessions, and single-use invites.
package auth

import (
	"github.com/SeriousBug/Veery/internal/store"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

// authUser adapts a stored user + its credentials to the webauthn.User
// interface expected by go-webauthn.
type authUser struct {
	id    []byte
	name  string
	creds []webauthn.Credential
}

func (u *authUser) WebAuthnID() []byte                         { return u.id }
func (u *authUser) WebAuthnName() string                       { return u.name }
func (u *authUser) WebAuthnDisplayName() string                { return u.name }
func (u *authUser) WebAuthnCredentials() []webauthn.Credential { return u.creds }

// newRegistrationUser builds a fresh user handle for enrollment. The WebAuthn
// user id is the app's random user id bytes.
func newRegistrationUser(userID, name string) *authUser {
	return &authUser{id: []byte(userID), name: name}
}

// loadAuthUser builds an authUser from persisted credentials for login.
func loadAuthUser(st *store.Store, userID, name string) (*authUser, error) {
	stored, err := st.CredentialsByUser(userID)
	if err != nil {
		return nil, err
	}
	u := &authUser{id: []byte(userID), name: name}
	for _, c := range stored {
		u.creds = append(u.creds, toWebauthnCredential(c))
	}
	return u, nil
}

func toWebauthnCredential(c store.StoredCredential) webauthn.Credential {
	var transports []protocol.AuthenticatorTransport
	for _, t := range c.Transports {
		transports = append(transports, protocol.AuthenticatorTransport(t))
	}
	return webauthn.Credential{
		ID:              c.CredID,
		PublicKey:       c.PublicKey,
		AttestationType: "none",
		Transport:       transports,
		Flags: webauthn.CredentialFlags{
			BackupEligible: c.BackupEligible,
			BackupState:    c.BackupState,
		},
		Authenticator: webauthn.Authenticator{
			AAGUID:    c.AAGUID,
			SignCount: c.SignCount,
		},
	}
}
