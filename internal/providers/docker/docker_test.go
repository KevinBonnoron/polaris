package docker

import (
	"os"
	"path/filepath"
	"testing"
)

func writeDockerfile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "Dockerfile")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDetectFindsDockerfileInSubdir(t *testing.T) {
	root := t.TempDir()
	dockerDir := filepath.Join(root, "docker")
	if err := os.MkdirAll(dockerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dockerDir, "Dockerfile"), []byte("FROM scratch\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Detect(root)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected to detect docker/Dockerfile, got nil")
	}
	if got.DockerfilePath != filepath.Join(dockerDir, "Dockerfile") {
		t.Errorf("got %q", got.DockerfilePath)
	}
}

func TestDetectPrefersRootDockerfile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Dockerfile"), []byte("FROM scratch\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dockerDir := filepath.Join(root, "docker")
	_ = os.MkdirAll(dockerDir, 0o755)
	_ = os.WriteFile(filepath.Join(dockerDir, "Dockerfile"), []byte("FROM scratch\n"), 0o644)
	got, err := Detect(root)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.DockerfilePath != filepath.Join(root, "Dockerfile") {
		t.Errorf("expected root Dockerfile, got %+v", got)
	}
}

func TestDetectSuffixedVariant(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Dockerfile.prod"), []byte("FROM scratch\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Detect(root)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.DockerfilePath != filepath.Join(root, "Dockerfile.prod") {
		t.Errorf("expected Dockerfile.prod, got %+v", got)
	}
}

func TestDetectNoDockerfile(t *testing.T) {
	got, err := Detect(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestParseDockerfileStagesAndTags(t *testing.T) {
	path := writeDockerfile(t, `
# comment
FROM --platform=linux/amd64 node:20-alpine AS builder
WORKDIR /app
COPY . .
RUN npm ci

FROM gcr.io/distroless/nodejs20-debian12@sha256:abc AS runtime
USER nonroot
EXPOSE 3000 8080
HEALTHCHECK CMD curl -f http://localhost:3000 || exit 1
`)
	df, err := ParseDockerfile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(df.Stages) != 2 {
		t.Fatalf("want 2 stages, got %d", len(df.Stages))
	}
	if df.Stages[0].BaseImage != "node:20-alpine" || df.Stages[0].Tag != "20-alpine" || df.Stages[0].Name != "builder" {
		t.Errorf("stage 0 wrong: %+v", df.Stages[0])
	}
	if !df.Stages[1].Final {
		t.Errorf("last stage should be final")
	}
	if df.Stages[1].Digest == "" {
		t.Errorf("runtime stage should have a digest")
	}
	if df.User != "nonroot" {
		t.Errorf("want user nonroot, got %q", df.User)
	}
	if !df.HasHealthcheck {
		t.Errorf("healthcheck should be detected")
	}
	if len(df.ExposedPorts) != 2 {
		t.Errorf("want 2 ports, got %v", df.ExposedPorts)
	}
}

func TestParseDockerfileFindings(t *testing.T) {
	path := writeDockerfile(t, `FROM ubuntu:latest
ENV API_KEY=supersecret
ADD https://example.com/x.tar.gz /tmp/
`)
	df, err := ParseDockerfile(path)
	if err != nil {
		t.Fatal(err)
	}
	rules := map[string]bool{}
	for _, f := range df.Findings {
		rules[f.Rule] = true
	}
	for _, want := range []string{"base-image-tag", "secret-in-build", "add-url", "root-user"} {
		if !rules[want] {
			t.Errorf("missing finding %q (got %v)", want, rules)
		}
	}
}

func TestParseSize(t *testing.T) {
	cases := map[string]int64{
		"0B":     0,
		"568kB":  568_000,
		"142MB":  142_000_000,
		"1.2GB":  1_200_000_000,
		"":       0,
		"  5MB ": 5_000_000,
	}
	for in, want := range cases {
		if got := parseSize(in); got != want {
			t.Errorf("parseSize(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestNormalizeRef(t *testing.T) {
	cases := map[string]string{
		"node":                   "node:latest",
		"node:20":                "node:20",
		"registry:5000/app":      "registry:5000/app:latest",
		"registry:5000/app:1.0":  "registry:5000/app:1.0",
		"alpine@sha256:deadbeef": "alpine",
	}
	for in, want := range cases {
		if got := normalizeRef(in); got != want {
			t.Errorf("normalizeRef(%q) = %q, want %q", in, got, want)
		}
	}
}
