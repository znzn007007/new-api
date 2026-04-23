package ratio_setting

import "testing"

func TestPublicGroupTagRatioJSONLifecycle(t *testing.T) {
	original := PublicGroupTagRatio2JSONString()
	t.Cleanup(func() {
		_ = UpdatePublicGroupTagRatioByJSONString(original)
	})

	valid := `{"ask-public":{"GPT":1.2,"Claude 第三方":1.5}}`
	if err := UpdatePublicGroupTagRatioByJSONString(valid); err != nil {
		t.Fatalf("expected valid public-group tag ratio json, got error: %v", err)
	}

	ratio, ok := GetPublicGroupTagRatio("ask-public", "GPT")
	if !ok {
		t.Fatalf("expected GPT ratio to exist")
	}
	if ratio != 1.2 {
		t.Fatalf("expected ratio 1.2, got %v", ratio)
	}
}

func TestCheckPublicGroupTagRatioRejectsNegativeRatio(t *testing.T) {
	err := CheckPublicGroupTagRatio(`{"ask-public":{"GPT":-1}}`)
	if err == nil {
		t.Fatalf("expected negative ratio to be rejected")
	}
}

func TestPublicGroupModelTagOverrideJSONLifecycle(t *testing.T) {
	original := PublicGroupModelTagOverride2JSONString()
	t.Cleanup(func() {
		_ = UpdatePublicGroupModelTagOverrideByJSONString(original)
	})

	valid := `{"ask-public":{"gpt-4o":"GPT","claude-3-7-sonnet":"Claude Code"}}`
	if err := UpdatePublicGroupModelTagOverrideByJSONString(valid); err != nil {
		t.Fatalf("expected valid public-group model-tag override json, got error: %v", err)
	}

	tag, ok := GetPublicGroupModelTagOverride("ask-public", "gpt-4o")
	if !ok {
		t.Fatalf("expected gpt-4o override to exist")
	}
	if tag != "GPT" {
		t.Fatalf("expected override tag GPT, got %q", tag)
	}
}

func TestUpdatePublicGroupModelTagOverrideAllowsEmptyString(t *testing.T) {
	original := PublicGroupModelTagOverride2JSONString()
	t.Cleanup(func() {
		_ = UpdatePublicGroupModelTagOverrideByJSONString(original)
	})

	if err := UpdatePublicGroupModelTagOverrideByJSONString(""); err != nil {
		t.Fatalf("expected empty override json to clear config, got error: %v", err)
	}
}
