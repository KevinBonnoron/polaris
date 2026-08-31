package polaris

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"
)

// pacHelpers is a JavaScript preamble implementing the standard PAC helper
// functions expected by FindProxyForURL scripts.
const pacHelpers = `
function isPlainHostName(host) {
  return host.indexOf('.') === -1;
}
function dnsDomainIs(host, domain) {
  return host.length >= domain.length &&
    host.substring(host.length - domain.length) === domain;
}
function localHostOrDomainIs(host, hostdom) {
  return host === hostdom ||
    hostdom.substring(0, hostdom.indexOf('.')) === host;
}
function isResolvable(host) { return true; }
function dnsResolve(host) { return ''; }
function myIpAddress() { return '127.0.0.1'; }
function dnsDomainLevels(host) {
  var n = 0, s = host;
  while ((s = s.replace(/[^.]*\./, '')) !== '') n++;
  return n;
}
function shExpMatch(str, pattern) {
  var re = '^' + pattern.replace(/\./g,'[.]').replace(/\*/g,'.*').replace(/\?/g,'.') + '$';
  return new RegExp(re).test(str);
}
function isInNet(host, pattern, mask) {
  var hp = host.split('.').map(Number);
  var pp = pattern.split('.').map(Number);
  var mp = mask.split('.').map(Number);
  if (hp.length !== 4) return false;
  for (var i = 0; i < 4; i++) {
    if ((hp[i] & mp[i]) !== (pp[i] & mp[i])) return false;
  }
  return true;
}
function weekdayRange() { return false; }
function dateRange()    { return false; }
function timeRange()    { return false; }
function alert()        {}
`

// pacProbe describes a URL to evaluate against the PAC file and what to do
// with the result.
type pacProbe struct {
	rawURL string // full URL passed to FindProxyForURL
	host   string // host portion
	// noProxyEntry is what to add to NO_PROXY if the result is DIRECT.
	noProxyEntry string
}

var pacProbes = []pacProbe{
	{"http://example.com/", "example.com", ""},  // determine HTTP proxy
	{"https://example.com/", "example.com", ""}, // determine HTTPS proxy
	{"http://localhost/", "localhost", "localhost"},
	{"http://127.0.0.1/", "127.0.0.1", "127.0.0.1"},
	{"http://10.0.0.1/", "10.0.0.1", "10.0.0.0/8"},
	{"http://172.16.0.1/", "172.16.0.1", "172.16.0.0/12"},
	{"http://192.168.0.1/", "192.168.0.1", "192.168.0.0/16"},
	{"http://host.local/", "host.local", ".local"},
	{"http://host.internal/", "host.internal", ".internal"},
}

// pacResult holds proxy env-var values derived from a PAC evaluation.
type pacResult struct {
	HTTPProxy  string
	HTTPSProxy string
	NoProxy    string
}

func (r pacResult) Env() []string {
	var env []string
	if r.HTTPProxy != "" {
		env = append(env, "HTTP_PROXY="+r.HTTPProxy, "http_proxy="+r.HTTPProxy)
	}
	if r.HTTPSProxy != "" {
		env = append(env, "HTTPS_PROXY="+r.HTTPSProxy, "https_proxy="+r.HTTPSProxy)
	}
	if r.NoProxy != "" {
		env = append(env, "NO_PROXY="+r.NoProxy, "no_proxy="+r.NoProxy)
	}
	return env
}

// pacResultCache is a goroutine-safe cache for PAC evaluation results with a
// configurable TTL. A zero fetchedAt means no result is available yet.
type pacResultCache struct {
	mu        sync.RWMutex
	result    pacResult
	fetchedAt time.Time
	ttl       time.Duration
}

func newPACResultCache() *pacResultCache {
	return &pacResultCache{ttl: 5 * time.Minute}
}

func (c *pacResultCache) get() (pacResult, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.fetchedAt.IsZero() || time.Since(c.fetchedAt) > c.ttl {
		return pacResult{}, false
	}
	return c.result, true
}

func (c *pacResultCache) set(r pacResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.result = r
	c.fetchedAt = time.Now()
}

func (c *pacResultCache) invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.fetchedAt = time.Time{}
}

func (c *pacResultCache) Env() []string {
	r, ok := c.get()
	if !ok {
		return nil
	}
	return r.Env()
}

// fetchPACScript downloads the PAC file at pacURL and returns its JS source.
func fetchPACScript(pacURL string) (string, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest(http.MethodGet, pacURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Cache-Control", "no-cache")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("PAC fetch status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

const pacEvalTimeout = 5 * time.Second

// evalFindProxyForURL runs FindProxyForURL(targetURL, host) in a fresh goja VM
// loaded with the PAC helpers preamble and the PAC script. Returns the raw
// result string (e.g. "PROXY host:port" or "DIRECT").
// A watchdog interrupts the VM after pacEvalTimeout to prevent a PAC script
// with an infinite loop from blocking the caller indefinitely.
func evalFindProxyForURL(pacScript, targetURL, host string) (string, error) {
	vm := goja.New()

	done := make(chan struct{})
	defer close(done)
	go func() {
		timer := time.NewTimer(pacEvalTimeout)
		defer timer.Stop()
		select {
		case <-done:
		case <-timer.C:
			vm.Interrupt("PAC script execution timed out")
		}
	}()

	if _, err := vm.RunString(pacHelpers + "\n" + pacScript); err != nil {
		return "", fmt.Errorf("pac eval: %w", err)
	}
	fn, ok := goja.AssertFunction(vm.Get("FindProxyForURL"))
	if !ok {
		return "", fmt.Errorf("FindProxyForURL not defined in PAC script")
	}
	v, err := fn(goja.Undefined(), vm.ToValue(targetURL), vm.ToValue(host))
	if err != nil {
		return "", fmt.Errorf("FindProxyForURL(%s): %w", host, err)
	}
	return v.String(), nil
}

// parseProxyDirective extracts the first proxy URL from a PAC result string
// such as "PROXY host:port; DIRECT". Preserves SOCKS/SOCKS4/SOCKS5 schemes.
// Returns "" for DIRECT-only results.
func parseProxyDirective(result string) string {
	for _, directive := range strings.Split(result, ";") {
		d := strings.TrimSpace(directive)
		var scheme, addr string
		switch {
		case strings.HasPrefix(d, "PROXY "):
			scheme, addr = "http://", strings.TrimPrefix(d, "PROXY ")
		case strings.HasPrefix(d, "SOCKS5 "):
			scheme, addr = "socks5://", strings.TrimPrefix(d, "SOCKS5 ")
		case strings.HasPrefix(d, "SOCKS4 "):
			scheme, addr = "socks4://", strings.TrimPrefix(d, "SOCKS4 ")
		case strings.HasPrefix(d, "SOCKS "):
			scheme, addr = "socks4://", strings.TrimPrefix(d, "SOCKS ")
		default:
			continue
		}
		if h := strings.TrimSpace(addr); h != "" {
			return scheme + h
		}
	}
	return ""
}

// isDirect reports whether a PAC result means "connect directly".
func isDirect(result string) bool {
	first := strings.TrimSpace(strings.SplitN(result, ";", 2)[0])
	return strings.EqualFold(first, "DIRECT")
}

// evaluatePACUrl fetches and evaluates a PAC file, running the probe set to
// derive HTTP_PROXY, HTTPS_PROXY and NO_PROXY values.
func evaluatePACUrl(pacURL string) (pacResult, error) {
	script, err := fetchPACScript(pacURL)
	if err != nil {
		return pacResult{}, fmt.Errorf("fetch PAC: %w", err)
	}

	var result pacResult
	var noProxyEntries []string
	seen := map[string]bool{}

	for _, probe := range pacProbes {
		raw, err := evalFindProxyForURL(script, probe.rawURL, probe.host)
		if err != nil {
			log.Printf("[DEBUG] pac probe %s: %v", probe.host, err)
			continue
		}

		u, _ := url.Parse(probe.rawURL)
		isHTTPS := u != nil && u.Scheme == "https"

		switch {
		case isDirect(raw):
			if probe.noProxyEntry != "" && !seen[probe.noProxyEntry] {
				noProxyEntries = append(noProxyEntries, probe.noProxyEntry)
				seen[probe.noProxyEntry] = true
			}
		case !isHTTPS && result.HTTPProxy == "":
			if p := parseProxyDirective(raw); p != "" {
				result.HTTPProxy = p
			}
		case isHTTPS && result.HTTPSProxy == "":
			if p := parseProxyDirective(raw); p != "" {
				result.HTTPSProxy = p
			}
		}
	}

	result.NoProxy = strings.Join(noProxyEntries, ",")
	return result, nil
}

// refreshPACCache fetches and evaluates the PAC file, storing the result in
// the cache. Errors are logged but not surfaced — a failed refresh leaves the
// cache unchanged (or empty on first attempt).
func (s *Service) refreshPACCache(pacURL string) {
	result, err := evaluatePACUrl(pacURL)
	if err != nil {
		log.Printf("[ERROR] pac evaluation failed: %v", err)
		return
	}
	log.Printf("[DEBUG] pac resolved: HTTP_PROXY=%s HTTPS_PROXY=%s NO_PROXY=%s",
		result.HTTPProxy, result.HTTPSProxy, result.NoProxy)
	if s.pacCache == nil {
		s.pacCache = newPACResultCache()
	}
	s.pacCache.set(result)
}

// WarmPACCache reads the stored network settings and, when PAC mode is active,
// performs an initial synchronous cache fill. Called at startup so the first
// agent spawn has proxy env vars ready.
func (s *Service) WarmPACCache() {
	ns, err := s.store.GetNetworkSettings()
	if err != nil || ns.Mode != ProxyModePAC || ns.PACUrl == "" {
		return
	}
	if s.pacCache == nil {
		s.pacCache = newPACResultCache()
	}
	s.refreshPACCache(ns.PACUrl)
}
