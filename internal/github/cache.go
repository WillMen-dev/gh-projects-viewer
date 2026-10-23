package github

import (
	"context"
	"sync"
	"time"
)

// CachedProvider envuelve un Provider y guarda las respuestas en memoria durante un TTL.
// Reduce el consumo del límite de la API de GitHub y mejora el tiempo de respuesta.
// No persiste datos de GitHub en la base de datos (GitHub sigue siendo la fuente de verdad).
type CachedProvider struct {
	inner Provider
	ttl   time.Duration
	now   func() time.Time

	mu       sync.Mutex
	projects *entry[[]Project]
	details  map[string]*entry[*ProjectDetail]
}

type entry[T any] struct {
	value   T
	expires time.Time
}

func NewCachedProvider(inner Provider, ttl time.Duration) *CachedProvider {
	return &CachedProvider{inner: inner, ttl: ttl, now: time.Now, details: map[string]*entry[*ProjectDetail]{}}
}

func (c *CachedProvider) ListProjects(ctx context.Context) ([]Project, error) {
	c.mu.Lock()
	if e := c.projects; e != nil && c.now().Before(e.expires) {
		c.mu.Unlock()
		return e.value, nil
	}
	c.mu.Unlock()

	v, err := c.inner.ListProjects(ctx)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.projects = &entry[[]Project]{value: v, expires: c.now().Add(c.ttl)}
	c.mu.Unlock()
	return v, nil
}

func (c *CachedProvider) GetProject(ctx context.Context, id string) (*ProjectDetail, error) {
	c.mu.Lock()
	if e, ok := c.details[id]; ok && c.now().Before(e.expires) {
		c.mu.Unlock()
		return e.value, nil
	}
	c.mu.Unlock()

	v, err := c.inner.GetProject(ctx, id)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	// Limpieza simple para evitar crecimiento ilimitado.
	if len(c.details) > 200 {
		c.details = map[string]*entry[*ProjectDetail]{}
	}
	c.details[id] = &entry[*ProjectDetail]{value: v, expires: c.now().Add(c.ttl)}
	c.mu.Unlock()
	return v, nil
}

// Invalidate borra la caché de un proyecto (y la lista), forzando una nueva consulta.
func (c *CachedProvider) Invalidate(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.details, id)
	c.projects = nil
}
