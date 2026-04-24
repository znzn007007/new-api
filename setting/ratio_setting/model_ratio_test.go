package ratio_setting

import "testing"

func restoreCompletionRatioMap(t *testing.T) func() {
	t.Helper()

	original := CompletionRatio2JSONString()
	return func() {
		if err := UpdateCompletionRatioByJSONString(original); err != nil {
			t.Fatalf("restore completion ratio map: %v", err)
		}
	}
}

func TestGetCompletionRatioUsesExplicitOverrideForGPTModels(t *testing.T) {
	defer restoreCompletionRatioMap(t)()

	if err := UpdateCompletionRatioByJSONString(`{"gpt-5.4":3.5}`); err != nil {
		t.Fatalf("set completion ratio override: %v", err)
	}

	if got := GetCompletionRatio("gpt-5.4"); got != 3.5 {
		t.Fatalf("expected explicit completion ratio override 3.5, got %v", got)
	}

	info := GetCompletionRatioInfo("gpt-5.4")
	if info.Ratio != 3.5 {
		t.Fatalf("expected completion ratio info to expose override 3.5, got %v", info.Ratio)
	}
	if info.Locked {
		t.Fatalf("expected GPT completion ratio override to remain editable")
	}
}

func TestGetCompletionRatioInfoKeepsGPTDefaultsEditable(t *testing.T) {
	defer restoreCompletionRatioMap(t)()

	if err := UpdateCompletionRatioByJSONString(`{}`); err != nil {
		t.Fatalf("clear completion ratio overrides: %v", err)
	}

	info := GetCompletionRatioInfo("gpt-5.4")
	if info.Ratio != 6 {
		t.Fatalf("expected default GPT completion ratio 6, got %v", info.Ratio)
	}
	if info.Locked {
		t.Fatalf("expected default GPT completion ratio to be editable")
	}
}

func TestGetCompletionRatioInfoKeepsNonGPTHardcodedRatiosLocked(t *testing.T) {
	defer restoreCompletionRatioMap(t)()

	if err := UpdateCompletionRatioByJSONString(`{}`); err != nil {
		t.Fatalf("clear completion ratio overrides: %v", err)
	}

	info := GetCompletionRatioInfo("claude-3-5-sonnet")
	if info.Ratio != 5 {
		t.Fatalf("expected claude hardcoded completion ratio 5, got %v", info.Ratio)
	}
	if !info.Locked {
		t.Fatalf("expected non-GPT hardcoded completion ratio to remain locked")
	}
}
