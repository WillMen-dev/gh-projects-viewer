// Package auth contiene el hashing de contraseñas, generación de tokens y limitación de intentos.
package auth

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// PBKDF2-HMAC-SHA256 con 600.000 iteraciones (recomendación OWASP 2023+).
// Se usa la biblioteca estándar de Go (crypto/pbkdf2) para no depender de terceros.
const (
	pbkdf2Iterations = 600_000
	saltLen          = 16
	keyLen           = 32
	MinPasswordLen   = 12
	MaxPasswordLen   = 128
)

var ErrWeakPassword = errors.New("la contraseña debe tener entre 12 y 128 caracteres e incluir letras y números")

// ValidatePassword aplica la política mínima de contraseñas.
func ValidatePassword(pw string) error {
	if len(pw) < MinPasswordLen || len(pw) > MaxPasswordLen {
		return ErrWeakPassword
	}
	var hasLetter, hasDigit bool
	for _, r := range pw {
		switch {
		case unicode.IsLetter(r):
			hasLetter = true
		case unicode.IsDigit(r):
			hasDigit = true
		}
	}
	if !hasLetter || !hasDigit {
		return ErrWeakPassword
	}
	return nil
}

// HashPassword devuelve "pbkdf2_sha256$iter$salt$hash" (base64 sin relleno).
func HashPassword(pw string) (string, error) {
	return hashWithIter(pw, pbkdf2Iterations)
}

func hashWithIter(pw string, iter int) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, pw, salt, iter, keyLen)
	if err != nil {
		return "", err
	}
	enc := base64.RawStdEncoding
	return fmt.Sprintf("pbkdf2_sha256$%d$%s$%s", iter, enc.EncodeToString(salt), enc.EncodeToString(key)), nil
}

// CheckPassword compara en tiempo constante.
func CheckPassword(pw, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2_sha256" {
		return false
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter < 1 {
		return false
	}
	enc := base64.RawStdEncoding
	salt, err1 := enc.DecodeString(parts[2])
	want, err2 := enc.DecodeString(parts[3])
	if err1 != nil || err2 != nil {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, pw, salt, iter, len(want))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}

// dummyHash se usa para igualar el tiempo de respuesta cuando el usuario no existe
// (evita enumerar usuarios midiendo tiempos).
var dummyHash, _ = HashPassword("dummy-password-for-timing-1")

func DummyCheck(pw string) { CheckPassword(pw, dummyHash) }

// NewToken genera un token aleatorio de 32 bytes (256 bits) en hex.
func NewToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// NewID genera un identificador aleatorio de 12 bytes en hex.
func NewID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// HashToken: en la base de datos solo se guarda el SHA-256 del token de sesión,
// así un volcado de la BD no permite secuestrar sesiones.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
