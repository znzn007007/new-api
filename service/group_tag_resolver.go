package service

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

type GroupBillingResolution struct {
	PublicGroup          string
	UserGroup            string
	ModelName            string
	MatchedTag           string
	RouteTag             string
	RouteTagStrict       bool
	BillingAttribution   string
	EffectiveRatio       float64
	GroupFallbackRatio   float64
	BillingRatioSource   string
	BillingRatioFallback bool
	UsedGroupSpecial     bool
	ResolvedByOverride   bool
}

func resolveGroupFallbackRatio(userGroup, publicGroup string) (float64, bool) {
	if ratio, ok := ratio_setting.GetGroupGroupRatio(userGroup, publicGroup); ok {
		return ratio, true
	}
	return ratio_setting.GetGroupRatio(publicGroup), false
}

func resolveConfiguredModelTag(publicGroup, modelName string) (string, bool) {
	if tag, ok := ratio_setting.GetPublicGroupModelTagOverride(publicGroup, modelName); ok && tag != "" {
		return tag, true
	}
	normalizedModel := ratio_setting.FormatMatchingModelName(modelName)
	if normalizedModel != "" && normalizedModel != modelName {
		if tag, ok := ratio_setting.GetPublicGroupModelTagOverride(publicGroup, normalizedModel); ok && tag != "" {
			return tag, true
		}
	}
	return "", false
}

func ResolveGroupBilling(publicGroup, userGroup, modelName string) (*GroupBillingResolution, error) {
	fallbackRatio, usedGroupSpecial := resolveGroupFallbackRatio(userGroup, publicGroup)
	resolution := &GroupBillingResolution{
		PublicGroup:          publicGroup,
		UserGroup:            userGroup,
		ModelName:            modelName,
		BillingAttribution:   publicGroup,
		EffectiveRatio:       fallbackRatio,
		GroupFallbackRatio:   fallbackRatio,
		BillingRatioSource:   "public_group_default",
		BillingRatioFallback: true,
		UsedGroupSpecial:     usedGroupSpecial,
	}
	if usedGroupSpecial {
		resolution.BillingRatioSource = "public_group_special"
	}

	if publicGroup == "" || modelName == "" {
		return resolution, nil
	}

	tags := model.GetEnabledTagsByGroupModel(publicGroup, modelName)
	overrideTag, hasOverride := resolveConfiguredModelTag(publicGroup, modelName)
	if hasOverride {
		if len(tags) == 0 || !common.StringsContains(tags, overrideTag) {
			return nil, fmt.Errorf("configured channel tag %q is unavailable for group %q model %q", overrideTag, publicGroup, modelName)
		}
		resolution.MatchedTag = overrideTag
		resolution.RouteTag = overrideTag
		resolution.RouteTagStrict = true
		resolution.BillingAttribution = overrideTag
		resolution.ResolvedByOverride = true
	} else {
		switch len(tags) {
		case 0:
			return resolution, nil
		case 1:
			resolution.MatchedTag = tags[0]
			resolution.RouteTag = tags[0]
			resolution.BillingAttribution = tags[0]
		}
	}

	if ratio, ok := ratio_setting.GetPublicGroupTagRatio(publicGroup, resolution.MatchedTag); ok {
		resolution.EffectiveRatio = ratio
		resolution.BillingRatioSource = "tag"
		resolution.BillingRatioFallback = false
	}

	return resolution, nil
}

func ApplyGroupBillingResolution(c *gin.Context, resolution *GroupBillingResolution) {
	if c == nil || resolution == nil {
		return
	}
	common.SetContextKey(c, constant.ContextKeyResolvedChannelTag, resolution.MatchedTag)
	common.SetContextKey(c, constant.ContextKeyBillingAttribution, resolution.BillingAttribution)
	common.SetContextKey(c, constant.ContextKeyBillingRatioSource, resolution.BillingRatioSource)
	common.SetContextKey(c, constant.ContextKeyBillingRatioFallback, resolution.BillingRatioFallback)
}

func ResolveAndApplyGroupBilling(c *gin.Context, publicGroup, modelName string) (*GroupBillingResolution, error) {
	userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	resolution, err := ResolveGroupBilling(publicGroup, userGroup, modelName)
	if err != nil {
		return nil, err
	}
	ApplyGroupBillingResolution(c, resolution)
	return resolution, nil
}
