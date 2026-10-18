package controlplane

import "context"

// GetJSON decodes the JSON body of an internal GET path; feature clients (connection specs,
// team salts) build on it instead of opening their own HTTP clients.
func (c *Client) GetJSON(ctx context.Context, path string, out any) error {
	return c.get(ctx, path, out)
}
