//go:build !windows

package secretstore

import "fmt"

func platformProtect(plain []byte) (string, []byte, error) {
	out := make([]byte, len(plain))
	copy(out, plain)
	return plainScheme, out, nil
}

func platformUnprotect(scheme string, cipher []byte) ([]byte, error) {
	switch scheme {
	case "", plainScheme:
		out := make([]byte, len(cipher))
		copy(out, cipher)
		return out, nil
	default:
		return nil, fmt.Errorf("unsupported secret scheme %q", scheme)
	}
}
