package data

import (
	"database/sql"
	"errors"
)

var ErrRecordNotFound = errors.New("record not found")

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
