package data

import (
	"errors"
	"testing"

	"github.com/google/uuid"
)

// TestUserInsertDuplicates pins that a duplicate email or username comes back as
// its own sentinel error rather than as a bare driver error.
//
// The detection matches on the constraint name, so this test is what catches a
// renamed index: without it, a mismatch would quietly turn the 422 the
// registration handler expects into a 500, and nothing else would notice.
func TestUserInsertDuplicates(t *testing.T) {
	db := testDB(t)
	model := UserModel{DB: db}

	existing := seedUser(t, db)

	var username, email string
	err := db.QueryRow(`SELECT username, email FROM users WHERE id = $1`, existing).Scan(&username, &email)
	if err != nil {
		t.Fatalf("reading the seeded user: %v", err)
	}

	newUser := func(username, email string) *User {
		user := &User{Username: username, Email: email}
		user.Password.hash = []byte("x")
		return user
	}

	t.Run("duplicate email", func(t *testing.T) {
		err := model.Insert(newUser("fresh-"+uuid.NewString(), email))
		if !errors.Is(err, ErrDuplicateEmail) {
			t.Errorf("got %v, want ErrDuplicateEmail", err)
		}
	})

	t.Run("duplicate username", func(t *testing.T) {
		err := model.Insert(newUser(username, "fresh-"+uuid.NewString()+"@example.test"))
		if !errors.Is(err, ErrDuplicateUsername) {
			t.Errorf("got %v, want ErrDuplicateUsername", err)
		}
	})

	t.Run("no conflict", func(t *testing.T) {
		id := uuid.NewString()
		user := newUser("fresh-"+id, "fresh-"+id+"@example.test")
		if err := model.Insert(user); err != nil {
			t.Fatalf("inserting a distinct user: %v", err)
		}
		t.Cleanup(func() {
			exec(t, db, `DELETE FROM users WHERE id = $1`, user.ID)
		})
	})
}

// TestUserHasAny checks the query behind the bootstrap exception to
// REGISTRATION_ENABLED against a real table. The empty case is not exercised
// here — the fixtures never truncate — but that half is plain SQL semantics;
// what can actually break is the table name or the Scan, which this covers.
func TestUserHasAny(t *testing.T) {
	db := testDB(t)
	model := UserModel{DB: db}

	seedUser(t, db)

	hasUsers, err := model.HasAny()
	if err != nil {
		t.Fatalf("HasAny: %v", err)
	}
	if !hasUsers {
		t.Error("got false with a seeded user, want true")
	}
}
