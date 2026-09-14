package mikrotik

// Credentials are the device credentials used by the REST transport. The DB
// only stores a router username today, so the shared password comes from
// configuration; per-router passwords can be supplied on Router.Password and
// take precedence.
type Credentials struct {
	Username string
	Password string
}

func (c Credentials) forRouter(r Router) Credentials {
	if r.Username != "" {
		c.Username = r.Username
	}
	if r.Password != "" {
		c.Password = r.Password
	}
	return c
}