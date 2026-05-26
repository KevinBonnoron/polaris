package polaris

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Version is the current application version. Override at link time with
// `-ldflags "-X github.com/KevinBonnoron/polaris/internal/polaris.Version=1.2.3"`.
var Version = "0.0.1"

const (
	updateCacheTTL  = 30 * time.Minute
	releasesAPI     = "https://api.github.com/repos/KevinBonnoron/polaris/releases/latest"
	releasesPageURL = "https://github.com/KevinBonnoron/polaris/releases/latest"
)

type UpdateInfo struct {
	Current       string `json:"current"`
	Latest        string `json:"latest"`
	HasUpdate     bool   `json:"hasUpdate"`
	HTMLURL       string `json:"htmlUrl"`
	ReleaseNotes  string `json:"releaseNotes,omitempty"`
	PublishedAt   string `json:"publishedAt,omitempty"`
	CheckedAt     string `json:"checkedAt"`
}

type releaseResponse struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	HTMLURL     string `json:"html_url"`
	Body        string `json:"body"`
	PublishedAt string `json:"published_at"`
	Draft       bool   `json:"draft"`
	Prerelease  bool   `json:"prerelease"`
}

var (
	updateCache   *UpdateInfo
	updateCacheAt time.Time
	updateMu      sync.Mutex
)

func AppVersion() string {
	return Version
}

func CheckForUpdate(force bool) (*UpdateInfo, error) {
	updateMu.Lock()
	defer updateMu.Unlock()

	if !force && updateCache != nil && time.Since(updateCacheAt) < updateCacheTTL {
		return updateCache, nil
	}

	info, err := fetchLatestRelease()
	if err != nil {
		return nil, err
	}
	updateCache = info
	updateCacheAt = time.Now()
	return info, nil
}

func fetchLatestRelease() (*UpdateInfo, error) {
	req, err := http.NewRequest(http.MethodGet, releasesAPI, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "polaris-updater")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call releases endpoint: %w", err)
	}
	defer resp.Body.Close()

	now := time.Now().UTC().Format(time.RFC3339)

	// 404: no release yet. 403: often returned by GitHub for repos with no
	// releases on unauthenticated calls (and for rate-limit). Treat both as
	// "no upstream version available" rather than surfacing a scary error.
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusForbidden {
		return &UpdateInfo{
			Current:   Version,
			Latest:    Version,
			HasUpdate: false,
			HTMLURL:   releasesPageURL,
			CheckedAt: now,
		}, nil
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("releases endpoint returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var data releaseResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("decode release response: %w", err)
	}

	latest := strings.TrimPrefix(strings.TrimSpace(data.TagName), "v")
	url := data.HTMLURL
	if url == "" {
		url = releasesPageURL
	}

	return &UpdateInfo{
		Current:      Version,
		Latest:       latest,
		HasUpdate:    latest != "" && compareVersions(latest, Version) > 0,
		HTMLURL:      url,
		ReleaseNotes: data.Body,
		PublishedAt:  data.PublishedAt,
		CheckedAt:    now,
	}, nil
}

// compareVersions returns -1, 0, 1 for semver-ish strings (e.g. "1.2.3", "1.2.3-rc1").
// Non-numeric segments compare lexicographically; pre-release suffixes count as lower.
func compareVersions(a, b string) int {
	aMain, aPre := splitPrerelease(a)
	bMain, bPre := splitPrerelease(b)

	aParts := strings.Split(aMain, ".")
	bParts := strings.Split(bMain, ".")
	n := len(aParts)
	if len(bParts) > n {
		n = len(bParts)
	}
	for i := 0; i < n; i++ {
		ai := segmentAt(aParts, i)
		bi := segmentAt(bParts, i)
		if c := compareSegment(ai, bi); c != 0 {
			return c
		}
	}

	switch {
	case aPre == "" && bPre == "":
		return 0
	case aPre == "":
		return 1
	case bPre == "":
		return -1
	default:
		return strings.Compare(aPre, bPre)
	}
}

func splitPrerelease(v string) (string, string) {
	if i := strings.Index(v, "-"); i >= 0 {
		return v[:i], v[i+1:]
	}
	return v, ""
}

func segmentAt(parts []string, i int) string {
	if i < len(parts) {
		return parts[i]
	}
	return "0"
}

func compareSegment(a, b string) int {
	ai, aErr := strconv.Atoi(a)
	bi, bErr := strconv.Atoi(b)
	if aErr == nil && bErr == nil {
		if ai < bi {
			return -1
		}
		if ai > bi {
			return 1
		}
		return 0
	}
	return strings.Compare(a, b)
}
