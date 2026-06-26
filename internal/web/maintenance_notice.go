package web

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/OurNeZt/ournezt-web/internal/config"
)

type MaintenanceNotice struct {
	Enabled  bool   `json:"enabled"`
	Level    string `json:"level"`
	Title    string `json:"title"`
	Message  string `json:"message"`
	StartsAt string `json:"starts_at"`
	EndsAt   string `json:"ends_at"`
}

func (a *App) maintenanceNotice() *MaintenanceNotice {
	notice := maintenanceNoticeFromConfig(a.cfg.MaintenanceNotice)
	if fileNotice := maintenanceNoticeFromFile(a.cfg.MaintenanceNotice.FilePath); fileNotice != nil {
		notice = fileNotice
	}
	if !notice.Enabled || strings.TrimSpace(notice.Message) == "" {
		return nil
	}
	notice.Level = maintenanceNoticeLevel(notice.Level)
	notice.Title = strings.TrimSpace(notice.Title)
	notice.Message = strings.TrimSpace(notice.Message)
	notice.StartsAt = strings.TrimSpace(notice.StartsAt)
	notice.EndsAt = strings.TrimSpace(notice.EndsAt)
	return notice
}

func maintenanceNoticeFromConfig(cfg config.MaintenanceNoticeConfig) *MaintenanceNotice {
	return &MaintenanceNotice{
		Enabled:  cfg.Enabled,
		Level:    cfg.Level,
		Title:    cfg.Title,
		Message:  cfg.Message,
		StartsAt: cfg.StartsAt,
		EndsAt:   cfg.EndsAt,
	}
}

func maintenanceNoticeFromFile(path string) *MaintenanceNotice {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var notice MaintenanceNotice
	if err := json.Unmarshal(content, &notice); err != nil {
		return nil
	}
	return &notice
}

func maintenanceNoticeLevel(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "info", "success", "warning", "error":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "warning"
	}
}
