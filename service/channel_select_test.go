package service

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
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
	if !IsChannelEnabledForResolution("ask-public", "shared-model", nonStrict, 1) {
		t.Fatalf("expected non-strict resolution to accept general fallback channel")
	}

	strict := &GroupBillingResolution{RouteTag: "GPT", RouteTagStrict: true}
	if IsChannelEnabledForResolution("ask-public", "shared-model", strict, 1) {
		t.Fatalf("expected strict resolution to reject untagged general channel")
	}
}
