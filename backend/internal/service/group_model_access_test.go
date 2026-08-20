package service

import (
	"reflect"
	"testing"
)

func TestGroupDisabledModels(t *testing.T) {
	models := normalizeDisabledModels([]string{" gpt-5.4 ", "GPT-5.4", "", "models/gemini-3-pro"})
	if want := []string{"gpt-5.4", "models/gemini-3-pro"}; !reflect.DeepEqual(models, want) {
		t.Fatalf("normalizeDisabledModels() = %v, want %v", models, want)
	}

	group := &Group{DisabledModels: models}
	for _, model := range []string{"gpt-5.4", "GPT-5.4", "gemini-3-pro", "models/gemini-3-pro"} {
		if !group.IsModelDisabled(model) {
			t.Fatalf("expected %q to be disabled", model)
		}
	}
	if group.IsModelDisabled("gpt-5.4-mini") {
		t.Fatal("exact disabled model must not block a different model")
	}
	if got := group.FilterDisabledModels([]string{"gpt-5.4", "gpt-5.4-mini", "gemini-3-pro"}); !reflect.DeepEqual(got, []string{"gpt-5.4-mini"}) {
		t.Fatalf("FilterDisabledModels() = %v", got)
	}
}
