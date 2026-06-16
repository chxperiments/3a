package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfigPath(t *testing.T) {
	path := DefaultConfigPath()
	if path == "" {
		t.Fatal("DefaultConfigPath() returned empty string")
	}
	if filepath.Base(path) != "config.yaml" {
		t.Errorf("expected path to end with config.yaml, got %s", path)
	}
	dir := filepath.Base(filepath.Dir(path))
	if dir != ".3a" {
		t.Errorf("expected parent directory to be .3a, got %s", dir)
	}
}

func TestLoadValidConfig(t *testing.T) {
	content := `db_path: /tmp/test.db
profiles:
  - name: prod-aws
    display_name: "Production AWS"
    provider: aws
    regions:
      - us-east-1
      - us-west-2
  - name: staging-oci
    display_name: "Staging OCI"
    provider: oci
    regions:
      - us-ashburn-1
`

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if cfg.DBPath != "/tmp/test.db" {
		t.Errorf("DBPath = %q, want %q", cfg.DBPath, "/tmp/test.db")
	}
	if len(cfg.Profiles) != 2 {
		t.Fatalf("len(Profiles) = %d, want 2", len(cfg.Profiles))
	}

	p := cfg.Profiles[0]
	if p.Name != "prod-aws" {
		t.Errorf("Profiles[0].Name = %q, want %q", p.Name, "prod-aws")
	}
	if p.DisplayName != "Production AWS" {
		t.Errorf("Profiles[0].DisplayName = %q, want %q", p.DisplayName, "Production AWS")
	}
	if p.Provider != "aws" {
		t.Errorf("Profiles[0].Provider = %q, want %q", p.Provider, "aws")
	}
	if len(p.Regions) != 2 {
		t.Fatalf("len(Profiles[0].Regions) = %d, want 2", len(p.Regions))
	}
	if p.Regions[0] != "us-east-1" || p.Regions[1] != "us-west-2" {
		t.Errorf("Profiles[0].Regions = %v, want [us-east-1 us-west-2]", p.Regions)
	}

	p2 := cfg.Profiles[1]
	if p2.Provider != "oci" {
		t.Errorf("Profiles[1].Provider = %q, want %q", p2.Provider, "oci")
	}
}

func TestLoadNonExistentFile(t *testing.T) {
	_, err := Load("/nonexistent/path/config.yaml")
	if err == nil {
		t.Fatal("Load() should return error for non-existent file")
	}

	var cfgErr *ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("expected *ConfigError, got %T", err)
	}
	if cfgErr.Path != "/nonexistent/path/config.yaml" {
		t.Errorf("ConfigError.Path = %q, want %q", cfgErr.Path, "/nonexistent/path/config.yaml")
	}
}

func TestLoadMalformedYAML(t *testing.T) {
	content := `db_path: /tmp/test.db
profiles:
  - name: [invalid yaml
    this is: broken
`

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("Load() should return error for malformed YAML")
	}

	var cfgErr *ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("expected *ConfigError, got %T", err)
	}
}

func TestSaveAndLoad(t *testing.T) {
	cfg := &Config{
		DBPath: "~/.3a/assessments.db",
		Profiles: []AccountProfile{
			{
				Name:        "test-aws",
				DisplayName: "Test AWS Account",
				Provider:    "aws",
				Regions:     []string{"us-east-1", "eu-west-1"},
			},
		},
	}

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "subdir", "config.yaml")

	if err := Save(cfgPath, cfg); err != nil {
		t.Fatalf("Save() returned error: %v", err)
	}

	loaded, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if loaded.DBPath != cfg.DBPath {
		t.Errorf("DBPath = %q, want %q", loaded.DBPath, cfg.DBPath)
	}
	if len(loaded.Profiles) != 1 {
		t.Fatalf("len(Profiles) = %d, want 1", len(loaded.Profiles))
	}

	p := loaded.Profiles[0]
	if p.Name != "test-aws" {
		t.Errorf("Name = %q, want %q", p.Name, "test-aws")
	}
	if p.Provider != "aws" {
		t.Errorf("Provider = %q, want %q", p.Provider, "aws")
	}
	if len(p.Regions) != 2 || p.Regions[0] != "us-east-1" || p.Regions[1] != "eu-west-1" {
		t.Errorf("Regions = %v, want [us-east-1 eu-west-1]", p.Regions)
	}
}

func TestSaveCreatesDirectories(t *testing.T) {
	cfg := &Config{DBPath: "/tmp/test.db"}

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "a", "b", "c", "config.yaml")

	if err := Save(cfgPath, cfg); err != nil {
		t.Fatalf("Save() returned error: %v", err)
	}

	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		t.Fatal("config file was not created")
	}
}

func TestConfigErrorImplementsError(t *testing.T) {
	inner := errors.New("underlying error")
	cfgErr := &ConfigError{
		Path:    "/some/path",
		Message: "something went wrong",
		Err:     inner,
	}

	msg := cfgErr.Error()
	if msg == "" {
		t.Fatal("Error() returned empty string")
	}
	if !errors.Is(cfgErr, inner) {
		t.Error("errors.Is should find the inner error")
	}
}

func TestConfigErrorWithoutInner(t *testing.T) {
	cfgErr := &ConfigError{
		Path:    "/some/path",
		Message: "something went wrong",
	}

	msg := cfgErr.Error()
	expected := "config error (/some/path): something went wrong"
	if msg != expected {
		t.Errorf("Error() = %q, want %q", msg, expected)
	}
}

func TestLoadEmptyConfig(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() returned error for empty file: %v", err)
	}

	if cfg.DBPath != "" {
		t.Errorf("DBPath = %q, want empty", cfg.DBPath)
	}
	if len(cfg.Profiles) != 0 {
		t.Errorf("len(Profiles) = %d, want 0", len(cfg.Profiles))
	}
}
