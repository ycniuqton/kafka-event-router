package sandbox

// scopeKey groups the sandboxes of one route version and mode (one sandbox per JS step).
type scopeKey struct {
	route, version string
	mode           ScopeMode
}

func keyOf(s RouteScope) scopeKey {
	return scopeKey{route: s.RouteID, version: s.VersionID, mode: s.Mode}
}

// ScalablePools implements ScalablePools: every pool of every open sandbox.
func (m *DefaultManager) ScalablePools() []ScopedPool {
	var out []ScopedPool
	for _, rs := range m.openSandboxes() {
		rs.mu.Lock()
		for _, mod := range rs.modules {
			out = append(out, ScopedPool{Scope: rs.scope, Pool: mod.pool})
		}
		rs.mu.Unlock()
	}
	return out
}

// PublishUsage implements ScalablePools: one pool gauge update per open route version.
func (m *DefaultManager) PublishUsage() {
	groups := map[scopeKey][]*routeSandbox{}
	for _, rs := range m.openSandboxes() {
		groups[keyOf(rs.scope)] = append(groups[keyOf(rs.scope)], rs)
	}
	for _, list := range groups {
		m.observeGroup(list)
	}
}

// publishScope republishes the pool gauges of scope's route version.
func (m *DefaultManager) publishScope(scope RouteScope) {
	if list := m.sandboxesOf(scope); len(list) > 0 {
		m.observeGroup(list)
	}
}

// observeGroup reports the summed usage of the sandboxes of one route version.
func (m *DefaultManager) observeGroup(list []*routeSandbox) {
	u := RouteUsage{}
	for _, rs := range list {
		one := rs.Usage()
		u.Instances += one.Instances
		u.MemoryBytes += one.MemoryBytes
		u.Calls += one.Calls
		u.CPUTime += one.CPUTime
	}
	m.deps.Observer.PoolObserved(list[0].scope, PoolStats{Size: u.Instances, MemoryBytes: u.MemoryBytes, Calls: u.Calls, CPUTime: u.CPUTime})
}

// openSandboxes snapshots the open sandboxes.
func (m *DefaultManager) openSandboxes() []*routeSandbox {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*routeSandbox, 0, len(m.open))
	for rs := range m.open {
		out = append(out, rs)
	}
	return out
}
