package csharp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDetectFindsCsprojInSubdir(t *testing.T) {
	root := t.TempDir()
	csproj := filepath.Join(root, "src", "App", "App.csproj")
	writeFile(t, csproj, `<Project Sdk="Microsoft.NET.Sdk"><ItemGroup></ItemGroup></Project>`)
	// Build output should be ignored.
	writeFile(t, filepath.Join(root, "src", "App", "obj", "Debug.csproj"), `<Project/>`)

	got, err := Detect(root)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected a project, got nil")
	}
	if got.ManifestPath != csproj {
		t.Fatalf("manifest = %q, want %q", got.ManifestPath, csproj)
	}
	if got.PackageManager != "dotnet" {
		t.Fatalf("packageManager = %q, want dotnet", got.PackageManager)
	}
}

func TestDetectNoProject(t *testing.T) {
	got, err := Detect(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

func TestReadManifestDepsAttributeAndElement(t *testing.T) {
	root := t.TempDir()
	csproj := filepath.Join(root, "App.csproj")
	writeFile(t, csproj, `<Project Sdk="Microsoft.NET.Sdk">
  <ItemGroup>
    <PackageReference Include="Newtonsoft.Json" Version="13.0.1" />
    <PackageReference Include="Serilog">
      <Version>3.1.1</Version>
    </PackageReference>
  </ItemGroup>
</Project>`)

	deps, err := readManifestDeps(csproj)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"Newtonsoft.Json": "13.0.1", "Serilog": "3.1.1"}
	if len(deps) != len(want) {
		t.Fatalf("got %d deps, want %d: %+v", len(deps), len(want), deps)
	}
	for _, d := range deps {
		if want[d.Name] != d.Version {
			t.Errorf("%s version = %q, want %q", d.Name, d.Version, want[d.Name])
		}
	}
}

func TestReadManifestDepsCentralPackageManagement(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "Directory.Packages.props"), `<Project>
  <ItemGroup>
    <PackageVersion Include="Newtonsoft.Json" Version="13.0.3" />
  </ItemGroup>
</Project>`)
	csproj := filepath.Join(root, "src", "App.csproj")
	writeFile(t, csproj, `<Project Sdk="Microsoft.NET.Sdk">
  <ItemGroup>
    <PackageReference Include="Newtonsoft.Json" />
  </ItemGroup>
</Project>`)

	deps, err := readManifestDeps(csproj)
	if err != nil {
		t.Fatal(err)
	}
	if len(deps) != 1 || deps[0].Version != "13.0.3" {
		t.Fatalf("got %+v, want Newtonsoft.Json@13.0.3 from central props", deps)
	}
}

func TestSetDependencyVersionRewritesAttribute(t *testing.T) {
	root := t.TempDir()
	csproj := filepath.Join(root, "App.csproj")
	writeFile(t, csproj, `<Project Sdk="Microsoft.NET.Sdk">
  <ItemGroup>
    <PackageReference Include="Newtonsoft.Json" Version="13.0.1" />
  </ItemGroup>
</Project>`)

	if err := SetDependencyVersion(csproj, "Newtonsoft.Json", "13.0.3"); err != nil {
		t.Fatal(err)
	}
	deps, err := readManifestDeps(csproj)
	if err != nil {
		t.Fatal(err)
	}
	if len(deps) != 1 || deps[0].Version != "13.0.3" {
		t.Fatalf("got %+v, want version 13.0.3", deps)
	}
}

func TestSetDependencyVersionBothAttributeOrders(t *testing.T) {
	root := t.TempDir()
	csproj := filepath.Join(root, "App.csproj")
	writeFile(t, csproj, `<Project Sdk="Microsoft.NET.Sdk">
  <ItemGroup Condition="'$(Configuration)' == 'Debug'">
    <PackageReference Include="Serilog" Version="3.1.0" />
  </ItemGroup>
  <ItemGroup Condition="'$(Configuration)' == 'Release'">
    <PackageReference Version="3.1.0" Include="Serilog" />
  </ItemGroup>
</Project>`)

	if err := SetDependencyVersion(csproj, "Serilog", "3.1.1"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(csproj)
	if err != nil {
		t.Fatal(err)
	}
	if c := strings.Count(string(data), `Version="3.1.1"`); c != 2 {
		t.Fatalf("expected both declarations rewritten, got %d occurrences of 3.1.1:\n%s", c, data)
	}
	if strings.Contains(string(data), `3.1.0`) {
		t.Fatalf("stale 3.1.0 left in file:\n%s", data)
	}
}

func TestSetDependencyVersionCentralProps(t *testing.T) {
	root := t.TempDir()
	props := filepath.Join(root, "Directory.Packages.props")
	writeFile(t, props, `<Project>
  <ItemGroup>
    <PackageVersion Include="Serilog" Version="3.1.0" />
  </ItemGroup>
</Project>`)
	csproj := filepath.Join(root, "App.csproj")
	writeFile(t, csproj, `<Project Sdk="Microsoft.NET.Sdk">
  <ItemGroup>
    <PackageReference Include="Serilog" />
  </ItemGroup>
</Project>`)

	if err := SetDependencyVersion(csproj, "Serilog", "3.1.1"); err != nil {
		t.Fatal(err)
	}
	deps, err := readManifestDeps(csproj)
	if err != nil {
		t.Fatal(err)
	}
	if len(deps) != 1 || deps[0].Version != "3.1.1" {
		t.Fatalf("got %+v, want version 3.1.1 from props", deps)
	}
}

func TestLaunchProfiles(t *testing.T) {
	root := t.TempDir()
	csproj := filepath.Join(root, "App.csproj")
	writeFile(t, csproj, `<Project Sdk="Microsoft.NET.Sdk"/>`)
	writeFile(t, filepath.Join(root, "Properties", "launchSettings.json"), `{
  "profiles": {
    "http": { "commandName": "Project" },
    "https": { "commandName": "Project" }
  }
}`)

	scripts, err := ListScripts(csproj)
	if err != nil {
		t.Fatal(err)
	}
	var profiles []string
	for _, s := range scripts {
		if s.Command == "dotnet run --launch-profile http" {
			profiles = append(profiles, "http")
		}
		if s.Command == "dotnet run --launch-profile https" {
			profiles = append(profiles, "https")
		}
	}
	if len(profiles) != 2 {
		t.Fatalf("expected http and https launch-profile scripts, got %+v", scripts)
	}
}

func TestResolveArgsStandardAndProfileWithSpaces(t *testing.T) {
	root := t.TempDir()
	csproj := filepath.Join(root, "App.csproj")
	writeFile(t, csproj, `<Project Sdk="Microsoft.NET.Sdk"/>`)
	writeFile(t, filepath.Join(root, "Properties", "launchSettings.json"), `{
  "profiles": {
    "IIS Express": { "commandName": "IISExpress" }
  }
}`)

	if got := ResolveArgs(csproj, "watch"); len(got) != 2 || got[0] != "watch" || got[1] != "run" {
		t.Fatalf("watch args = %v, want [watch run]", got)
	}
	got := ResolveArgs(csproj, "IIS Express (profile)")
	want := []string{"run", "--launch-profile", "IIS Express"}
	if len(got) != len(want) {
		t.Fatalf("profile args = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("profile args = %v, want %v", got, want)
		}
	}
	// Unknown target falls back to whitespace split (custom command).
	if got := ResolveArgs(csproj, "run --no-build"); len(got) != 2 || got[0] != "run" || got[1] != "--no-build" {
		t.Fatalf("custom args = %v, want [run --no-build]", got)
	}
}

func TestPackageTableRows(t *testing.T) {
	out := []byte(`Project ` + "`App`" + ` has the following updates to its packages
   [net8.0]:
   Top-level Package      Requested   Resolved   Latest
   > Newtonsoft.Json      13.0.1      13.0.1     13.0.3
   > Serilog              3.1.0       3.1.0      3.1.1
`)
	rows := packageTableRows(out)
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2: %+v", len(rows), rows)
	}
	if rows[0][1] != "Newtonsoft.Json" || rows[0][4] != "13.0.3" {
		t.Errorf("row 0 = %+v", rows[0])
	}
}
