package data

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base32"
	"errors"
	"fmt"
	"time"

	"github.com/flthibaud/signet/internal/validator"
	"github.com/google/uuid"
)

// Token scopes. The scope is part of every lookup, so a token minted for one
// purpose cannot be replayed against another.
const (
	ScopeActivation     = "activation"
	ScopeAuthentication = "authentication"
)

// tokenEntropyBytes is how much CSPRNG output backs a token: 128 bits, far
// beyond guessing range for a value that is also rate limited and hashed at
// rest.
const tokenEntropyBytes = 16

// tokenEncoding is base32 without padding, so a token is a bare alphanumeric
// string that survives being pasted into a header or a cookie.
var tokenEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// tokenPlaintextLength is derived from the encoding rather than written down,
// so changing tokenEntropyBytes or the encoding can't leave the validator
// checking a stale literal.
var tokenPlaintextLength = tokenEncoding.EncodedLen(tokenEntropyBytes)

// Token is an authentication credential. Plaintext exists only in the response
// that issues the token — only Hash is stored, so a dump of the tokens table
// yields nothing that can be presented as a credential.
type Token struct {
	Plaintext string    `json:"token"`
	Hash      []byte    `json:"-"`
	UserID    uuid.UUID `json:"-"`
	Expiry    time.Time `json:"expiry"`
	Scope     string    `json:"-"`
}

func generateToken(userID uuid.UUID, ttl time.Duration, scope string) (*Token, error) {
	// Create a Token instance containing the user ID, expiry, and scope information.
	// Notice that we add the provided ttl (time-to-live) duration parameter to the
	// current time to get the expiry time?
	token := &Token{
		UserID: userID,
		Expiry: time.Now().Add(ttl),
		Scope:  scope,
	}
	// Initialize a zero-valued byte slice with a length of 16 bytes.
	randomBytes := make([]byte, tokenEntropyBytes)
	// Use the Read() function from the crypto/rand package to fill the byte slice with
	// random bytes from your operating system's CSPRNG. This will return an error if
	// the CSPRNG fails to function correctly.
	_, err := rand.Read(randomBytes)
	if err != nil {
		return nil, err
	}
	// Encode the byte slice to a base-32-encoded string and assign it to the token
	// Plaintext field. This will be the token string that we send to the user in their
	// welcome email. They will look similar to this:
	//
	// # Y3QMGX3PJ3WLRL2YRTQGQ6KRHU
	//
	// Note that by default base-32 strings may be padded at the end with the =
	// character. We don't need this padding character for the purpose of our tokens, so
	// we use the WithPadding(base32.NoPadding) method in the line below to omit them.
	token.Plaintext = tokenEncoding.EncodeToString(randomBytes)
	// Store only the SHA-256 hash of the plaintext, so a dump of the tokens table
	// yields nothing that can be replayed.
	token.Hash = HashTokenPlaintext(token.Plaintext)
	return token, nil
}

// HashTokenPlaintext returns the value stored in the tokens table for a given
// plaintext token. Callers that hold a plaintext (the authenticate middleware,
// GetForToken) go through this rather than hashing inline, so there is one
// definition of what a token's identity is.
func HashTokenPlaintext(tokenPlaintext string) []byte {
	hash := sha256.Sum256([]byte(tokenPlaintext))
	return hash[:]
}

// ValidateTokenPlaintext checks the token is present and the right shape before
// it costs a database round trip.
func ValidateTokenPlaintext(v *validator.Validator, tokenPlaintext string) {
	v.Check(tokenPlaintext != "", "token", "must be provided")
	v.Check(len(tokenPlaintext) == tokenPlaintextLength, "token", fmt.Sprintf("must be %d characters long", tokenPlaintextLength))
}

// TokenModel gives access to the tokens table.
type TokenModel struct {
	DB *sql.DB
}

// New mints a token for the user and stores it, returning it with its Plaintext
// set — the only moment that value exists, since only the hash is persisted.
func (m TokenModel) New(userID uuid.UUID, ttl time.Duration, scope string) (*Token, error) {
	token, err := generateToken(userID, ttl, scope)
	if err != nil {
		return nil, err
	}
	err = m.Insert(token)
	return token, err
}

// Insert stores a token. Most callers want New, which generates one first.
func (m TokenModel) Insert(token *Token) error {
	query := `
		INSERT INTO tokens (hash, user_id, expiry, scope)
		VALUES ($1, $2, $3, $4)`

	args := []any{token.Hash, token.UserID, token.Expiry, token.Scope}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := m.DB.ExecContext(ctx, query, args...)
	return err
}

// DeleteAllForUser deletes every token a user holds in a scope. This is
// the "sign out everywhere" hammer — an ordinary logout uses DeleteByHash so it
// only ends the session that asked.
func (m TokenModel) DeleteAllForUser(scope string, userID uuid.UUID) error {
	query := `
		DELETE FROM tokens
		WHERE scope = $1 AND user_id = $2`

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := m.DB.ExecContext(ctx, query, scope, userID)
	return err
}

// DeleteByHash deletes a single token, identified by the hash of the plaintext
// the client presented. Deleting an already-expired or unknown token is not an
// error: the caller only cares that it is gone afterwards.
func (m TokenModel) DeleteByHash(scope string, hash []byte) error {
	query := `
		DELETE FROM tokens
		WHERE scope = $1 AND hash = $2`

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := m.DB.ExecContext(ctx, query, scope, hash)
	return err
}

// Refresh slides a still-valid token's expiry to now+ttl and returns the new
// expiry. The expiry > now() guard means a token that lapsed between the
// authentication read and this write stays lapsed rather than being resurrected;
// that case surfaces as ErrRecordNotFound.
func (m TokenModel) Refresh(scope string, hash []byte, ttl time.Duration) (time.Time, error) {
	query := `
		UPDATE tokens
		SET expiry = $1
		WHERE scope = $2 AND hash = $3 AND expiry > $4
		RETURNING expiry`

	now := time.Now()
	newExpiry := now.Add(ttl)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var expiry time.Time
	err := m.DB.QueryRowContext(ctx, query, newExpiry, scope, hash, now).Scan(&expiry)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return time.Time{}, ErrRecordNotFound
		}
		return time.Time{}, err
	}

	return expiry, nil
}

// DeleteExpired removes every lapsed token, whatever the scope, and reports how
// many went. Nothing else prunes the table: expired rows are ignored by
// GetForToken but would otherwise accumulate for the life of the install.
func (m TokenModel) DeleteExpired(ctx context.Context) (int64, error) {
	query := `
		DELETE FROM tokens
		WHERE expiry < now()`

	result, err := m.DB.ExecContext(ctx, query)
	if err != nil {
		return 0, err
	}

	return result.RowsAffected()
}
