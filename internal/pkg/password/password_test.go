package password

import (
	"strings"
	"testing"
)

func TestHash_ValidPassword(t *testing.T) {
	hash, err := Hash("ValidPassword123!")
	if err != nil {
		t.Fatalf("Hash() returned error: %v", err)
	}
	if hash == "" {
		t.Fatal("Hash() returned empty string")
	}
	if !strings.HasPrefix(hash, "$2a$") && !strings.HasPrefix(hash, "$2b$") {
		t.Fatalf("Hash() doesn't look like bcrypt: %s", hash)
	}
}

func TestHash_TooShort(t *testing.T) {
	_, err := Hash("short")
	if err == nil {
		t.Fatal("Hash() should return error for password < 12 chars")
	}
}

func TestHash_Empty(t *testing.T) {
	_, err := Hash("")
	if err == nil {
		t.Fatal("Hash() should return error for empty password")
	}
}

func TestVerify_CorrectPassword(t *testing.T) {
	password := "CorrectPassword123!"
	hash, err := Hash(password)
	if err != nil {
		t.Fatalf("Hash() failed: %v", err)
	}
	if !Verify(password, hash) {
		t.Fatal("Verify() should return true for correct password")
	}
}

func TestVerify_WrongPassword(t *testing.T) {
	password := "CorrectPassword123!"
	hash, err := Hash(password)
	if err != nil {
		t.Fatalf("Hash() failed: %v", err)
	}
	if Verify("WrongPassword123!", hash) {
		t.Fatal("Verify() should return false for wrong password")
	}
}

func TestVerify_EmptyPassword(t *testing.T) {
	hash, err := Hash("ValidPassword123!")
	if err != nil {
		t.Fatalf("Hash() failed: %v", err)
	}
	if Verify("", hash) {
		t.Fatal("Verify() should return false for empty password")
	}
}

func TestVerify_InvalidHash(t *testing.T) {
	if Verify("password", "invalid-hash") {
		t.Fatal("Verify() should return false for invalid hash")
	}
}

func TestHash_Uniqueness(t *testing.T) {
	password := "SamePassword123!"
	hash1, err := Hash(password)
	if err != nil {
		t.Fatalf("Hash() failed: %v", err)
	}
	hash2, err := Hash(password)
	if err != nil {
		t.Fatalf("Hash() failed: %v", err)
	}
	// bcrypt generates different salts, so hashes should differ
	if hash1 == hash2 {
		t.Fatal("Hash() should produce different outputs due to random salt")
	}
	// But both should verify correctly
	if !Verify(password, hash1) {
		t.Fatal("Verify() should work for hash1")
	}
	if !Verify(password, hash2) {
		t.Fatal("Verify() should work for hash2")
	}
}

func TestHash_ExactMinLength(t *testing.T) {
	// Exactly 12 characters
	password := "Exactly12Char!"
	hash, err := Hash(password)
	if err != nil {
		t.Fatalf("Hash() returned error for exactly 12 chars: %v", err)
	}
	if !Verify(password, hash) {
		t.Fatal("Verify() should work for exactly 12 char password")
	}
}

func TestHash_LongPassword(t *testing.T) {
	// 50 characters (bcrypt has a 72-byte limit, stay under it)
	password := strings.Repeat("A", 50)
	hash, err := Hash(password)
	if err != nil {
		t.Fatalf("Hash() returned error for long password: %v", err)
	}
	if !Verify(password, hash) {
		t.Fatal("Verify() should work for long password")
	}
}

func TestHash_UnicodePassword(t *testing.T) {
	password := "مرحبا العالم123!"
	hash, err := Hash(password)
	if err != nil {
		t.Fatalf("Hash() returned error for unicode: %v", err)
	}
	if !Verify(password, hash) {
		t.Fatal("Verify() should work for unicode password")
	}
}
