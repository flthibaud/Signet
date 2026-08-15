package data

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"time"

	"github.com/flthibaud/signet/internal/validator"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// User is an account. Password is never serialized — the struct is written
// straight to the response body by the users and tokens handlers, so the `-`
// tag is what keeps the hash out of the JSON.
type User struct {
	ID        uuid.UUID `json:"id"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	Password  password  `json:"-"`
	CreatedAt time.Time `json:"created_at"`
}

// UserModel gives access to the users table.
type UserModel struct {
	DB *sql.DB
}

// password pairs a plaintext with its bcrypt digest. plaintext is set only on
// the way in, when a caller supplies a new password, and stays nil for a user
// loaded from the database.
type password struct {
	plaintext *string
	hash      []byte
}

// AnonymousUser stands for "no one is signed in". authenticate puts it on every
// request that arrived without a valid token, rather than rejecting the request
// there, so the SPA's guest pages stay reachable and only
// requireAuthenticatedUser decides what a missing session costs. Compare with
// User.IsAnonymous, which is an identity check against this exact pointer.
var AnonymousUser = &User{}

// ErrDuplicateEmail reports that the address is already registered. Handlers
// turn it into a 422 field error rather than a generic failure.
var (
	ErrDuplicateEmail = errors.New("duplicate email")
)

// ErrDuplicateUsername reports that the username is already taken.
var (
	ErrDuplicateUsername = errors.New("duplicate username")
)

// ErrEditConflict reports that an update found no row to write, because it was
// deleted or changed between being read and being saved.
var (
	ErrEditConflict = errors.New("edit conflict")
)

// Unique indexes on users, matched by name when a write is refused.
const (
	userEmailConstraint    = "users_email_key"
	userUsernameConstraint = "users_username_key"
)

// passwordHashCost is the bcrypt work factor. DecoyPasswordCheck derives its own
// cost from this, so the two can never drift apart.
const passwordHashCost = 12

// decoyHash is a bcrypt digest of a value nobody can present, computed once on
// first use. It exists only to be compared against.
var decoyHash = sync.OnceValue(func() []byte {
	hash, err := bcrypt.GenerateFromPassword([]byte("decoy"), passwordHashCost)
	if err != nil {
		// Only possible if the cost is out of range, which is a constant here.
		panic("data: cannot compute the decoy password hash: " + err.Error())
	}
	return hash
})

// DecoyPasswordCheck spends the same CPU a real password comparison would,
// without any user to compare against.
//
// Sign-in looks up the account first, so an unknown email would otherwise skip
// bcrypt entirely and answer in microseconds where a known email takes the
// ~250ms the hash costs. The response bodies are identical, but that gap is not:
// it tells an attacker which addresses have accounts here, one request at a
// time. Calling this on the not-found path makes both answers cost the same.
func DecoyPasswordCheck(plaintextPassword string) {
	// The comparison always fails; only its duration matters.
	_ = bcrypt.CompareHashAndPassword(decoyHash(), []byte(plaintextPassword))
}

// IsAnonymous reports whether u is the AnonymousUser sentinel, and so whether
// the request it came from is unauthenticated.
func (u *User) IsAnonymous() bool {
	return u == AnonymousUser
}

// Set hashes plaintextPassword and stores both halves on p. bcrypt caps its
// input at 72 bytes, which is why ValidatePasswordPlaintext bounds the length
// in bytes rather than characters.
func (p *password) Set(plaintextPassword string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(plaintextPassword), passwordHashCost)
	if err != nil {
		return err
	}
	p.plaintext = &plaintextPassword
	p.hash = hash

	return nil
}

// Matches reports whether plaintextPassword hashes to p's stored digest. A
// wrong password is (false, nil), not an error — only a malformed or unreadable
// hash produces one.
func (p *password) Matches(plaintextPassword string) (bool, error) {
	err := bcrypt.CompareHashAndPassword(p.hash, []byte(plaintextPassword))
	if err != nil {
		switch {
		case errors.Is(err, bcrypt.ErrMismatchedHashAndPassword):
			return false, nil
		default:
			return false, err
		}
	}
	return true, nil
}

// ValidateEmail records the problems with an email address on v. Split out from
// ValidateUser because sign-in validates an address without having a User.
func ValidateEmail(v *validator.Validator, email string) {
	v.Check(email != "", "email", "must be provided")
	v.Check(validator.Matches(email, validator.EmailRX), "email", "must be a valid email address")
}

// ValidatePasswordPlaintext records the problems with a candidate password on
// v. The bounds are in bytes, not characters, because bcrypt truncates past 72
// bytes: accepting a longer one would silently ignore its tail.
func ValidatePasswordPlaintext(v *validator.Validator, password string) {
	v.Check(password != "", "password", "must be provided")
	v.Check(len(password) >= 8, "password", "must be at least 8 bytes long")
	v.Check(len(password) <= 72, "password", "must not be more than 72 bytes long")
}

// ValidateUser records every problem with user on v, checking the password only
// when one was supplied — an update that leaves it alone carries no plaintext.
//
// A user reaching here with no hash at all is a caller that forgot to call
// Set, not bad input from a client, so it panics rather than adding to v: a
// validation error would report it to the user as though they could fix it,
// and letting it through would store an account nobody can sign in to.
func ValidateUser(v *validator.Validator, user *User) {
	v.Check(user.Username != "", "username", "must be provided")
	v.Check(len(user.Username) <= 500, "username", "must not be more than 500 bytes long")

	ValidateEmail(v, user.Email)

	if user.Password.plaintext != nil {
		ValidatePasswordPlaintext(v, *user.Password.plaintext)
	}

	if user.Password.hash == nil {
		panic("missing password hash for user")
	}
}

// GetByEmail returns the account registered under email, with its password hash
// loaded so the caller can verify a sign-in. Returns ErrRecordNotFound if there
// is no such account — a path that must still spend bcrypt's time, see
// DecoyPasswordCheck.
func (m UserModel) GetByEmail(email string) (*User, error) {
	query := `
		SELECT id, created_at, username, email, password_hash
		FROM users
		WHERE email = $1`

	var user User

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)

	defer cancel()

	err := m.DB.QueryRowContext(ctx, query, email).Scan(
		&user.ID,
		&user.CreatedAt,
		&user.Username,
		&user.Email,
		&user.Password.hash,
	)

	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return nil, ErrRecordNotFound
		default:
			return nil, err
		}
	}

	return &user, nil
}

// GetForToken returns the user a valid token belongs to, along with that
// token's expiry — the caller needs it to decide whether the session is due to
// be slid forward, and reading it here keeps that decision free of a second
// round trip.
func (m UserModel) GetForToken(tokenScope, tokenPlaintext string) (*User, time.Time, error) {
	tokenHash := HashTokenPlaintext(tokenPlaintext)

	// Set up the SQL query.
	query := `
		SELECT users.id, users.created_at, users.username, users.email, users.password_hash, tokens.expiry
		FROM users
		INNER JOIN tokens
		ON users.id = tokens.user_id
		WHERE tokens.hash = $1
		AND tokens.scope = $2
		AND tokens.expiry > $3`

	// Pass the current time as the value to check the token expiry against, so an
	// expired token reads as no user at all.
	args := []any{tokenHash, tokenScope, time.Now()}

	var user User
	var expiry time.Time

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Execute the query, scanning the return values into a User struct. If no matching
	// record is found we return an ErrRecordNotFound error.
	err := m.DB.QueryRowContext(ctx, query, args...).Scan(
		&user.ID,
		&user.CreatedAt,
		&user.Username,
		&user.Email,
		&user.Password.hash,
		&expiry,
	)
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return nil, time.Time{}, ErrRecordNotFound
		default:
			return nil, time.Time{}, err
		}
	}

	return &user, expiry, nil
}

// Get returns the account with the given ID. Unlike GetByEmail it does not load
// the password hash, since no caller of this needs to verify a password.
// Returns ErrRecordNotFound if no account has that ID.
func (m UserModel) Get(id uuid.UUID) (*User, error) {
	query := `
		SELECT id, username, email, created_at
		FROM users
		WHERE id = $1`

	var user User

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)

	defer cancel()

	err := m.DB.QueryRowContext(ctx, query, id).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.CreatedAt,
	)

	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return nil, ErrRecordNotFound
		default:
			return nil, err
		}
	}

	return &user, nil
}

// Insert creates the account and fills in user's generated ID and CreatedAt.
// Returns ErrDuplicateEmail or ErrDuplicateUsername when the address or name is
// taken; the two are told apart by which unique index the write breached, so
// the handler can attach the message to the right field.
func (m UserModel) Insert(user *User) error {
	query := `
		INSERT INTO users (username, email, password_hash)
		VALUES ($1, $2, $3)
		RETURNING id, created_at`

	args := []any{user.Username, user.Email, user.Password.hash}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)

	defer cancel()

	err := m.DB.QueryRowContext(ctx, query, args...).Scan(&user.ID, &user.CreatedAt)
	if err != nil {
		switch {
		case isUniqueViolation(err, userEmailConstraint):
			return ErrDuplicateEmail
		case isUniqueViolation(err, userUsernameConstraint):
			return ErrDuplicateUsername
		default:
			return err
		}
	}
	return nil
}

// Update writes user's username, email and password hash back to its row.
// Returns ErrDuplicateEmail or ErrDuplicateUsername on a collision, and
// ErrEditConflict if the row no longer exists.
func (m UserModel) Update(user *User) error {
	query := `
		UPDATE users
		SET username = $1, email = $2, password_hash = $3
		WHERE id = $4
		RETURNING id`

	args := []any{
		user.Username,
		user.Email,
		user.Password.hash,
		user.ID,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)

	defer cancel()

	err := m.DB.QueryRowContext(ctx, query, args...).Scan(&user.ID)

	if err != nil {
		switch {
		case isUniqueViolation(err, userEmailConstraint):
			return ErrDuplicateEmail
		case isUniqueViolation(err, userUsernameConstraint):
			return ErrDuplicateUsername
		case errors.Is(err, sql.ErrNoRows):
			return ErrEditConflict
		default:
			return err
		}
	}

	return nil
}
