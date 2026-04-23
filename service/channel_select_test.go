package service

import (
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func seedGroupTagResolverChannelWithPriority(t *testing.T, db *gorm.DB, id int, group, modelName, tag string, priority int64) {
	t.Helper()

	var tagPtr *string
	if tag != "" {
		tagPtr = &tag
	}
	weight := uint(1)
	autoBan := 1
	channel := &model.Channel{
		Id:       id,
		Type:     1,
		Key:      fmt.Sprintf("test-key-%d", id),
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

func TestGetRandomSatisfiedChannelByResolutionStrictOverrideUsesTaggedPool(t *testing.T) {
	db := setupGroupTagResolverTestDB(t)
	withResolverSettingsReset(t)

	seedGroupTagResolverChannelWithPriority(t, db, 1, "ask-public", "shared-model", "GPT", 0)
	seedGroupTagResolverChannelWithPriority(t, db, 2, "ask-public", "shared-model", "", 10)
	model.InitChannelCache()

	resolution := &GroupBillingResolution{RouteTag: "GPT", RouteTagStrict: true}
	channel, err := GetRandomSatisfiedChannelByResolution("ask-public", "shared-model", resolution, 0)
	if err != nil {
		t.Fatalf("expected strict route selection to succeed, got error: %v", err)
	}
	if channel == nil || channel.Id != 1 {
		t.Fatalf("expected strict route selection to use tagged channel 1, got %#v", channel)
	}
}

func TestGetRandomSatisfiedChannelByResolutionPrefersTaggedPoolBeforeGeneralFallback(t *testing.T) {
	db := setupGroupTagResolverTestDB(t)
	withResolverSettingsReset(t)

	seedGroupTagResolverChannelWithPriority(t, db, 1, "ask-public", "shared-model", "GPT", 0)
	seedGroupTagResolverChannelWithPriority(t, db, 2, "ask-public", "shared-model", "", 10)
	model.InitChannelCache()

	resolution := &GroupBillingResolution{RouteTag: "GPT", RouteTagStrict: false}
	channel, err := GetRandomSatisfiedChannelByResolution("ask-public", "shared-model", resolution, 0)
	if err != nil {
		t.Fatalf("expected preferred route selection to succeed, got error: %v", err)
	}
	if channel == nil || channel.Id != 1 {
		t.Fatalf("expected preferred route selection to try tagged channel first, got %#v", channel)
	}
}

func TestGetRandomSatisfiedChannelByResolutionFallsBackToGeneralPool(t *testing.T) {
	db := setupGroupTagResolverTestDB(t)
	withResolverSettingsReset(t)

	seedGroupTagResolverChannelWithPriority(t, db, 2, "ask-public", "shared-model", "", 0)
	model.InitChannelCache()

	resolution := &GroupBillingResolution{RouteTag: "GPT", RouteTagStrict: false}
	channel, err := GetRandomSatisfiedChannelByResolution("ask-public", "shared-model", resolution, 0)
	if err != nil {
		t.Fatalf("expected general fallback selection to succeed, got error: %v", err)
	}
	if channel == nil || channel.Id != 2 {
		t.Fatalf("expected general fallback to return channel 2, got %#v", channel)
	}
}

func TestGetRandomSatisfiedChannelByResolutionWithoutRouteTagUsesGeneralPool(t *testing.T) {
	db := setupGroupTagResolverTestDB(t)
	withResolverSettingsReset(t)

	seedGroupTagResolverChannelWithPriority(t, db, 1, "ask-public", "shared-model", "GPT", 0)
	seedGroupTagResolverChannelWithPriority(t, db, 2, "ask-public", "shared-model", "", 10)
	model.InitChannelCache()

	channel, err := GetRandomSatisfiedChannelByResolution("ask-public", "shared-model", &GroupBillingResolution{}, 0)
	if err != nil {
		t.Fatalf("expected general selection to succeed, got error: %v", err)
	}
	if channel == nil || channel.Id != 2 {
		t.Fatalf("expected general selection to use highest-priority general channel 2, got %#v", channel)
	}
}

func TestIsChannelEnabledForResolutionAllowsGeneralFallbackWhenNonStrict(t *testing.T) {
	db := setupGroupTagResolverTestDB(t)
	withResolverSettingsReset(t)

	seedGroupTagResolverChannelWithPriority(t, db, 1, "ask-public", "shared-model", "", 0)
	model.InitChannelCache()

	nonStrict := &GroupBillingResolution{RouteTag: "GPT", RouteTagStrict: false}
	if IsChannelEnabledForResolution("ask-public", "shared-model", nonStrict, 1) {
		t.Fatalf("expected affinity validation to reject non-tagged channel when route tag is resolved")
	}

	strict := &GroupBillingResolution{RouteTag: "GPT", RouteTagStrict: true}
	if IsChannelEnabledForResolution("ask-public", "shared-model", strict, 1) {
		t.Fatalf("expected strict resolution to reject untagged general channel")
	}
}

func withAutoGroupSettingsReset(t *testing.T) {
	t.Helper()
	originalAutoGroups := setting.AutoGroups2JsonString()
	originalUserUsableGroups := setting.UserUsableGroups2JSONString()
	t.Cleanup(func() {
		_ = setting.UpdateAutoGroupsByJsonString(originalAutoGroups)
		_ = setting.UpdateUserUsableGroupsByJSONString(originalUserUsableGroups)
	})
}

func TestCacheGetRandomSatisfiedChannelAutoSkipsBrokenOverrideGroup(t *testing.T) {
	db := setupGroupTagResolverTestDB(t)
	withResolverSettingsReset(t)
	withAutoGroupSettingsReset(t)

	seedGroupTagResolverChannelWithPriority(t, db, 2, "vip", "shared-model", "", 0)
	model.InitChannelCache()

	if err := ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1}`); err != nil {
		t.Fatalf("failed to seed group ratio: %v", err)
	}
	if err := ratio_setting.UpdatePublicGroupModelTagOverrideByJSONString(`{"default":{"shared-model":"GPT"}}`); err != nil {
		t.Fatalf("failed to seed model-tag override: %v", err)
	}
	if err := setting.UpdateAutoGroupsByJsonString(`["default","vip"]`); err != nil {
		t.Fatalf("failed to seed auto groups: %v", err)
	}
	if err := setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组","vip":"vip分组"}`); err != nil {
		t.Fatalf("failed to seed usable groups: %v", err)
	}

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")

	channel, selectGroup, err := CacheGetRandomSatisfiedChannel(&RetryParam{
		Ctx:        ctx,
		TokenGroup: "auto",
		ModelName:  "shared-model",
		Retry:      common.GetPointer(0),
	})
	if err != nil {
		t.Fatalf("expected auto group selection to continue past broken override, got error: %v", err)
	}
	if selectGroup != "vip" {
		t.Fatalf("expected auto group selection to fall through to vip, got %q", selectGroup)
	}
	if channel == nil || channel.Id != 2 {
		t.Fatalf("expected fallback group to return vip channel 2, got %#v", channel)
	}
}

func TestCacheUpdateChannelStatusRemovesTaggedChannelFromCache(t *testing.T) {
	db := setupGroupTagResolverTestDB(t)
	withResolverSettingsReset(t)

	seedGroupTagResolverChannelWithPriority(t, db, 1, "ask-public", "shared-model", "GPT", 0)
	model.InitChannelCache()

	if !model.IsChannelEnabledForGroupModelTag("ask-public", "shared-model", "GPT", 1) {
		t.Fatalf("expected tagged channel to be enabled before cache update")
	}

	model.CacheUpdateChannelStatus(1, common.ChannelStatusAutoDisabled)

	if model.IsChannelEnabledForGroupModelTag("ask-public", "shared-model", "GPT", 1) {
		t.Fatalf("expected disabled tagged channel to be removed from tag cache")
	}
	channel, err := model.GetRandomSatisfiedChannel("ask-public", "shared-model", "GPT", 0)
	if err != nil {
		t.Fatalf("expected no error after removing tagged channel from cache, got %v", err)
	}
	if channel != nil {
		t.Fatalf("expected tagged selection to return nil after cache removal, got %#v", channel)
	}
}
