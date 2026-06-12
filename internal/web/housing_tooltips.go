package web

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

type housingTooltipEntry struct {
	Label       string `json:"label"`
	Meaning     string `json:"meaning"`
	Calculation string `json:"calculation"`
	Assumptions string `json:"assumptions"`
	Source      string `json:"source"`
}

//go:embed content/housing_tooltips.json
var housingTooltipCatalogJSON []byte

var housingTooltipCatalog = mustLoadHousingTooltipCatalog()

func mustLoadHousingTooltipCatalog() map[string]housingTooltipEntry {
	catalog := make(map[string]housingTooltipEntry)
	if err := json.Unmarshal(housingTooltipCatalogJSON, &catalog); err != nil {
		panic(fmt.Errorf("load housing tooltip catalog: %w", err))
	}
	return catalog
}

func housingTooltip(key string) string {
	entry, ok := housingTooltipCatalog[normalizeLookup(key)]
	if !ok {
		return ""
	}

	parts := make([]string, 0, 4)
	if text := sentenceText(entry.Meaning); text != "" {
		parts = append(parts, text)
	}
	if text := sentenceText(entry.Calculation); text != "" {
		parts = append(parts, text)
	}
	if text := sentenceText(entry.Assumptions); text != "" {
		parts = append(parts, text)
	}
	if text := sentenceText(entry.Source); text != "" {
		parts = append(parts, text)
	}
	return strings.Join(parts, " ")
}

func sentenceText(value string) string {
	clean := strings.TrimSpace(value)
	if clean == "" {
		return ""
	}
	if strings.HasSuffix(clean, ".") || strings.HasSuffix(clean, "!") || strings.HasSuffix(clean, "?") {
		return clean
	}
	return clean + "."
}

func tooltipAriaLabel(label string) string {
	clean := strings.TrimSpace(label)
	if clean == "" {
		return "Show help"
	}
	return "Show help for " + clean
}

func dict(values ...any) (map[string]any, error) {
	if len(values)%2 != 0 {
		return nil, fmt.Errorf("dict requires even number of arguments")
	}

	result := make(map[string]any, len(values)/2)
	for i := 0; i < len(values); i += 2 {
		key, ok := values[i].(string)
		if !ok {
			return nil, fmt.Errorf("dict keys must be strings")
		}
		result[key] = values[i+1]
	}
	return result, nil
}
