//go:build !go1.24

package password

import (
	"crypto/hmac"
	"encoding/binary"
	"errors"
	"hash"
)

// derive is a compatibility implementation for local tooling older than Go 1.24.
// Supported qh8z builds use the standard library crypto/pbkdf2 implementation.
func derive[Hash hash.Hash](h func() Hash, password string, salt []byte, iter, keyLength int) ([]byte, error) {
	if iter <= 0 || keyLength <= 0 {
		return nil, errors.New("invalid PBKDF2 parameters")
	}
	prf := hmac.New(func() hash.Hash { return h() }, []byte(password))
	hLen := prf.Size()
	blocks := (keyLength + hLen - 1) / hLen
	out := make([]byte, 0, blocks*hLen)
	buf := make([]byte, 4)
	for block := 1; block <= blocks; block++ {
		prf.Reset()
		_, _ = prf.Write(salt)
		binary.BigEndian.PutUint32(buf, uint32(block))
		_, _ = prf.Write(buf)
		u := prf.Sum(nil)
		t := append([]byte(nil), u...)
		for i := 1; i < iter; i++ {
			prf.Reset()
			_, _ = prf.Write(u)
			u = prf.Sum(nil)
			for j := range t {
				t[j] ^= u[j]
			}
		}
		out = append(out, t...)
	}
	return out[:keyLength], nil
}
