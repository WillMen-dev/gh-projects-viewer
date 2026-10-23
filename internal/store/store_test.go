package store

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

// contractTest verifica el mismo comportamiento en cualquier implementación de Store.
func contractTest(t *testing.T, st Store) {
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	if n, err := st.CountUsers(ctx); err != nil || n != 0 {
		t.Fatalf("base nueva debe estar vacía: %d %v", n, err)
	}
	u := &User{ID: "aaaaaaaaaaaaaaaaaaaaaaaa", Email: "ana@empresa.com", Name: "Ana", PasswordHash: "h",
		Role: RoleViewer, Active: true, CreatedAt: now, UpdatedAt: now}
	if err := st.CreateUser(ctx, u); err != nil {
		t.Fatal(err)
	}
	dup := *u
	dup.ID = "bbbbbbbbbbbbbbbbbbbbbbbb"
	if err := st.CreateUser(ctx, &dup); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("correo duplicado debe dar ErrDuplicate: %v", err)
	}
	got, err := st.GetUserByEmail(ctx, "ana@empresa.com")
	if err != nil || got.ID != u.ID || got.Role != RoleViewer || !got.Active {
		t.Fatalf("GetUserByEmail: %+v %v", got, err)
	}
	if _, err := st.GetUserByID(ctx, "no-existe"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("usuario inexistente debe dar ErrNotFound: %v", err)
	}
	got.Role = RoleAdmin
	got.MustChangePassword = true
	if err := st.UpdateUser(ctx, got); err != nil {
		t.Fatal(err)
	}
	again, _ := st.GetUserByID(ctx, u.ID)
	if again.Role != RoleAdmin || !again.MustChangePassword {
		t.Fatalf("UpdateUser no persistió: %+v", again)
	}
	list, _ := st.ListUsers(ctx)
	if len(list) != 1 {
		t.Fatalf("ListUsers: %d", len(list))
	}

	s := &Session{TokenHash: "hash1", UserID: u.ID, CSRFToken: "c", CreatedAt: now, LastSeenAt: now, ExpiresAt: now.Add(time.Hour)}
	if err := st.CreateSession(ctx, s); err != nil {
		t.Fatal(err)
	}
	gs, err := st.GetSession(ctx, "hash1")
	if err != nil || gs.UserID != u.ID || gs.CSRFToken != "c" {
		t.Fatalf("GetSession: %+v %v", gs, err)
	}
	later := now.Add(10 * time.Minute)
	_ = st.TouchSession(ctx, "hash1", later)
	gs, _ = st.GetSession(ctx, "hash1")
	if !gs.LastSeenAt.Equal(later) {
		t.Fatalf("TouchSession no actualizó: %v", gs.LastSeenAt)
	}
	_ = st.CreateSession(ctx, &Session{TokenHash: "hash2", UserID: u.ID, CreatedAt: now, LastSeenAt: now, ExpiresAt: now.Add(time.Hour)})
	if err := st.DeleteUserSessions(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetSession(ctx, "hash2"); !errors.Is(err, ErrNotFound) {
		t.Fatal("DeleteUserSessions debe borrar todas las sesiones del usuario")
	}

	for i := 0; i < 3; i++ {
		_ = st.AddAudit(ctx, &AuditEntry{ID: string(rune('a'+i)) + "-audit", At: now.Add(time.Duration(i) * time.Second), Action: "login", Success: true})
	}
	entries, err := st.ListAudit(ctx, 2)
	if err != nil || len(entries) != 2 || !entries[0].At.After(entries[1].At) {
		t.Fatalf("ListAudit debe devolver los más recientes primero: %+v %v", entries, err)
	}
	if err := st.Ping(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestMemoryStore(t *testing.T) {
	contractTest(t, NewMemory())
}

// TestMongoStore usa un MongoDB REAL. Se ejecuta solo si se define MONGO_TEST_URI, por ejemplo:
//
//	MONGO_TEST_URI=mongodb://localhost:27017 go test ./internal/store -run Mongo -v
//
// Crea una base temporal y la elimina al terminar.
func TestMongoStore(t *testing.T) {
	uri := os.Getenv("MONGO_TEST_URI")
	if uri == "" {
		t.Skip("MONGO_TEST_URI no definido: se omite la prueba con MongoDB real")
	}
	ctx := context.Background()
	dbName := "gpv_test_" + time.Now().Format("20060102150405")
	m, err := NewMongo(ctx, uri, dbName)
	if err != nil {
		t.Fatalf("no se pudo conectar a MongoDB: %v", err)
	}
	t.Cleanup(func() {
		_ = m.client.Database(dbName).Drop(context.Background())
		_ = m.Close(context.Background())
	})
	contractTest(t, m)
}
