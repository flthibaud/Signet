// Package data is the only layer that speaks SQL. Every table is reached
// through a model — a struct wrapping *sql.DB, aggregated by Models — and no
// query lives outside this package.
//
// The models return sentinel errors rather than statuses, since they know
// nothing about HTTP: ErrRecordNotFound, ErrDuplicateEmail, ErrDuplicateUsername,
// ErrDuplicateFolder and ErrEditConflict are what handlers match with errors.Is
// to pick a response. Anything else is unexpected and becomes a 500.
//
// Ownership is enforced in the WHERE clause, not above it. A user-scoped read or
// write always carries its user_id, so a row belonging to someone else is
// reported as ErrRecordNotFound rather than as a forbidden one — which is also
// why it never confirms that the row exists at all.
package data

import (
	"database/sql"
	"errors"

	"github.com/lib/pq"
)

// ErrRecordNotFound reports that a query matched no row — either because
// nothing has that ID, or because the row belongs to another user and the
// query's user_id predicate excluded it. The two are deliberately
// indistinguishable. Handlers turn it into a 404.
var ErrRecordNotFound = errors.New("record not found")

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
