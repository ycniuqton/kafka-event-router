package platform

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/eventrouter/router/test/stress/kit/scenario"
)

// AuthPaths are the sign-in endpoints (same as test/acceptance/lib/Login.ts).
var AuthPaths = struct {
	DevLogin, OIDCLogin, Me string
	MaxRedirects            int
}{DevLogin: "/auth/dev/login", OIDCLogin: "/auth/oidc/login", Me: "/api/v1/auth/me", MaxRedirects: 10}

// LoginStrategy signs a client in.
type LoginStrategy interface {
	SignIn(ctx context.Context, c *Client, spec scenario.APISpec) error
}

// DevLogin posts to /auth/dev/login (local: the user becomes admin of the default team).
type DevLogin struct{}

// SignIn performs the dev login.
func (DevLogin) SignIn(ctx context.Context, c *Client, spec scenario.APISpec) error {
	body := map[string]string{}
	if spec.Email != "" {
		body["email"] = spec.Email
	}
	res, err := c.Do(ctx, http.MethodPost, AuthPaths.DevLogin, body, "")
	if err != nil {
		return err
	}
	return res.Err("dev login")
}

// TokenLogin uses an API token (POST /api/v1/auth/tokens with write scope) as bearer.
type TokenLogin struct{}

// SignIn sets the bearer token.
func (TokenLogin) SignIn(_ context.Context, c *Client, spec scenario.APISpec) error {
	if spec.Token == "" {
		return fmt.Errorf("token login needs api.token (e.g. ${env:STRESS_API_TOKEN})")
	}
	c.SetBearer(spec.Token)
	return nil
}

// DexLogin walks the OIDC redirect chain to Dex's password form and posts the static staging user.
type DexLogin struct{}

var formAction = regexp.MustCompile(`(?is)<form[^>]*method="post"[^>]*action="([^"]+)"`)

// SignIn performs the browserless OIDC login.
func (d DexLogin) SignIn(ctx context.Context, c *Client, spec scenario.APISpec) error {
	page, final, err := d.follow(ctx, c, http.MethodGet, c.URL(AuthPaths.OIDCLogin), nil)
	if err != nil {
		return err
	}
	m := formAction.FindSubmatch(page)
	if m == nil {
		return fmt.Errorf("dex login: no password form at %s", final.Path)
	}
	action, err := final.Parse(html.UnescapeString(string(m[1])))
	if err != nil {
		return err
	}
	form := url.Values{"login": {spec.Email}, "password": {spec.Password}}
	_, landed, err := d.follow(ctx, c, http.MethodPost, action, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	if strings.HasPrefix(landed.Path, "/dex") {
		return fmt.Errorf("dex login rejected (landed on %s)", landed.Path)
	}
	return nil
}

func (DexLogin) follow(ctx context.Context, c *Client, method string, u *url.URL, body io.Reader) ([]byte, *url.URL, error) {
	for i := 0; i < AuthPaths.MaxRedirects; i++ {
		req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
		if err != nil {
			return nil, nil, err
		}
		if body != nil {
			req.Header.Set(Wire.ContentType, "application/x-www-form-urlencoded")
		}
		res, err := c.HTTP().Do(req)
		if err != nil {
			return nil, nil, err
		}
		page, _ := io.ReadAll(res.Body)
		res.Body.Close()
		loc := res.Header.Get("Location")
		if res.StatusCode < 300 || res.StatusCode >= 400 || loc == "" {
			return page, u, nil
		}
		if u, err = u.Parse(loc); err != nil {
			return nil, nil, err
		}
		method, body = http.MethodGet, nil
	}
	return nil, nil, fmt.Errorf("dex login: too many redirects")
}

// LoginManager selects the configured strategy.
type LoginManager struct{}

var loginStrategies = map[scenario.LoginMethod]LoginStrategy{
	scenario.LoginDev: DevLogin{}, scenario.LoginDex: DexLogin{}, scenario.LoginToken: TokenLogin{},
}

// SignIn opens an authenticated session for spec.
func (LoginManager) SignIn(ctx context.Context, spec scenario.APISpec) (*Client, error) {
	s, ok := loginStrategies[spec.Login]
	if !ok {
		return nil, fmt.Errorf("unknown login %q (dev|dex|token)", spec.Login)
	}
	c, err := NewClient(spec.BaseURL)
	if err != nil {
		return nil, err
	}
	if err := s.SignIn(ctx, c, spec); err != nil {
		return nil, err
	}
	me, err := c.Get(ctx, AuthPaths.Me)
	if err != nil {
		return nil, err
	}
	return c, me.Err("whoami after login")
}
