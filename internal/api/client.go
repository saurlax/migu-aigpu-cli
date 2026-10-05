// Package api implements the observed same-origin web API, not a documented public API.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/saurlax/migu-aigpu-cli/internal/auth"
)

const Origin = "https://aigpu.migu.cn"
const Instance = "/v1/cloud/traininfer/v1/instance"

type Client struct {
	Store auth.Store
	HTTP  *http.Client
	Team  string
	base  string // test-only override, never exposed as a user flag
}

func New(store auth.Store, timeout time.Duration, team string) *Client {
	return &Client{Store: store, Team: team, base: Origin, HTTP: &http.Client{Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

type HTTPError struct{ Status int }

func (e *HTTPError) Error() string {
	return fmt.Sprintf("HTTP %d; response omitted to protect credentials", e.Status)
}

func (c *Client) request(ctx context.Context, method, path string, body any, session *auth.Session, form bool) (map[string]any, error) {
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") || strings.Contains(path, "..") {
		return nil, errors.New("invalid platform path")
	}
	var reader io.Reader
	if form {
		reader = strings.NewReader(body.(url.Values).Encode())
	} else if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, errors.New("cannot encode request")
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, reader)
	if err != nil {
		return nil, errors.New("cannot construct platform request")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "migu-aigpu-cli")
	req.Header.Set("RUBICK-AJAX-REQUEST", "true")
	req.Header.Set("X-REQUESTED-WITH", "XMLHttpRequest")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if session != nil {
		if form {
			req.SetBasicAuth(session.ClientID, session.ClientSecret)
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		} else {
			req.Header.Set("Authorization", "Bearer "+session.Access)
			if session.CSRF != "" {
				req.Header.Set("X-CSRFToken", session.CSRF)
			}
			if c.Team != "" {
				req.Header.Set("head_teamId", c.Team)
			}
		}
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, errors.New("network request failed; mutations are never automatically replayed")
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, &HTTPError{res.StatusCode}
	}
	if !strings.Contains(res.Header.Get("Content-Type"), "json") {
		return nil, errors.New("expected JSON response; reauthentication may be required")
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, 16*1024*1024+1))
	if err != nil || len(b) > 16*1024*1024 {
		return nil, errors.New("response unreadable or too large")
	}
	var result map[string]any
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	if dec.Decode(&result) != nil || result == nil {
		return nil, errors.New("invalid JSON response")
	}
	return result, nil
}

func (c *Client) refresh(ctx context.Context, s auth.Session) (auth.Session, error) {
	result, err := c.request(ctx, "POST", "/v4/oauth/token", url.Values{"grant_type": {"refresh_token"}, "refresh_token": {s.Refresh}}, &s, true)
	if err != nil {
		return s, err
	}
	if _, ok := result["access_token"].(string); !ok {
		if data, ok := result["data"].(map[string]any); ok {
			result = data
		}
	}
	access, _ := result["access_token"].(string)
	ttl, err := time.ParseDuration(fmt.Sprint(result["expires_in"]) + "s")
	if access == "" || err != nil || ttl <= 240*time.Second {
		return s, errors.New("refresh rejected; log in and import again; existing session preserved")
	}
	next := s
	next.Access = access
	if token, ok := result["refresh_token"].(string); ok && token != "" {
		next.Refresh = token
	}
	now := time.Now()
	next.ExpiresAt = float64(now.Add(ttl).Unix())
	next.RefreshAt = next.ExpiresAt - 240
	next.LastRefresh = float64(now.Unix())
	if err = c.Store.Save(next); err != nil {
		return s, err
	}
	return next, nil
}

func (c *Client) Ensure(ctx context.Context, force bool) (auth.Session, bool, error) {
	release, err := c.Store.Lock(ctx)
	if err != nil {
		return auth.Session{}, false, err
	}
	defer release()
	s, err := c.Store.Load()
	if err != nil {
		return s, false, err
	}
	if force || s.Due(time.Now()) {
		s, err = c.refresh(ctx, s)
		return s, err == nil, err
	}
	return s, false, nil
}

func (c *Client) Call(ctx context.Context, method, path string, body any, readOnly bool) (map[string]any, error) {
	release, err := c.Store.Lock(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	s, err := c.Store.Load()
	if err != nil {
		return nil, err
	}
	if s.Due(time.Now()) {
		s, err = c.refresh(ctx, s)
		if err != nil {
			return nil, err
		}
	}
	result, err := c.request(ctx, method, path, body, &s, false)
	var status *HTTPError
	if readOnly && errors.As(err, &status) && status.Status == 401 {
		s, err = c.refresh(ctx, s)
		if err != nil {
			return nil, err
		}
		result, err = c.request(ctx, method, path, body, &s, false)
	}
	if err != nil {
		return nil, err
	}
	if code, ok := result["code"]; ok && fmt.Sprint(code) != "1000" {
		return nil, errors.New("platform rejected operation; response omitted to protect credentials")
	}
	return result, nil
}
