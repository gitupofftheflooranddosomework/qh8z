//go:build go1.24

package password

import (
	"crypto/pbkdf2"
	"hash"
)

func derive[Hash hash.Hash](h func() Hash, password string, salt []byte, iter, keyLength int) ([]byte, error) {
	return pbkdf2.Key(h, password, salt, iter, keyLength)
}
