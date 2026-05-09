package terraform

import (
	"crypto/rand"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

const (
	TerraformStateFormatRuntime = "runtime-envelope-v2"
)

func randomNonce() ([]byte, error) {
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return nonce, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
