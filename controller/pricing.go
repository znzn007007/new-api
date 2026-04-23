package controller

import (
	"sort"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

func getVisibleGroupRatio(userGroup, publicGroup string) (float64, string) {
	if ratio, ok := ratio_setting.GetGroupGroupRatio(userGroup, publicGroup); ok {
		return ratio, "public_group_special"
	}
	return ratio_setting.GetGroupRatio(publicGroup), "public_group_default"
}

func enrichEffectiveGroupPricing(pricing []model.Pricing, userGroup string) {
	for i := range pricing {
		if len(pricing[i].EnableGroup) == 0 {
			continue
		}
		effectivePricing := make(map[string]model.EffectiveGroupPricing, len(pricing[i].EnableGroup))
		for _, publicGroup := range pricing[i].EnableGroup {
			resolution, err := service.ResolveGroupBilling(publicGroup, userGroup, pricing[i].ModelName)
			if err != nil {
				ratio, source := getVisibleGroupRatio(userGroup, publicGroup)
				effectivePricing[publicGroup] = model.EffectiveGroupPricing{
					Group:                publicGroup,
					Ratio:                ratio,
					BillingAttribution:   publicGroup,
					BillingRatioSource:   source,
					BillingRatioFallback: true,
				}
				continue
			}
			effectivePricing[publicGroup] = model.EffectiveGroupPricing{
				Group:                publicGroup,
				Ratio:                resolution.EffectiveRatio,
				MatchedTag:           resolution.MatchedTag,
				BillingAttribution:   resolution.BillingAttribution,
				BillingRatioSource:   resolution.BillingRatioSource,
				BillingRatioFallback: resolution.BillingRatioFallback,
			}
		}
		pricing[i].EffectiveGroupPricing = effectivePricing
	}
}

func filterPricingByUsableGroups(pricing []model.Pricing, usableGroup map[string]string) []model.Pricing {
	if len(pricing) == 0 {
		return pricing
	}
	if len(usableGroup) == 0 {
		return []model.Pricing{}
	}

	filtered := make([]model.Pricing, 0, len(pricing))
	for _, item := range pricing {
		visibleGroups := make([]string, 0)
		if common.StringsContains(item.EnableGroup, "all") {
			for group := range usableGroup {
				if group == "auto" || group == "" {
					continue
				}
				visibleGroups = append(visibleGroups, group)
			}
		} else {
			for _, group := range item.EnableGroup {
				if _, ok := usableGroup[group]; ok {
					visibleGroups = append(visibleGroups, group)
				}
			}
		}
		if len(visibleGroups) > 0 {
			sort.Strings(visibleGroups)
			item.EnableGroup = visibleGroups
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func GetPricing(c *gin.Context) {
	pricing := model.GetPricing()
	userId, exists := c.Get("id")
	usableGroup := map[string]string{}
	groupRatio := map[string]float64{}
	for s, f := range ratio_setting.GetGroupRatioCopy() {
		groupRatio[s] = f
	}
	var group string
	if exists {
		user, err := model.GetUserCache(userId.(int))
		if err == nil {
			group = user.Group
			for g := range groupRatio {
				ratio, ok := ratio_setting.GetGroupGroupRatio(group, g)
				if ok {
					groupRatio[g] = ratio
				}
			}
		}
	}

	usableGroup = service.GetUserUsableGroups(group)
	pricing = filterPricingByUsableGroups(pricing, usableGroup)
	enrichEffectiveGroupPricing(pricing, group)
	// check groupRatio contains usableGroup
	for group := range ratio_setting.GetGroupRatioCopy() {
		if _, ok := usableGroup[group]; !ok {
			delete(groupRatio, group)
		}
	}

	c.JSON(200, gin.H{
		"success":            true,
		"data":               pricing,
		"vendors":            model.GetVendors(),
		"group_ratio":        groupRatio,
		"usable_group":       usableGroup,
		"supported_endpoint": model.GetSupportedEndpointMap(),
		"auto_groups":        service.GetUserAutoGroup(group),
		"pricing_version":    "a42d372ccf0b5dd13ecf71203521f9d2",
	})
}

func ResetModelRatio(c *gin.Context) {
	defaultStr := ratio_setting.DefaultModelRatio2JSONString()
	err := model.UpdateOption("ModelRatio", defaultStr)
	if err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	err = ratio_setting.UpdateModelRatioByJSONString(defaultStr)
	if err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(200, gin.H{
		"success": true,
		"message": "重置模型倍率成功",
	})
}
