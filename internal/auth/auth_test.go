package auth

import (
	"strings"
	"testing"
	"time"
)

func TestHashAndCheckPassword(t *testing.T) {
	h, err := hashWithIter("ClaveSegura2026", 1000)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "pbkdf2_sha256$1000$") {
		t.Fatalf("formato inesperado: %s", h)
	}
	if !CheckPassword("ClaveSegura2026", h) {
		t.Fatal("la contraseña correcta debería validar")
	}
	if CheckPassword("ClaveSegura2027", h) {
		t.Fatal("una contraseña incorrecta no debería validar")
	}
	if CheckPassword("x", "basura") || CheckPassword("x", "pbkdf2_sha256$abc$$") {
		t.Fatal("hash malformado no debería validar")
	}
	h2, _ := hashWithIter("ClaveSegura2026", 1000)
	if h == h2 {
		t.Fatal("cada hash debe usar una sal distinta")
	}
}

func TestValidatePassword(t *testing.T) {
	cases := map[string]bool{
		"corta1":                 false,
		"soloLetrasMuyLargas":    false,
		"123456789012345":        false,
		"ClaveSegura2026":        true,
		strings.Repeat("a1", 70): false, // > 128
	}
	for pw, ok := range cases {
		if (ValidatePassword(pw) == nil) != ok {
			t.Errorf("ValidatePassword(%q) esperado ok=%v", pw, ok)
		}
	}
}

func TestTokens(t *testing.T) {
	a, _ := NewToken()
	b, _ := NewToken()
	if len(a) != 64 || a == b {
		t.Fatal("tokens deben ser de 64 hex y únicos")
	}
	if HashToken(a) == a || len(HashToken(a)) != 64 {
		t.Fatal("el hash del token debe ser distinto y de 64 hex")
	}
	if len(NewID()) != 24 {
		t.Fatal("ID debe tener 24 hex")
	}
}

func TestLimiter(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	l := NewLimiter(3, time.Minute)
	l.now = func() time.Time { return now }
	for i := 0; i < 3; i++ {
		if l.Blocked("k") {
			t.Fatal("no debería estar bloqueado aún")
		}
		l.Hit("k")
	}
	if !l.Blocked("k") {
		t.Fatal("debería bloquear tras 3 intentos")
	}
	now = now.Add(61 * time.Second)
	if l.Blocked("k") {
		t.Fatal("el bloqueo debe expirar con la ventana")
	}
	if !l.Allow("x") || !l.Allow("x") || !l.Allow("x") || l.Allow("x") {
		t.Fatal("Allow debe permitir exactamente 3")
	}
	l.Reset("x")
	if !l.Allow("x") {
		t.Fatal("Reset debe limpiar")
	}
}
