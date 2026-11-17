// Package data is the only layer that speaks SQL. Every table is reached
// through a model — a struct wrapping *sql.DB, aggregated by Models — and no
// query lives outside this package.
//
// The models return sentinel errors rather than statuses, since they know
// nothing about HTTP: ErrRecordNotFound, ErrDuplicateEmail, ErrDuplicateUsername,
// ErrDuplicateFolder and ErrEditConflict are what handlers match with errors.Is
// to pick a response. Anything else is unexpected and becomes a 500.
// ErrUnstorable is the one matched by the feed sync rather than by a handler.
//
// Ownership is enforced in the WHERE clause, not above it. A user-scoped read or
// write always carries its user_id, so a row belonging to someone else is
// reported as ErrRecordNotFound rather than as a forbidden one — which is also
// why it never confirms that the row exists at all.
package data

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/lib/pq"
)

// ErrRecordNotFound reports that a query matched no row — either because
// nothing has that ID, or because the row belongs to another user and the
// query's user_id predicate excluded it. The two are deliberately
// indistinguishable. Handlers turn it into a 404.
var ErrRecordNotFound = errors.New("record not found")

// ErrUnstorable reports a write PostgreSQL refused because of the data itself,
// a value it cannot hold or a constraint it breaks, so retrying the same write
// gets the same refusal. The feed sync matches it to move past an item that can
// never be stored instead of re-downloading the feed every sync to retry it.
// The driver's error stays wrapped inside.
var ErrUnstorable = errors.New("rejected by the database")

// foreignKeyViolation is the SQLSTATE Postgres reports for a reference to a
// row that does not exist.
const foreignKeyViolation = "23503"

// asUnstorable wraps err in ErrUnstorable when Postgres rejected the data
// itself: SQLSTATE class 22 (data exception) or 23 (integrity constraint
// violation). Anything else, a timeout, a dropped connection or a cancelled
// query, may succeed next time and is returned unchanged.
//
// A foreign key violation is the exception within class 23: on the tables the
// feed sync writes, it means a user or feed was deleted while the sync ran,
// and the next sync will not run into it again.
func asUnstorable(err error) error {
	var pqErr *pq.Error
	if !errors.As(err, &pqErr) || pqErr.Code == foreignKeyViolation {
		return err
	}
	switch pqErr.Code.Class() {
	case "22", "23":
		return fmt.Errorf("%w: %w", ErrUnstorable, err)
	}
	return err
}

// uniqueViolation is the SQLSTATE class Postgres reports for a unique
// constraint breach.
const uniqueViolation = "23505"

// isUniqueViolation reports whether err is Postgres refusing a write because it
// would duplicate the named unique constraint.
//
// It matches on the SQLSTATE code and the constraint name rather than on the
// driver's rendered message. Comparing message text ties the check to lib/pq's
// formatting and to the server's locale, and a silent mismatch is worse than it
// looks: the duplicate stops being reported as a field error and surfaces as a
// 500 instead.
func isUniqueViolation(err error, constraint string) bool {
	var pqErr *pq.Error
	if !errors.As(err, &pqErr) {
		return false
	}

	return pqErr.Code == uniqueViolation && pqErr.Constraint == constraint
}

// checkOneRow turns "the WHERE matched nothing" into ErrRecordNotFound. Every
// user-scoped UPDATE/DELETE goes through it, so a row belonging to another user
// is reported as missing rather than as a silent success.
func checkOneRow(result sql.Result) error {
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrRecordNotFound
	}

	return nil
}
