package password

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

const bcryptCost = 12

// Hash hashes a plaintext password using bcrypt.
func Hash(password string) (string, error) {
	if len(password) < 12 {
		return "", fmt.Errorf("password must be at least 12 characters")
	}
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(bytes), nil
}

// Verify compares a plaintext password with a bcrypt hash.
func Verify(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}
