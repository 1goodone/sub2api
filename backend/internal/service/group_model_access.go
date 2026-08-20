package service

import "strings"

func normalizeDisabledModels(models []string) []string {
	seen := make(map[string]struct{}, len(models))
	out := make([]string, 0, len(models))
	for _, model := range models {
		model = strings.TrimSpace(model)
		key := disabledModelKey(model)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, model)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func disabledModelKey(model string) string {
	model = strings.ToLower(strings.TrimSpace(model))
	return strings.TrimPrefix(model, "models/")
}

func (g *Group) IsModelDisabled(model string) bool {
	if g == nil {
		return false
	}
	key := disabledModelKey(model)
	if key == "" {
		return false
	}
	for _, disabled := range g.DisabledModels {
		if disabledModelKey(disabled) == key {
			return true
		}
	}
	return false
}

func (g *Group) FilterDisabledModels(models []string) []string {
	if g == nil || len(g.DisabledModels) == 0 || len(models) == 0 {
		return models
	}
	filtered := make([]string, 0, len(models))
	for _, model := range models {
		if !g.IsModelDisabled(model) {
			filtered = append(filtered, model)
		}
	}
	return filtered
}
