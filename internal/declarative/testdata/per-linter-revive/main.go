// Package fixture is a test fixture for the revive exported rule integration test.
// It deliberately omits doc comments on exported symbols to trigger revive's
// exported rule.
package fixture

// User holds user data.
type User struct {
	name string
}

// Name returns the name but this comment does not start with the method name.
func (u *User) GetName() string {
	return u.name
}
