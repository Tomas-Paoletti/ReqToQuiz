package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Session states reported by AuthChecker.
const (
	AuthLoggedIn  = "logged_in"
	AuthLoggedOut = "logged_out"
	AuthUnknown   = "unknown" // the check itself failed: the engine is still tried
)

// AuthStatus says whether an engine has a session it can generate with.
type AuthStatus struct {
	State        string `json:"state"`
	Detail       string `json:"detail,omitempty"`       // e.g. "claude.ai · plan pro"
	LoginCommand string `json:"loginCommand,omitempty"` // how to log in
}

// Usable reports whether a generation should be attempted.
func (a AuthStatus) Usable() bool { return a.State != AuthLoggedOut }

// AuthChecker is implemented by engines that can tell if they are logged in.
type AuthChecker interface {
	Auth(ctx context.Context) AuthStatus
}

// runStatus runs a short status command and returns its stdout and exit code
// (-1 if it could not run).
func runStatus(ctx context.Context, bin string, args ...string) (string, int) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Cancel = func() error { return killTree(cmd) }
	cmd.WaitDelay = 2 * time.Second
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	var exitErr *exec.ExitError
	switch {
	case err == nil:
		return out.String(), 0
	case errors.As(err, &exitErr):
		return out.String(), exitErr.ExitCode()
	default:
		return out.String(), -1
	}
}

// Auth uses `claude auth status`, which prints JSON with "loggedIn".
func (c *Claude) Auth(ctx context.Context) AuthStatus {
	const login = "claude auth login"
	out, code := runStatus(ctx, c.Path, "auth", "status")
	var st struct {
		LoggedIn         *bool  `json:"loggedIn"`
		AuthMethod       string `json:"authMethod"`
		SubscriptionType string `json:"subscriptionType"`
	}
	start := strings.IndexByte(out, '{')
	if start < 0 || json.Unmarshal([]byte(out[start:]), &st) != nil || st.LoggedIn == nil {
		return AuthStatus{State: AuthUnknown, Detail: "no se pudo comprobar la sesión (exit " + itoa(code) + ")", LoginCommand: login}
	}
	if !*st.LoggedIn {
		return AuthStatus{State: AuthLoggedOut, LoginCommand: login}
	}
	detail := st.AuthMethod
	if st.SubscriptionType != "" {
		detail += " · plan " + st.SubscriptionType
	}
	return AuthStatus{State: AuthLoggedIn, Detail: detail, LoginCommand: login}
}

// Auth uses `codex login status`: exit 0 = logged in, exit 1 = "Not logged in".
// An API key in the environment also works for `codex exec`.
func (c *Codex) Auth(ctx context.Context) AuthStatus {
	const login = "codex login"
	out, code := runStatus(ctx, c.Path, "login", "status")
	msg := firstLine(out, "WARNING")
	switch {
	case code == 0:
		return AuthStatus{State: AuthLoggedIn, Detail: strings.TrimPrefix(msg, "Logged in using "), LoginCommand: login}
	case os.Getenv("CODEX_API_KEY") != "" || os.Getenv("OPENAI_API_KEY") != "":
		return AuthStatus{State: AuthLoggedIn, Detail: "API key en variable de entorno", LoginCommand: login}
	case code == 1 && strings.Contains(strings.ToLower(out), "not logged in"):
		return AuthStatus{State: AuthLoggedOut, LoginCommand: login}
	default:
		return AuthStatus{State: AuthUnknown, Detail: "no se pudo comprobar la sesión: " + msg, LoginCommand: login}
	}
}

// firstLine returns the first non-empty line that does not start with skip.
func firstLine(s, skip string) string {
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, skip) {
			return l
		}
	}
	return ""
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

// authCache avoids spawning the status commands on every request.
type authCache struct {
	mu      sync.Mutex
	ttl     time.Duration
	at      time.Time
	results map[string]AuthStatus
}

// statuses returns the session state of every engine, checked in parallel.
// Engines without AuthChecker are reported as unknown (and still tried).
func (c *authCache) statuses(ctx context.Context, engines []Engine, force bool) map[string]AuthStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !force && c.results != nil && time.Since(c.at) < c.ttl {
		return c.results
	}
	res := make(map[string]AuthStatus, len(engines))
	var wg sync.WaitGroup
	var mu sync.Mutex
	for _, e := range engines {
		checker, ok := e.(AuthChecker)
		if !ok {
			res[e.ID()] = AuthStatus{State: AuthUnknown}
			continue
		}
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			st := checker.Auth(ctx)
			mu.Lock()
			res[id] = st
			mu.Unlock()
		}(e.ID())
	}
	wg.Wait()
	c.results, c.at = res, time.Now()
	return res
}
