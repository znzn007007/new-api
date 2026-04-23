package ratio_setting

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/types"
)

var defaultGroupRatio = map[string]float64{
	"default": 1,
	"vip":     1,
	"svip":    1,
}

var groupRatioMap = types.NewRWMap[string, float64]()

var defaultGroupGroupRatio = map[string]map[string]float64{
	"vip": {
		"edit_this": 0.9,
	},
}

var groupGroupRatioMap = types.NewRWMap[string, map[string]float64]()

var defaultGroupSpecialUsableGroup = map[string]map[string]string{
	"vip": {
		"append_1":   "vip_special_group_1",
		"-:remove_1": "vip_removed_group_1",
	},
}

var defaultPublicGroupTagRatio = map[string]map[string]float64{}

var publicGroupTagRatioMap = types.NewRWMap[string, map[string]float64]()

var defaultPublicGroupModelTagOverride = map[string]map[string]string{}

var publicGroupModelTagOverrideMap = types.NewRWMap[string, map[string]string]()

type GroupRatioSetting struct {
	GroupRatio              *types.RWMap[string, float64]            `json:"group_ratio"`
	GroupGroupRatio         *types.RWMap[string, map[string]float64] `json:"group_group_ratio"`
	GroupSpecialUsableGroup *types.RWMap[string, map[string]string]  `json:"group_special_usable_group"`
	PublicGroupTagRatio     *types.RWMap[string, map[string]float64] `json:"public_group_tag_ratio"`
	PublicGroupModelTag     *types.RWMap[string, map[string]string]  `json:"public_group_model_tag"`
}

var groupRatioSetting GroupRatioSetting

func init() {
	groupSpecialUsableGroup := types.NewRWMap[string, map[string]string]()
	groupSpecialUsableGroup.AddAll(defaultGroupSpecialUsableGroup)

	groupRatioMap.AddAll(defaultGroupRatio)
	groupGroupRatioMap.AddAll(defaultGroupGroupRatio)
	publicGroupTagRatioMap.AddAll(defaultPublicGroupTagRatio)
	publicGroupModelTagOverrideMap.AddAll(defaultPublicGroupModelTagOverride)

	groupRatioSetting = GroupRatioSetting{
		GroupSpecialUsableGroup: groupSpecialUsableGroup,
		GroupRatio:              groupRatioMap,
		GroupGroupRatio:         groupGroupRatioMap,
		PublicGroupTagRatio:     publicGroupTagRatioMap,
		PublicGroupModelTag:     publicGroupModelTagOverrideMap,
	}

	config.GlobalConfig.Register("group_ratio_setting", &groupRatioSetting)
}

func GetGroupRatioSetting() *GroupRatioSetting {
	if groupRatioSetting.GroupSpecialUsableGroup == nil {
		groupRatioSetting.GroupSpecialUsableGroup = types.NewRWMap[string, map[string]string]()
		groupRatioSetting.GroupSpecialUsableGroup.AddAll(defaultGroupSpecialUsableGroup)
	}
	if groupRatioSetting.PublicGroupTagRatio == nil {
		groupRatioSetting.PublicGroupTagRatio = types.NewRWMap[string, map[string]float64]()
		groupRatioSetting.PublicGroupTagRatio.AddAll(defaultPublicGroupTagRatio)
	}
	if groupRatioSetting.PublicGroupModelTag == nil {
		groupRatioSetting.PublicGroupModelTag = types.NewRWMap[string, map[string]string]()
		groupRatioSetting.PublicGroupModelTag.AddAll(defaultPublicGroupModelTagOverride)
	}
	return &groupRatioSetting
}

func GetGroupRatioCopy() map[string]float64 {
	return groupRatioMap.ReadAll()
}

func ContainsGroupRatio(name string) bool {
	_, ok := groupRatioMap.Get(name)
	return ok
}

func GroupRatio2JSONString() string {
	return groupRatioMap.MarshalJSONString()
}

func UpdateGroupRatioByJSONString(jsonStr string) error {
	return types.LoadFromJsonString(groupRatioMap, jsonStr)
}

func GetGroupRatio(name string) float64 {
	ratio, ok := groupRatioMap.Get(name)
	if !ok {
		common.SysLog("group ratio not found: " + name)
		return 1
	}
	return ratio
}

func GetGroupGroupRatio(userGroup, usingGroup string) (float64, bool) {
	gp, ok := groupGroupRatioMap.Get(userGroup)
	if !ok {
		return -1, false
	}
	ratio, ok := gp[usingGroup]
	if !ok {
		return -1, false
	}
	return ratio, true
}

func GroupGroupRatio2JSONString() string {
	return groupGroupRatioMap.MarshalJSONString()
}

func UpdateGroupGroupRatioByJSONString(jsonStr string) error {
	return types.LoadFromJsonString(groupGroupRatioMap, jsonStr)
}

func CheckGroupRatio(jsonStr string) error {
	checkGroupRatio := make(map[string]float64)
	err := common.Unmarshal([]byte(jsonStr), &checkGroupRatio)
	if err != nil {
		return err
	}
	for name, ratio := range checkGroupRatio {
		if ratio < 0 {
			return errors.New("group ratio must be not less than 0: " + name)
		}
	}
	return nil
}

func PublicGroupTagRatio2JSONString() string {
	return publicGroupTagRatioMap.MarshalJSONString()
}

func UpdatePublicGroupTagRatioByJSONString(jsonStr string) error {
	if strings.TrimSpace(jsonStr) == "" {
		jsonStr = "{}"
	}
	return types.LoadFromJsonString(publicGroupTagRatioMap, jsonStr)
}

func GetPublicGroupTagRatio(group, tag string) (float64, bool) {
	if tag == "" {
		return 0, false
	}
	groupRules, ok := publicGroupTagRatioMap.Get(group)
	if !ok {
		return 0, false
	}
	ratio, ok := groupRules[tag]
	return ratio, ok
}

func CheckPublicGroupTagRatio(jsonStr string) error {
	if strings.TrimSpace(jsonStr) == "" {
		return nil
	}
	checkRules := make(map[string]map[string]float64)
	if err := common.Unmarshal([]byte(jsonStr), &checkRules); err != nil {
		return err
	}
	for group, tagRules := range checkRules {
		for tag, ratio := range tagRules {
			if ratio < 0 {
				return errors.New("public-group tag ratio must be not less than 0: " + group + "/" + tag)
			}
		}
	}
	return nil
}

func PublicGroupModelTagOverride2JSONString() string {
	return publicGroupModelTagOverrideMap.MarshalJSONString()
}

func UpdatePublicGroupModelTagOverrideByJSONString(jsonStr string) error {
	if strings.TrimSpace(jsonStr) == "" {
		jsonStr = "{}"
	}
	return types.LoadFromJsonString(publicGroupModelTagOverrideMap, jsonStr)
}

func GetPublicGroupModelTagOverride(group, model string) (string, bool) {
	groupRules, ok := publicGroupModelTagOverrideMap.Get(group)
	if !ok {
		return "", false
	}
	tag, ok := groupRules[model]
	return tag, ok
}

func CheckPublicGroupModelTagOverride(jsonStr string) error {
	if strings.TrimSpace(jsonStr) == "" {
		return nil
	}
	checkRules := make(map[string]map[string]string)
	if err := common.Unmarshal([]byte(jsonStr), &checkRules); err != nil {
		return err
	}
	return nil
}
