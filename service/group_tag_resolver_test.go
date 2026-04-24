package service

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupGroupTagResolverTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	previousDB := model.DB
	previousLogDB := model.LOG_DB

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.MemoryCacheEnabled = true

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
	}

	model.DB = db
	model.LOG_DB = db

	if err := db.AutoMigrate(&model.Channel{}, &model.Ability{}); err != nil {
		t.Fatalf("failed to migrate tables: %v", err)
	}

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
		model.DB = previousDB
		model.LOG_DB = previousLogDB
	})

	return db
}

func seedGroupTagResolverChannel(t *testing.T, db *gorm.DB, id int, group, modelName, tag string) {
	t.Helper()

	var tagPtr *string
	if tag != "" {
		tagPtr = &tag
	}
	weight := uint(1)
	priority := int64(0)
	autoBan := 1
	channel := &model.Channel{
		Id:       id,
		Type:     1,
		Key:      "test-key",
		Status:   common.ChannelStatusEnabled,
		Name:     fmt.Sprintf("channel-%d", id),
		Weight:   &weight,
		Priority: &priority,
		AutoBan:  &autoBan,
		Group:    group,
		Models:   modelName,
		Tag:      tagPtr,
	}
	if err := db.Create(channel).Error; err != nil {
		t.Fatalf("failed to create channel: %v", err)
	}
	if err := channel.AddAbilities(nil); err != nil {
		t.Fatalf("failed to add abilities: %v", err)
	}
}

func withResolverSettingsReset(t *testing.T) {
	t.Helper()
	originalGroupRatio := ratio_setting.GroupRatio2JSONString()
	originalTagRatio := ratio_setting.PublicGroupTagRatio2JSONString()
	originalModelTag := ratio_setting.PublicGroupModelTagOverride2JSONString()
	t.Cleanup(func() {
		_ = ratio_setting.UpdateGroupRatioByJSONString(originalGroupRatio)
		_ = ratio_setting.UpdatePublicGroupTagRatioByJSONString(originalTagRatio)
		_ = ratio_setting.UpdatePublicGroupModelTagOverrideByJSONString(originalModelTag)
	})
}

func TestResolveGroupBillingOverrideWins(t *testing.T) {
	db := setupGroupTagResolverTestDB(t)
	withResolverSettingsReset(t)

	seedGroupTagResolverChannel(t, db, 1, "ask-public", "shared-model", "GPT")
	seedGroupTagResolverChannel(t, db, 2, "ask-public", "shared-model", "Claude Code")
	model.InitChannelCache()

	if err := ratio_setting.UpdateGroupRatioByJSONString(`{"ask-public":1}`); err != nil {
		t.Fatalf("failed to seed group ratio: %v", err)
	}
	if err := ratio_setting.UpdatePublicGroupModelTagOverrideByJSONString(`{"ask-public":{"shared-model":"GPT"}}`); err != nil {
		t.Fatalf("failed to seed model-tag override: %v", err)
	}

	resolution, err := ResolveGroupBilling("ask-public", "default", "shared-model")
	if err != nil {
		t.Fatalf("expected override resolution to succeed, got error: %v", err)
	}
	if resolution.MatchedTag != "GPT" {
		t.Fatalf("expected override tag GPT, got %q", resolution.MatchedTag)
	}
	if resolution.RouteTag != "GPT" {
		t.Fatalf("expected route tag GPT, got %q", resolution.RouteTag)
	}
	if !resolution.RouteTagStrict {
		t.Fatalf("expected override route to be strict")
	}
	if !resolution.ResolvedByOverride {
		t.Fatalf("expected resolution to be marked as override")
	}
}

func TestResolveGroupBillingAutoDerivesUniqueTagAndRatio(t *testing.T) {
	db := setupGroupTagResolverTestDB(t)
	withResolverSettingsReset(t)

	seedGroupTagResolverChannel(t, db, 1, "ask-public", "single-model", "Claude 第三方")
	model.InitChannelCache()

	if err := ratio_setting.UpdateGroupRatioByJSONString(`{"ask-public":1}`); err != nil {
		t.Fatalf("failed to seed group ratio: %v", err)
	}
	if err := ratio_setting.UpdatePublicGroupTagRatioByJSONString(`{"ask-public":{"Claude 第三方":1.8}}`); err != nil {
		t.Fatalf("failed to seed tag ratio: %v", err)
	}

	resolution, err := ResolveGroupBilling("ask-public", "default", "single-model")
	if err != nil {
		t.Fatalf("expected unique-tag resolution to succeed, got error: %v", err)
	}
	if resolution.MatchedTag != "Claude 第三方" {
		t.Fatalf("expected resolved tag Claude 第三方, got %q", resolution.MatchedTag)
	}
	if resolution.RouteTag != "Claude 第三方" {
		t.Fatalf("expected route tag Claude 第三方, got %q", resolution.RouteTag)
	}
	if resolution.RouteTagStrict {
		t.Fatalf("expected inferred route tag to be non-strict")
	}
	if resolution.EffectiveRatio != 1.8 {
		t.Fatalf("expected effective ratio 1.8, got %v", resolution.EffectiveRatio)
	}
	if resolution.BillingRatioFallback {
		t.Fatalf("expected tag-specific ratio, not fallback")
	}
}

func TestResolveGroupBillingFallsBackWhenNoTagExists(t *testing.T) {
	db := setupGroupTagResolverTestDB(t)
	withResolverSettingsReset(t)

	seedGroupTagResolverChannel(t, db, 1, "ask-public", "untagged-model", "")
	model.InitChannelCache()

	if err := ratio_setting.UpdateGroupRatioByJSONString(`{"ask-public":1.25}`); err != nil {
		t.Fatalf("failed to seed group ratio: %v", err)
	}

	resolution, err := ResolveGroupBilling("ask-public", "default", "untagged-model")
	if err != nil {
		t.Fatalf("expected fallback resolution to succeed, got error: %v", err)
	}
	if resolution.MatchedTag != "" {
		t.Fatalf("expected no resolved tag, got %q", resolution.MatchedTag)
	}
	if resolution.RouteTag != "" {
		t.Fatalf("expected no route tag, got %q", resolution.RouteTag)
	}
	if resolution.RouteTagStrict {
		t.Fatalf("expected route tag to be non-strict")
	}
	if resolution.EffectiveRatio != 1.25 {
		t.Fatalf("expected fallback ratio 1.25, got %v", resolution.EffectiveRatio)
	}
	if !resolution.BillingRatioFallback {
		t.Fatalf("expected fallback marker to be true")
	}
}

func TestResolveGroupBillingLeavesAmbiguousTagsUnlocked(t *testing.T) {
	db := setupGroupTagResolverTestDB(t)
	withResolverSettingsReset(t)

	seedGroupTagResolverChannel(t, db, 1, "ask-public", "ambiguous-model", "GPT")
	seedGroupTagResolverChannel(t, db, 2, "ask-public", "ambiguous-model", "Claude Code")
	model.InitChannelCache()

	if err := ratio_setting.UpdateGroupRatioByJSONString(`{"ask-public":1}`); err != nil {
		t.Fatalf("failed to seed group ratio: %v", err)
	}

	resolution, err := ResolveGroupBilling("ask-public", "default", "ambiguous-model")
	if err != nil {
		t.Fatalf("expected ambiguous tag resolution to fall back, got error: %v", err)
	}
	if resolution.MatchedTag != "" {
		t.Fatalf("expected no billing tag for ambiguous routing, got %q", resolution.MatchedTag)
	}
	if resolution.RouteTag != "" {
		t.Fatalf("expected no route tag for ambiguous routing, got %q", resolution.RouteTag)
	}
	if resolution.RouteTagStrict {
		t.Fatalf("expected ambiguous routing to stay non-strict")
	}
}

func TestResolveGroupBillingRejectsMissingOverrideTag(t *testing.T) {
	db := setupGroupTagResolverTestDB(t)
	withResolverSettingsReset(t)

	seedGroupTagResolverChannel(t, db, 1, "ask-public", "shared-model", "GPT")
	model.InitChannelCache()

	if err := ratio_setting.UpdateGroupRatioByJSONString(`{"ask-public":1}`); err != nil {
		t.Fatalf("failed to seed group ratio: %v", err)
	}
	if err := ratio_setting.UpdatePublicGroupModelTagOverrideByJSONString(`{"ask-public":{"shared-model":"Claude Code"}}`); err != nil {
		t.Fatalf("failed to seed model-tag override: %v", err)
	}

	_, err := ResolveGroupBilling("ask-public", "default", "shared-model")
	if err == nil {
		t.Fatalf("expected missing override tag to fail")
	}
}
