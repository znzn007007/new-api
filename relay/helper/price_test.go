package helper

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

func withGroupRatioReset(t *testing.T) {
	t.Helper()
	originalGroupRatio := ratio_setting.GroupRatio2JSONString()
	originalTagRatio := ratio_setting.PublicGroupTagRatio2JSONString()
	t.Cleanup(func() {
		_ = ratio_setting.UpdateGroupRatioByJSONString(originalGroupRatio)
		_ = ratio_setting.UpdatePublicGroupTagRatioByJSONString(originalTagRatio)
	})
}

func newPriceTestContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)
	return ctx
}

func TestHandleGroupRatioKeepsAmbiguousBillingOnPublicGroupFallback(t *testing.T) {
	withGroupRatioReset(t)

	if err := ratio_setting.UpdateGroupRatioByJSONString(`{"ask-public":1.0}`); err != nil {
		t.Fatalf("failed to seed group ratio: %v", err)
	}
	if err := ratio_setting.UpdatePublicGroupTagRatioByJSONString(`{"ask-public":{"cc":1.8}}`); err != nil {
		t.Fatalf("failed to seed tag ratio: %v", err)
	}

	ctx := newPriceTestContext()
	common.SetContextKey(ctx, constant.ContextKeyResolvedChannelTag, "")
	common.SetContextKey(ctx, constant.ContextKeyChannelTag, "cc")
	common.SetContextKey(ctx, constant.ContextKeyBillingAttribution, "ask-public")
	common.SetContextKey(ctx, constant.ContextKeyBillingRatioSource, "public_group_default")
	common.SetContextKey(ctx, constant.ContextKeyBillingRatioFallback, true)

	info := &relaycommon.RelayInfo{UsingGroup: "ask-public", UserGroup: "default"}
	ratioInfo := HandleGroupRatio(ctx, info)

	if ratioInfo.MatchedTag != "" {
		t.Fatalf("expected ambiguous billing to keep empty resolved tag, got %q", ratioInfo.MatchedTag)
	}
	if ratioInfo.GroupRatio != 1.0 {
		t.Fatalf("expected public-group fallback ratio 1.0, got %v", ratioInfo.GroupRatio)
	}
	if ratioInfo.BillingRatioSource != "public_group_default" {
		t.Fatalf("expected public-group default billing source, got %q", ratioInfo.BillingRatioSource)
	}
	if !ratioInfo.BillingRatioFallback {
		t.Fatalf("expected billing ratio fallback to remain true")
	}
}

func TestHandleGroupRatioStillUsesResolvedTagRatioWhenProvided(t *testing.T) {
	withGroupRatioReset(t)

	if err := ratio_setting.UpdateGroupRatioByJSONString(`{"ask-public":1.0}`); err != nil {
		t.Fatalf("failed to seed group ratio: %v", err)
	}
	if err := ratio_setting.UpdatePublicGroupTagRatioByJSONString(`{"ask-public":{"cc":1.8}}`); err != nil {
		t.Fatalf("failed to seed tag ratio: %v", err)
	}

	ctx := newPriceTestContext()
	common.SetContextKey(ctx, constant.ContextKeyResolvedChannelTag, "cc")
	common.SetContextKey(ctx, constant.ContextKeyChannelTag, "other")
	common.SetContextKey(ctx, constant.ContextKeyBillingAttribution, "cc")
	common.SetContextKey(ctx, constant.ContextKeyBillingRatioSource, "tag")
	common.SetContextKey(ctx, constant.ContextKeyBillingRatioFallback, false)

	info := &relaycommon.RelayInfo{UsingGroup: "ask-public", UserGroup: "default"}
	ratioInfo := HandleGroupRatio(ctx, info)

	if ratioInfo.MatchedTag != "cc" {
		t.Fatalf("expected resolved tag cc, got %q", ratioInfo.MatchedTag)
	}
	if ratioInfo.GroupRatio != 1.8 {
		t.Fatalf("expected resolved tag ratio 1.8, got %v", ratioInfo.GroupRatio)
	}
	if ratioInfo.BillingAttribution != "cc" {
		t.Fatalf("expected billing attribution cc, got %q", ratioInfo.BillingAttribution)
	}
	if ratioInfo.BillingRatioSource != "tag" {
		t.Fatalf("expected tag billing source, got %q", ratioInfo.BillingRatioSource)
	}
}
