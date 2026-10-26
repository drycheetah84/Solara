// Package di is the Go port of AUnrealFeatures.DependencyInjection.
//
// Semantics preserved from the original:
//   - Only singletons exist (AddSingleton); there are no scoped/transient lifetimes.
//   - Registering the same class twice is ignored (first registration wins).
//   - AddSingleton requires a class + interface pair; anything else is ignored.
//   - The provider eagerly initialises every service, in registration order,
//     when it is built (CompositeServiceProvider constructor), and lazily
//     retries initialisation on lookup.
//   - Lookup matches either the class or the interface key, first match wins.
//   - Quirk: a service whose class has constructor parameters AND was given a
//     pre-built implementation object is never instantiated - its Instance
//     stays nil (DependencyService.TryInitialize has no branch for it). The
//     original host hit this for AstraDatabaseManager and AstraDatabase, so
//     resolving IDatabase from the container yielded null; servers use the
//     static Program.Database instead. Solara reproduces it (HasCtorParams).
package di

import (
	"fmt"
	"sync"

	"solara/internal/dotnet"
)

// Service mirrors DependencyService.
type Service struct {
	Id        string
	Class     string
	Interface string
	// HasCtorParams records whether the original class's widest constructor
	// takes parameters (drives the implementation-object quirk).
	HasCtorParams bool
	impl          any
	ctor          func(p Provider) (any, error)
	instance      any
}

// Instance returns the resolved singleton (nil if never constructed).
func (s *Service) Instance() any { return s.instance }

// Provider mirrors ServiceProvider.GetService.
type Provider interface {
	GetService(key string) any
}

func (s *Service) tryInitialize(p Provider) error {
	if s.instance != nil {
		return nil
	}
	if !s.HasCtorParams {
		if s.impl != nil {
			s.instance = s.impl
			return nil
		}
		if s.ctor == nil {
			return nil
		}
		inst, err := s.ctor(p)
		if err != nil {
			return err
		}
		s.instance = inst
		return nil
	}
	if s.impl == nil && s.ctor != nil {
		inst, err := s.ctor(p)
		if err != nil {
			return err
		}
		s.instance = inst
	}
	return nil
}

// Builder mirrors DependencyServiceBuilder.
type Builder struct {
	services []*Service
}

// AddSingleton mirrors AddSingleton<TClass, TInterface>([implementationObject]).
// ctor builds the instance (resolving its dependencies from p); impl is the
// optional pre-built object.
func (b *Builder) AddSingleton(class, iface string, hasCtorParams bool, impl any, ctor func(p Provider) (any, error)) {
	if class == "" || iface == "" {
		return
	}
	for _, s := range b.services {
		if s.Class == class {
			return
		}
	}
	b.services = append(b.services, &Service{Id: dotnet.NewGuid(), Class: class, Interface: iface,
		HasCtorParams: hasCtorParams, impl: impl, ctor: ctor})
}

// Services mirrors ServiceBuilder.Services.
func (b *Builder) Services() []*Service { return b.services }

// CompositeProvider mirrors CompositeServiceProvider.
type CompositeProvider struct {
	mu       sync.Mutex
	services []*Service
}

// NewCompositeProvider initialises every service in registration order. An
// initialisation error aborts construction (the original threw).
func NewCompositeProvider(builders ...*Builder) (*CompositeProvider, error) {
	p := &CompositeProvider{}
	for _, b := range builders {
		p.services = append(p.services, b.services...)
	}
	for _, s := range p.services {
		if err := s.tryInitialize(p); err != nil {
			return nil, fmt.Errorf("initialising %s: %w", s.Class, err)
		}
	}
	return p, nil
}

// GetService resolves by class or interface key.
func (p *CompositeProvider) GetService(key string) any {
	for _, s := range p.services {
		if s.Class == key || s.Interface == key {
			_ = s.tryInitialize(p)
			return s.instance
		}
	}
	return nil
}

// ServiceHost mirrors ServiceHost.
type ServiceHost struct {
	name     string
	builder  *Builder
	provider *CompositeProvider
}

// NewServiceHost mirrors ServiceHost.Create(name).
func NewServiceHost(name string) *ServiceHost { return &ServiceHost{name: name} }

// AddDependencies mirrors ServiceHost.AddDependencies (the callback runs immediately).
func (h *ServiceHost) AddDependencies(fn func(*Builder)) *ServiceHost {
	if h.builder == nil {
		h.builder = &Builder{}
	}
	fn(h.builder)
	return h
}

// GetServiceProvider mirrors ServiceHost.GetServiceProvider (built once).
func (h *ServiceHost) GetServiceProvider() (*CompositeProvider, error) {
	if h.provider != nil {
		return h.provider, nil
	}
	if h.builder == nil {
		h.builder = &Builder{}
	}
	p, err := NewCompositeProvider(h.builder)
	if err != nil {
		return nil, err
	}
	h.provider = p
	return p, nil
}
