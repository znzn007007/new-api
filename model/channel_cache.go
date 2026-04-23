package model

import (
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

var group2model2channels map[string]map[string][]int // enabled channel
var group2model2tag2channels map[string]map[string]map[string][]int
var channelsIDM map[int]*Channel // all channels include disabled
var channelSyncLock sync.RWMutex

func InitChannelCache() {
	if !common.MemoryCacheEnabled {
		return
	}
	newChannelId2channel := make(map[int]*Channel)
	var channels []*Channel
	DB.Find(&channels)
	for _, channel := range channels {
		newChannelId2channel[channel.Id] = channel
	}
	var abilities []*Ability
	DB.Find(&abilities)
	groups := make(map[string]bool)
	for _, ability := range abilities {
		groups[ability.Group] = true
	}
	newGroup2model2channels := make(map[string]map[string][]int)
	newGroup2model2tag2channels := make(map[string]map[string]map[string][]int)
	for group := range groups {
		newGroup2model2channels[group] = make(map[string][]int)
		newGroup2model2tag2channels[group] = make(map[string]map[string][]int)
	}
	for _, channel := range channels {
		if channel.Status != common.ChannelStatusEnabled {
			continue // skip disabled channels
		}
		groups := strings.Split(channel.Group, ",")
		for _, group := range groups {
			models := strings.Split(channel.Models, ",")
			for _, model := range models {
				if _, ok := newGroup2model2channels[group][model]; !ok {
					newGroup2model2channels[group][model] = make([]int, 0)
				}
				newGroup2model2channels[group][model] = append(newGroup2model2channels[group][model], channel.Id)
				if channel.Tag != nil && *channel.Tag != "" {
					if _, ok := newGroup2model2tag2channels[group][model]; !ok {
						newGroup2model2tag2channels[group][model] = make(map[string][]int)
					}
					newGroup2model2tag2channels[group][model][*channel.Tag] = append(newGroup2model2tag2channels[group][model][*channel.Tag], channel.Id)
				}
			}
		}
	}

	// sort by priority
	for group, model2channels := range newGroup2model2channels {
		for model, channels := range model2channels {
			sort.Slice(channels, func(i, j int) bool {
				return newChannelId2channel[channels[i]].GetPriority() > newChannelId2channel[channels[j]].GetPriority()
			})
			newGroup2model2channels[group][model] = channels
		}
	}
	for group, model2tag2channels := range newGroup2model2tag2channels {
		for model, tag2channels := range model2tag2channels {
			for tag, channels := range tag2channels {
				sort.Slice(channels, func(i, j int) bool {
					return newChannelId2channel[channels[i]].GetPriority() > newChannelId2channel[channels[j]].GetPriority()
				})
				newGroup2model2tag2channels[group][model][tag] = channels
			}
		}
	}

	channelSyncLock.Lock()
	group2model2channels = newGroup2model2channels
	group2model2tag2channels = newGroup2model2tag2channels
	//channelsIDM = newChannelId2channel
	for i, channel := range newChannelId2channel {
		if channel.ChannelInfo.IsMultiKey {
			channel.Keys = channel.GetKeys()
			if channel.ChannelInfo.MultiKeyMode == constant.MultiKeyModePolling {
				if oldChannel, ok := channelsIDM[i]; ok {
					// 存在旧的渠道，如果是多key且轮询，保留轮询索引信息
					if oldChannel.ChannelInfo.IsMultiKey && oldChannel.ChannelInfo.MultiKeyMode == constant.MultiKeyModePolling {
						channel.ChannelInfo.MultiKeyPollingIndex = oldChannel.ChannelInfo.MultiKeyPollingIndex
					}
				}
			}
		}
	}
	channelsIDM = newChannelId2channel
	channelSyncLock.Unlock()
	common.SysLog("channels synced from database")
}

func SyncChannelCache(frequency int) {
	for {
		time.Sleep(time.Duration(frequency) * time.Second)
		common.SysLog("syncing channels from database")
		InitChannelCache()
	}
}

func GetRandomSatisfiedChannel(group string, model string, tag string, retry int) (*Channel, error) {
	// if memory cache is disabled, get channel directly from database
	if !common.MemoryCacheEnabled {
		return GetChannel(group, model, tag, retry)
	}

	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	channels := getChannelsByGroupModelTagUnlocked(group, model, tag)

	if len(channels) == 0 {
		return nil, nil
	}

	if len(channels) == 1 {
		if channel, ok := channelsIDM[channels[0]]; ok {
			return channel, nil
		}
		return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channels[0])
	}

	uniquePriorities := make(map[int]bool)
	for _, channelId := range channels {
		if channel, ok := channelsIDM[channelId]; ok {
			uniquePriorities[int(channel.GetPriority())] = true
		} else {
			return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channelId)
		}
	}
	var sortedUniquePriorities []int
	for priority := range uniquePriorities {
		sortedUniquePriorities = append(sortedUniquePriorities, priority)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(sortedUniquePriorities)))

	if retry >= len(uniquePriorities) {
		retry = len(uniquePriorities) - 1
	}
	targetPriority := int64(sortedUniquePriorities[retry])

	// get the priority for the given retry number
	var sumWeight = 0
	var targetChannels []*Channel
	for _, channelId := range channels {
		if channel, ok := channelsIDM[channelId]; ok {
			if channel.GetPriority() == targetPriority {
				sumWeight += channel.GetWeight()
				targetChannels = append(targetChannels, channel)
			}
		} else {
			return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channelId)
		}
	}

	if len(targetChannels) == 0 {
		return nil, errors.New(fmt.Sprintf("no channel found, group: %s, model: %s, priority: %d", group, model, targetPriority))
	}

	// smoothing factor and adjustment
	smoothingFactor := 1
	smoothingAdjustment := 0

	if sumWeight == 0 {
		// when all channels have weight 0, set sumWeight to the number of channels and set smoothing adjustment to 100
		// each channel's effective weight = 100
		sumWeight = len(targetChannels) * 100
		smoothingAdjustment = 100
	} else if sumWeight/len(targetChannels) < 10 {
		// when the average weight is less than 10, set smoothing factor to 100
		smoothingFactor = 100
	}

	// Calculate the total weight of all channels up to endIdx
	totalWeight := sumWeight * smoothingFactor

	// Generate a random value in the range [0, totalWeight)
	randomWeight := rand.Intn(totalWeight)

	// Find a channel based on its weight
	for _, channel := range targetChannels {
		randomWeight -= channel.GetWeight()*smoothingFactor + smoothingAdjustment
		if randomWeight < 0 {
			return channel, nil
		}
	}
	// return null if no channel is not found
	return nil, errors.New("channel not found")
}

func GetEnabledTagsByGroupModel(group string, model string) []string {
	if !common.MemoryCacheEnabled {
		return getEnabledTagsByGroupModelDB(group, model)
	}

	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	tag2channels := getTagChannelsByGroupModelUnlocked(group, model)
	if len(tag2channels) == 0 {
		return []string{}
	}
	tags := make([]string, 0, len(tag2channels))
	for tag := range tag2channels {
		if tag == "" {
			continue
		}
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	return tags
}

func getTagChannelsByGroupModelUnlocked(group string, model string) map[string][]int {
	if group2model2tag2channels == nil {
		return nil
	}
	tagChannels := group2model2tag2channels[group][model]
	if len(tagChannels) > 0 {
		return tagChannels
	}
	normalizedModel := ratio_setting.FormatMatchingModelName(model)
	if normalizedModel == "" || normalizedModel == model {
		return nil
	}
	return group2model2tag2channels[group][normalizedModel]
}

func getChannelsByGroupModelTagUnlocked(group string, model string, tag string) []int {
	if tag != "" {
		tagChannels := getTagChannelsByGroupModelUnlocked(group, model)
		if len(tagChannels) > 0 {
			return tagChannels[tag]
		}
	}

	channels := group2model2channels[group][model]
	if len(channels) > 0 {
		return channels
	}

	normalizedModel := ratio_setting.FormatMatchingModelName(model)
	if normalizedModel == "" || normalizedModel == model {
		return nil
	}
	return group2model2channels[group][normalizedModel]
}

func CacheGetChannel(id int) (*Channel, error) {
	if !common.MemoryCacheEnabled {
		return GetChannelById(id, true)
	}
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	c, ok := channelsIDM[id]
	if !ok {
		return nil, fmt.Errorf("渠道# %d，已不存在", id)
	}
	return c, nil
}

func CacheGetChannelInfo(id int) (*ChannelInfo, error) {
	if !common.MemoryCacheEnabled {
		channel, err := GetChannelById(id, true)
		if err != nil {
			return nil, err
		}
		return &channel.ChannelInfo, nil
	}
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	c, ok := channelsIDM[id]
	if !ok {
		return nil, fmt.Errorf("渠道# %d，已不存在", id)
	}
	return &c.ChannelInfo, nil
}

func CacheUpdateChannelStatus(id int, status int) {
	if !common.MemoryCacheEnabled {
		return
	}
	channelSyncLock.Lock()
	defer channelSyncLock.Unlock()
	if channel, ok := channelsIDM[id]; ok {
		channel.Status = status
	}
	if status != common.ChannelStatusEnabled {
		// delete the channel from group2model2channels
		for group, model2channels := range group2model2channels {
			for model, channels := range model2channels {
				group2model2channels[group][model] = removeChannelIDFromSlice(channels, id)
			}
		}
		for group, model2tag2channels := range group2model2tag2channels {
			for model, tag2channels := range model2tag2channels {
				for tag, channels := range tag2channels {
					group2model2tag2channels[group][model][tag] = removeChannelIDFromSlice(channels, id)
				}
			}
		}
	}
}

func removeChannelIDFromSlice(channels []int, channelID int) []int {
	if len(channels) == 0 {
		return channels
	}
	filtered := channels[:0]
	for _, id := range channels {
		if id != channelID {
			filtered = append(filtered, id)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

func CacheUpdateChannel(channel *Channel) {
	if !common.MemoryCacheEnabled {
		return
	}
	channelSyncLock.Lock()
	defer channelSyncLock.Unlock()
	if channel == nil {
		return
	}

	println("CacheUpdateChannel:", channel.Id, channel.Name, channel.Status, channel.ChannelInfo.MultiKeyPollingIndex)

	println("before:", channelsIDM[channel.Id].ChannelInfo.MultiKeyPollingIndex)
	channelsIDM[channel.Id] = channel
	println("after :", channelsIDM[channel.Id].ChannelInfo.MultiKeyPollingIndex)
}
