// Package password is the email + password login (ROUTER_AUTH_MODES=password): argon2id credentials in Postgres,
// a failed-login throttle with account lockout, the forced first change, self-service change and the
// first-start admin seed. There is no self-registration: credentials are created only by the seed or an admin.
package password
