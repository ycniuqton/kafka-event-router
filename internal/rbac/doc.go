// Package rbac is access control: roles, the permission catalog and role matrix, the config-selected
// PolicyEngine, the cached subject loader, and the Manager that implements platform-core's Authorizer port.
// Business code talks only to Manager; users and role bindings are administered through its services.
package rbac
