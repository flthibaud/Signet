package data

import (
	"database/sql"
	"errors"

	"github.com/lib/pq"
)

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
