package main

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
	"sync"
	"time"
)

const (
	shortTimeout = 2 * time.Minute
	longTimeout  = 10 * time.Minute
	// jobTimeout bounds the wait for a background job (migration, clone,
	// backup): the server itself gives up after 24 hours.
	jobTimeout      = 24 * time.Hour
	jobPollInterval = 3 * time.Second
)

// Client is the Hostwarden REST API client.
type Client struct {
	base     string
	username string
	password string
	http     *http.Client

	mu    sync.Mutex
	token string
}

// NewClient authenticates with a token, or with a user name and password when
// the token is empty: the session is then opened on first use and reopened
// when it expires.
func NewClient(address, token, username, password string) *Client {
	return &Client{
		base:     strings.TrimRight(address, "/"),
		token:    token,
		username: username,
		password: password,
		http:     &http.Client{},
	}
}

type apiError struct {
	Code int
	Msg  string
}

func (e *apiError) Error() string { return fmt.Sprintf("hostwarden api error %d: %s", e.Code, e.Msg) }

// isNotFound reports whether err is the API saying the object does not exist.
func isNotFound(err error) bool {
	var ae *apiError
	return errors.As(err, &ae) && ae.Code == http.StatusNotFound
}

type taskTargetKey struct{}

// withTaskTarget names the object an operation is about: Hostwarden shows it
// in the Target column of Recent Tasks.
func withTaskTarget(ctx context.Context, target string) context.Context {
	return context.WithValue(ctx, taskTargetKey{}, target)
}

func (c *Client) currentToken() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.token
}

// login opens a session with the configured credentials, unless another
// request already replaced the token that was rejected.
func (c *Client) login(ctx context.Context, rejected string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && c.token != rejected {
		return nil
	}
	var out struct {
		Token string `json:"token"`
	}
	body := map[string]string{"username": c.username, "password": c.password}
	if err := c.send(ctx, http.MethodPost, "/api/v1/auth/login", "", body, &out); err != nil {
		return fmt.Errorf("login as %s: %w", c.username, err)
	}
	c.token = out.Token
	return nil
}

func (c *Client) do(ctx context.Context, method, path string, body, out any, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	token := c.currentToken()
	if token == "" {
		if err := c.login(ctx, ""); err != nil {
			return err
		}
		token = c.currentToken()
	}
	err := c.send(ctx, method, path, token, body, out)
	var ae *apiError
	if errors.As(err, &ae) && ae.Code == http.StatusUnauthorized && c.username != "" {
		// The session expired: open a new one and repeat the request once.
		if err := c.login(ctx, token); err != nil {
			return err
		}
		return c.send(ctx, method, path, c.currentToken(), body, out)
	}
	return err
}

func (c *Client) send(ctx context.Context, method, path, token string, body, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "terraform-provider-hostwarden/"+version)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if target, _ := ctx.Value(taskTargetKey{}).(string); target != "" {
		// Header values must be ASCII: the server decodes it.
		req.Header.Set("X-Task-Target", url.QueryEscape(target))
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}
	if resp.StatusCode >= 400 {
		msg := strings.TrimSpace(string(raw))
		var e struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(raw, &e) == nil && e.Error != "" {
			msg = e.Error
		}
		return &apiError{Code: resp.StatusCode, Msg: msg}
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("decoding response: %w", err)
		}
	}
	return nil
}

// seg escapes one path segment (names can contain spaces).
func seg(s string) string { return url.PathEscape(s) }

func hostPath(hostID string, rest ...string) string {
	p := "/api/v1/hosts/" + seg(hostID)
	for _, r := range rest {
		p += "/" + seg(r)
	}
	return p
}

func vmPath(hostID, vmID string, rest ...string) string {
	return hostPath(hostID, append([]string{"vms", vmID}, rest...)...)
}

// WaitJob polls a background job until it ends and returns its final status.
// A failed job is returned as an error.
func (c *Client) WaitJob(ctx context.Context, job *apiJob) (*apiJob, error) {
	ctx, cancel := context.WithTimeout(ctx, jobTimeout)
	defer cancel()
	id, failures := job.ID, 0
	for {
		switch job.Status {
		case "ok":
			return job, nil
		case "error":
			return job, fmt.Errorf("%s of %s failed during %q: %s", job.Kind, job.VMName, job.Phase, job.Error)
		}
		select {
		case <-ctx.Done():
			return job, fmt.Errorf("waiting for the %s of %s: %w", job.Kind, job.VMName, ctx.Err())
		case <-time.After(jobPollInterval):
		}
		var next apiJob
		if err := c.do(ctx, http.MethodGet, "/api/v1/jobs/"+seg(id), nil, &next, shortTimeout); err != nil {
			// A job is only known to the server process that started it.
			if failures++; isNotFound(err) || failures > 10 {
				return job, fmt.Errorf("lost track of the %s of %s (last phase %q): %w", job.Kind, job.VMName, job.Phase, err)
			}
			continue
		}
		failures, job = 0, &next
	}
}
