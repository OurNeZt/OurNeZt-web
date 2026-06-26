package web

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/OurNeZt/ournezt-web/internal/config"
)

func TestMaintenanceNoticeUsesFileOverride(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notice.json")
	if err := os.WriteFile(path, []byte(`{
		"enabled": true,
		"level": "warning",
		"title": "Planned maintenance",
		"message": "OurNeZt may be unavailable during the ingress migration.",
		"starts_at": "28 Jun 2026, 12:00 AM SGT",
		"ends_at": "28 Jun 2026, approximately 2:00 AM SGT"
	}`), 0o600); err != nil {
		t.Fatalf("write notice file: %v", err)
	}

	app := &App{
		cfg: config.Config{
			MaintenanceNotice: config.MaintenanceNoticeConfig{
				FilePath: path,
				Enabled:  false,
			},
		},
	}

	notice := app.maintenanceNotice()
	if notice == nil {
		t.Fatal("expected notice")
	}
	if notice.Title != "Planned maintenance" {
		t.Fatalf("title = %q", notice.Title)
	}
	if notice.StartsAt != "28 Jun 2026, 12:00 AM SGT" {
		t.Fatalf("starts at = %q", notice.StartsAt)
	}
}

func TestMaintenanceNoticeHiddenWithoutMessage(t *testing.T) {
	app := &App{
		cfg: config.Config{
			MaintenanceNotice: config.MaintenanceNoticeConfig{
				Enabled: true,
				Title:   "Planned maintenance",
			},
		},
	}

	if notice := app.maintenanceNotice(); notice != nil {
		t.Fatalf("expected no notice, got %#v", notice)
	}
}

func TestMaintenanceNoticeDefaultsUnknownLevelToWarning(t *testing.T) {
	app := &App{
		cfg: config.Config{
			MaintenanceNotice: config.MaintenanceNoticeConfig{
				Enabled: true,
				Level:   "loud",
				Message: "Short interruption expected.",
			},
		},
	}

	notice := app.maintenanceNotice()
	if notice == nil {
		t.Fatal("expected notice")
	}
	if notice.Level != "warning" {
		t.Fatalf("level = %q", notice.Level)
	}
}
