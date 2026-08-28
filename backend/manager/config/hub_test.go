package config

import "testing"

func TestLoadHubConfigDefaultsClosed(t *testing.T) {
	for _, key := range []string{"A2A888_HUB_MODE", "A2A888_HUB_ID", "A2A888_HUB_BOOTSTRAP_TOKEN", "A2A888_HUB_PUBLIC_CONFIRM"} {
		t.Setenv(key, "")
	}
	cfg, err := LoadHubConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Mode != HubModeClosed || cfg.RegistrationEnabled {
		t.Fatalf("default hub config = %+v", cfg)
	}
}

func TestLoadHubConfigOpenRequiresBootstrapToken(t *testing.T) {
	t.Setenv("A2A888_HUB_MODE", "open")
	t.Setenv("A2A888_HUB_ID", "hub-private")
	t.Setenv("A2A888_HUB_BOOTSTRAP_TOKEN", "")
	if _, err := LoadHubConfig(); err == nil {
		t.Fatal("open mode must require a bootstrap token")
	}
	t.Setenv("A2A888_HUB_BOOTSTRAP_TOKEN", "local-bootstrap-token")
	cfg, err := LoadHubConfig()
	if err != nil || cfg.Mode != HubModeOpen || !cfg.RegistrationEnabled {
		t.Fatalf("open hub config = %+v, err=%v", cfg, err)
	}
}

func TestLoadHubConfigPublicRequiresExplicitConfirmation(t *testing.T) {
	t.Setenv("A2A888_HUB_MODE", "public")
	t.Setenv("A2A888_HUB_ID", "hub-public")
	t.Setenv("A2A888_HUB_PUBLIC_CONFIRM", "")
	if _, err := LoadHubConfig(); err == nil {
		t.Fatal("public mode must require explicit confirmation")
	}
	t.Setenv("A2A888_HUB_PUBLIC_CONFIRM", "true")
	cfg, err := LoadHubConfig()
	if err != nil || cfg.Mode != HubModePublic || !cfg.RegistrationEnabled {
		t.Fatalf("public hub config = %+v, err=%v", cfg, err)
	}
}
