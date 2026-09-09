// SPDX-License-Identifier: GPL-3.0-or-later

package catalog

import "sort"

// Closure expands a set of service names through the needs graph.
//
// Asking about one service is really asking about everything it stands on: a
// gateway with a broken dependency is a broken gateway, even though nothing
// about the gateway itself is wrong. Checking only the literal names produces
// the worst possible outcome — a clean diagnosis followed by a failed start,
// which teaches people to stop trusting the diagnosis.
//
// It returns the services reachable from the given names (including them),
// the infrastructure components they need, and any name that is not a service
// of this ecosystem.
func (e *Ecosystem) Closure(names []string, infra map[string]bool) (services, components, unknown []string) {
	seen := map[string]bool{}
	comps := map[string]bool{}
	var missing []string

	var visit func(name string)
	visit = func(name string) {
		if seen[name] {
			return
		}
		svc, ok := e.Services[name]
		if !ok {
			if infra[name] {
				comps[name] = true
				return
			}
			missing = append(missing, name)
			return
		}
		seen[name] = true
		for _, need := range svc.Needs {
			visit(need)
		}
	}

	for _, n := range names {
		visit(n)
	}

	for name := range seen {
		services = append(services, name)
	}
	for name := range comps {
		components = append(components, name)
	}
	sort.Strings(services)
	sort.Strings(components)
	sort.Strings(missing)
	return services, components, missing
}

// AllServices lists every service name, sorted.
func (e *Ecosystem) AllServices() []string {
	out := make([]string, 0, len(e.Services))
	for name := range e.Services {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// InfraPorts are the ports well-known components listen on, used when the
// catalog does not say otherwise. A soft default beats asking every catalog
// to restate what everyone knows.
var InfraPorts = map[string]int{
	"postgres":  5432,
	"mysql":     3306,
	"mariadb":   3306,
	"redis":     6379,
	"rabbitmq":  5672,
	"kafka":     9092,
	"minio":     9000,
	"hazelcast": 5701,
	"mongodb":   27017,
	"memcached": 11211,
}

// PortOf returns the port a component listens on: the catalog's value if it
// declares one, otherwise the well-known default. Zero means unknown.
func (i Infra) PortOf(component string) int {
	if p, ok := i.Ports[component]; ok {
		return p
	}
	return InfraPorts[component]
}
