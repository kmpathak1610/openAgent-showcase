package integration

import (
	"context"
)

// Provider abstraction — do not hard-code providers into agent logic
type Provider interface {
	Name() string // linkedin, x, instagram, facebook, email, calendar, google_drive, slack, http_api, web_search
	Execute(ctx context.Context, toolName string, input map[string]any, credentials map[string]any) (map[string]any, error)
	ValidateCredentials(creds map[string]any) error
}

type Registry struct {
	providers map[string]Provider
}

func NewRegistry() *Registry { return &Registry{providers: make(map[string]Provider)} }

func (r *Registry) Register(p Provider) { r.providers[p.Name()] = p }

func (r *Registry) Get(name string) (Provider, bool) { p, ok := r.providers[name]; return p, ok }

func (r *Registry) List() []string {
	keys := make([]string, 0, len(r.providers))
	for k := range r.providers { keys = append(keys, k) }
	return keys
}
