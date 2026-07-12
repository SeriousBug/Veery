package auth

import (
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/SeriousBug/Veery/internal/store"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

// Manager runs the WebAuthn ceremonies against the store.
type Manager struct {
	wa    *webauthn.WebAuthn
	st    *store.Store
	mu    sync.Mutex
	ceremonies map[string]*ceremony // keyed by temp ceremony id
}

type ceremony struct {
	data    *webauthn.SessionData
	userID  string // set for registration
	name    string
	isAdmin bool
	expires time.Time
}

// Config for the relying party.
type Config struct {
	RPID    string // e.g. "veery.example.com"
	Origin  string // e.g. "https://veery.example.com"
	RPName  string
}

// NewManager constructs a WebAuthn manager.
func NewManager(st *store.Store, cfg Config) (*Manager, error) {
	name := cfg.RPName
	if name == "" {
		name = "Veery"
	}
	wa, err := webauthn.New(&webauthn.Config{
		RPID:          cfg.RPID,
		RPDisplayName: name,
		RPOrigins:     []string{cfg.Origin},
	})
	if err != nil {
		return nil, fmt.Errorf("webauthn config: %w", err)
	}
	m := &Manager{wa: wa, st: st, ceremonies: map[string]*ceremony{}}
	return m, nil
}

func (m *Manager) put(c *ceremony) string {
	id := randToken(16)
	c.expires = time.Now().Add(5 * time.Minute)
	m.mu.Lock()
	m.ceremonies[id] = c
	m.gcLocked()
	m.mu.Unlock()
	return id
}

func (m *Manager) take(id string) (*ceremony, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.ceremonies[id]
	if ok {
		delete(m.ceremonies, id)
	}
	if ok && time.Now().After(c.expires) {
		return nil, false
	}
	return c, ok
}

func (m *Manager) gcLocked() {
	now := time.Now()
	for k, v := range m.ceremonies {
		if now.After(v.expires) {
			delete(m.ceremonies, k)
		}
	}
}

// BeginRegistration validates the invite and starts enrollment, returning the
// creation options JSON and a ceremony id to round-trip.
func (m *Manager) BeginRegistration(inviteToken, name string) (*protocol.CredentialCreation, string, bool, error) {
	inv, err := m.st.GetInvite(inviteToken)
	if err != nil {
		return nil, "", false, ErrInvalidInvite
	}
	if inv.UsedAt != 0 || inv.ExpiresAt < time.Now().Unix() {
		return nil, "", false, ErrInvalidInvite
	}
	if name == "" {
		name = "user"
	}
	userID := randToken(16)
	user := newRegistrationUser(userID, name)

	sel := protocol.AuthenticatorSelection{
		ResidentKey:      protocol.ResidentKeyRequirementRequired,
		UserVerification: protocol.VerificationPreferred,
	}
	opts, sessionData, err := m.wa.BeginRegistration(user,
		webauthn.WithAuthenticatorSelection(sel),
		webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementRequired),
	)
	if err != nil {
		return nil, "", false, err
	}
	cid := m.put(&ceremony{data: sessionData, userID: userID, name: name, isAdmin: inv.IsAdmin})
	return opts, cid, inv.IsAdmin, nil
}

// FinishRegistration completes enrollment: verifies the attestation, creates the
// user, stores the credential, and consumes the invite. Returns the new user id.
func (m *Manager) FinishRegistration(ceremonyID, inviteToken string, r *http.Request) (string, error) {
	cer, ok := m.take(ceremonyID)
	if !ok {
		return "", ErrCeremonyExpired
	}
	parsed, err := protocol.ParseCredentialCreationResponseBody(r.Body)
	if err != nil {
		return "", err
	}
	user := newRegistrationUser(cer.userID, cer.name)
	cred, err := m.wa.CreateCredential(user, *cer.data, parsed)
	if err != nil {
		return "", err
	}
	// Consume the invite atomically before creating the user.
	if err := m.st.ConsumeInvite(inviteToken); err != nil {
		return "", ErrInvalidInvite
	}
	if _, err := m.st.CreateUser(cer.userID, cer.name, cer.isAdmin); err != nil {
		return "", err
	}
	var transports []string
	for _, t := range cred.Transport {
		transports = append(transports, string(t))
	}
	err = m.st.AddCredential(store.StoredCredential{
		ID:         randToken(12),
		UserID:     cer.userID,
		CredID:     cred.ID,
		PublicKey:  cred.PublicKey,
		SignCount:  cred.Authenticator.SignCount,
		Transports: transports,
		AAGUID:     cred.Authenticator.AAGUID,
		Name:       "Passkey",
		CreatedAt:  time.Now().Unix(),
	})
	if err != nil {
		return "", err
	}
	return cer.userID, nil
}

// BeginAddDevice starts enrolling an ADDITIONAL passkey for an already
// authenticated user (recovery / second device). It reuses the user's existing
// WebAuthn id and excludes their current credentials so the same authenticator
// isn't registered twice. No invite is involved.
func (m *Manager) BeginAddDevice(userID, userName string, existingCreds []store.StoredCredential) (*protocol.CredentialCreation, string, error) {
	user := &authUser{id: []byte(userID), name: userName}
	for _, c := range existingCreds {
		user.creds = append(user.creds, toWebauthnCredential(c))
	}
	exclusions := make([]protocol.CredentialDescriptor, 0, len(user.creds))
	for i := range user.creds {
		exclusions = append(exclusions, user.creds[i].Descriptor())
	}

	sel := protocol.AuthenticatorSelection{
		ResidentKey:      protocol.ResidentKeyRequirementRequired,
		UserVerification: protocol.VerificationPreferred,
	}
	opts, sessionData, err := m.wa.BeginRegistration(user,
		webauthn.WithAuthenticatorSelection(sel),
		webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementRequired),
		webauthn.WithExclusions(exclusions),
	)
	if err != nil {
		return nil, "", err
	}
	cid := m.put(&ceremony{data: sessionData, userID: userID, name: userName})
	return opts, cid, nil
}

// FinishAddDevice completes an add-device ceremony, storing the new credential
// against the existing user. It creates no user and consumes no invite.
func (m *Manager) FinishAddDevice(ceremonyID, userID string, r *http.Request) error {
	cer, ok := m.take(ceremonyID)
	if !ok {
		return ErrCeremonyExpired
	}
	if cer.userID != userID {
		return errors.New("ceremony does not match session user")
	}
	parsed, err := protocol.ParseCredentialCreationResponseBody(r.Body)
	if err != nil {
		return err
	}
	user := newRegistrationUser(cer.userID, cer.name)
	cred, err := m.wa.CreateCredential(user, *cer.data, parsed)
	if err != nil {
		return err
	}
	var transports []string
	for _, t := range cred.Transport {
		transports = append(transports, string(t))
	}
	return m.st.AddCredential(store.StoredCredential{
		ID:         randToken(12),
		UserID:     cer.userID,
		CredID:     cred.ID,
		PublicKey:  cred.PublicKey,
		SignCount:  cred.Authenticator.SignCount,
		Transports: transports,
		AAGUID:     cred.Authenticator.AAGUID,
		Name:       "Passkey",
		CreatedAt:  time.Now().Unix(),
	})
}

// BeginLogin starts a usernameless (discoverable) login.
func (m *Manager) BeginLogin() (*protocol.CredentialAssertion, string, error) {
	opts, sessionData, err := m.wa.BeginDiscoverableLogin()
	if err != nil {
		return nil, "", err
	}
	cid := m.put(&ceremony{data: sessionData})
	return opts, cid, nil
}

// FinishLogin verifies a discoverable assertion and returns the authenticated
// user id.
func (m *Manager) FinishLogin(ceremonyID string, r *http.Request) (string, error) {
	cer, ok := m.take(ceremonyID)
	if !ok {
		return "", ErrCeremonyExpired
	}
	parsed, err := protocol.ParseCredentialRequestResponseBody(r.Body)
	if err != nil {
		return "", err
	}
	var loggedInUserID string
	handler := func(rawID, userHandle []byte) (webauthn.User, error) {
		sc, err := m.st.CredentialByCredID(rawID)
		if err != nil {
			return nil, errors.New("unknown credential")
		}
		user, err := m.st.GetUser(sc.UserID)
		if err != nil {
			return nil, err
		}
		loggedInUserID = sc.UserID
		return loadAuthUser(m.st, user.ID, user.Name)
	}
	cred, err := m.wa.ValidateDiscoverableLogin(handler, *cer.data, parsed)
	if err != nil {
		return "", err
	}
	// Persist the updated sign counter to detect cloned authenticators.
	if err := m.st.UpdateSignCount(cred.ID, cred.Authenticator.SignCount); err != nil {
		return "", err
	}
	return loggedInUserID, nil
}

// Errors surfaced to handlers.
var (
	ErrInvalidInvite   = errors.New("invalid or expired invite")
	ErrCeremonyExpired = errors.New("ceremony expired, try again")
)
