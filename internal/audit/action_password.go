package audit

// Catalogued actions of password login (access-control 00001-password-auth).
const (
	ActionUserCreated       Action = "user.created"
	ActionUserPasswordReset Action = "user.password_reset"
	ActionPasswordChanged   Action = "auth.password_changed"
)

// registerPasswordActions adds the password-login actions to c (admin changes and self-service access events).
func (c *Catalog) registerPasswordActions() {
	c.registerAll(CategoryAdmin, false, ActionUserCreated, ActionUserPasswordReset)
	c.registerAll(CategoryAccess, false, ActionPasswordChanged)
}
