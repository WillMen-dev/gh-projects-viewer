package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotEnv(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, ".env")
	content := "# comentario\nGPV_A=uno\nexport GPV_B=\"dos con espacios\"\nGPV_C=tres # comentario\nGPV_D=ya-definida\nGPV_E='a=b'\n\n"
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GPV_D", "del-sistema")
	for _, k := range []string{"GPV_A", "GPV_B", "GPV_C", "GPV_E"} {
		os.Unsetenv(k)
		defer os.Unsetenv(k)
	}
	ok, err := LoadDotEnv(p)
	if err != nil || !ok {
		t.Fatalf("carga fallida: %v", err)
	}
	want := map[string]string{"GPV_A": "uno", "GPV_B": "dos con espacios", "GPV_C": "tres", "GPV_D": "del-sistema", "GPV_E": "a=b"}
	for k, v := range want {
		if got := os.Getenv(k); got != v {
			t.Errorf("%s = %q, esperado %q", k, got, v)
		}
	}
	if ok, err := LoadDotEnv(filepath.Join(dir, "no-existe")); ok || err != nil {
		t.Fatal("un archivo inexistente debe ignorarse")
	}
}

func TestLoadValidation(t *testing.T) {
	t.Setenv("GITHUB_MODE", "api")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("STORE", "memory")
	if _, err := Load(); err == nil {
		t.Fatal("api sin token debe fallar")
	}
	t.Setenv("GITHUB_MODE", "demo")
	t.Setenv("BOOTSTRAP_ADMIN_EMAIL", "")
	if _, err := Load(); err != nil {
		t.Fatalf("demo con memoria debe ser válido: %v", err)
	}
	t.Setenv("STORE", "mongo")
	t.Setenv("MONGO_URI", "")
	if _, err := Load(); err == nil {
		t.Fatal("mongo sin MONGO_URI debe fallar")
	}
}
