package storage

import "fmt"

// InvalidKeyError reports an object key that cannot be mapped to a backend path.
type InvalidKeyError struct {
	Key string
}

// Error returns a human-readable invalid key error.
func (e InvalidKeyError) Error() string {
	return fmt.Sprintf("invalid object key %q", e.Key)
}
