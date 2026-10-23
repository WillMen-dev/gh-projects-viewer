package auth

import (
	"sync"
	"time"
)

// Limiter cuenta eventos por clave en una ventana deslizante simple.
// Se usa para bloquear temporalmente intentos de login fallidos (fuerza bruta)
// y para limitar la frecuencia de acciones costosas (refrescar desde GitHub).
type Limiter struct {
	max    int
	window time.Duration
	now    func() time.Time

	mu   sync.Mutex
	hits map[string][]time.Time
}

func NewLimiter(max int, window time.Duration) *Limiter {
	return &Limiter{max: max, window: window, now: time.Now, hits: map[string][]time.Time{}}
}

func (l *Limiter) prune(key string, now time.Time) []time.Time {
	ts := l.hits[key]
	cut := 0
	for cut < len(ts) && now.Sub(ts[cut]) > l.window {
		cut++
	}
	ts = ts[cut:]
	if len(ts) == 0 {
		delete(l.hits, key)
	} else {
		l.hits[key] = ts
	}
	return ts
}

// Blocked indica si la clave superó el máximo dentro de la ventana.
func (l *Limiter) Blocked(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.prune(key, l.now())) >= l.max
}

// Hit registra un evento.
func (l *Limiter) Hit(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.hits[key] = append(l.prune(key, now), now)
	if len(l.hits) > 50_000 { // protección de memoria
		l.hits = map[string][]time.Time{}
	}
}

// Allow registra y devuelve true si aún no se supera el límite.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	ts := l.prune(key, now)
	if len(ts) >= l.max {
		return false
	}
	l.hits[key] = append(ts, now)
	return true
}

// Reset borra el contador (p. ej. tras un login exitoso).
func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.hits, key)
}
