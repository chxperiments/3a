package config

import (
	"errors"
	"testing"
)

func TestGetProfile_Found(t *testing.T) {
	cfg := &Config{
		Profiles: []AccountProfile{
			{Name: "dev", Provider: "aws", DisplayName: "Development"},
			{Name: "prod", Provider: "oci", DisplayName: "Production"},
		},
	}

	profile, err := GetProfile(cfg, "prod")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if profile.Name != "prod" {
		t.Errorf("expected profile name %q, got %q", "prod", profile.Name)
	}
	if profile.Provider != "oci" {
		t.Errorf("expected provider %q, got %q", "oci", profile.Provider)
	}
	if profile.DisplayName != "Production" {
		t.Errorf("expected display name %q, got %q", "Production", profile.DisplayName)
	}
}

func TestGetProfile_NotFound(t *testing.T) {
	cfg := &Config{
		Profiles: []AccountProfile{
			{Name: "dev", Provider: "aws"},
			{Name: "staging", Provider: "oci"},
		},
	}

	_, err := GetProfile(cfg, "prod")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var pnfErr *ProfileNotFoundError
	if !errors.As(err, &pnfErr) {
		t.Fatalf("expected ProfileNotFoundError, got %T: %v", err, err)
	}
	if pnfErr.RequestedName != "prod" {
		t.Errorf("expected requested name %q, got %q", "prod", pnfErr.RequestedName)
	}
	if len(pnfErr.AvailableNames) != 2 {
		t.Fatalf("expected 2 available names, got %d", len(pnfErr.AvailableNames))
	}
	if pnfErr.AvailableNames[0] != "dev" || pnfErr.AvailableNames[1] != "staging" {
		t.Errorf("expected available names [dev staging], got %v", pnfErr.AvailableNames)
	}
}

func TestGetProfile_EmptyConfig(t *testing.T) {
	cfg := &Config{}

	_, err := GetProfile(cfg, "anything")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var pnfErr *ProfileNotFoundError
	if !errors.As(err, &pnfErr) {
		t.Fatalf("expected ProfileNotFoundError, got %T: %v", err, err)
	}
	if len(pnfErr.AvailableNames) != 0 {
		t.Errorf("expected 0 available names, got %d", len(pnfErr.AvailableNames))
	}
}

func TestListProfiles(t *testing.T) {
	cfg := &Config{
		Profiles: []AccountProfile{
			{Name: "alpha", Provider: "aws"},
			{Name: "beta", Provider: "oci"},
			{Name: "gamma", Provider: "aws"},
		},
	}

	profiles := ListProfiles(cfg)
	if len(profiles) != 3 {
		t.Fatalf("expected 3 profiles, got %d", len(profiles))
	}
	if profiles[0].Name != "alpha" {
		t.Errorf("expected first profile %q, got %q", "alpha", profiles[0].Name)
	}
	if profiles[2].Name != "gamma" {
		t.Errorf("expected third profile %q, got %q", "gamma", profiles[2].Name)
	}
}

func TestListProfiles_Empty(t *testing.T) {
	cfg := &Config{}

	profiles := ListProfiles(cfg)
	if profiles != nil {
		t.Errorf("expected nil for empty profiles, got %v", profiles)
	}
}

func TestAddProfile(t *testing.T) {
	cfg := &Config{
		Profiles: []AccountProfile{
			{Name: "existing", Provider: "aws"},
		},
	}

	newProfile := AccountProfile{
		Name:        "new-profile",
		DisplayName: "New Profile",
		Provider:    "oci",
		Regions:     []string{"us-ashburn-1", "eu-frankfurt-1"},
	}

	AddProfile(cfg, newProfile)

	if len(cfg.Profiles) != 2 {
		t.Fatalf("expected 2 profiles after add, got %d", len(cfg.Profiles))
	}
	added := cfg.Profiles[1]
	if added.Name != "new-profile" {
		t.Errorf("expected added profile name %q, got %q", "new-profile", added.Name)
	}
	if added.Provider != "oci" {
		t.Errorf("expected provider %q, got %q", "oci", added.Provider)
	}
	if len(added.Regions) != 2 {
		t.Errorf("expected 2 regions, got %d", len(added.Regions))
	}
}

func TestAddProfile_ToEmpty(t *testing.T) {
	cfg := &Config{}

	AddProfile(cfg, AccountProfile{Name: "first", Provider: "aws"})

	if len(cfg.Profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(cfg.Profiles))
	}
	if cfg.Profiles[0].Name != "first" {
		t.Errorf("expected profile name %q, got %q", "first", cfg.Profiles[0].Name)
	}
}

func TestProfileNotFoundError_Message(t *testing.T) {
	err := &ProfileNotFoundError{
		RequestedName:  "missing",
		AvailableNames: []string{"a", "b"},
	}

	msg := err.Error()
	if msg == "" {
		t.Error("expected non-empty error message")
	}
	// Verify the message contains the requested name and available names
	if !containsSubstring(msg, "missing") {
		t.Errorf("error message should contain requested name, got: %s", msg)
	}
	if !containsSubstring(msg, "a") || !containsSubstring(msg, "b") {
		t.Errorf("error message should contain available names, got: %s", msg)
	}
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && stringContains(s, substr))
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
