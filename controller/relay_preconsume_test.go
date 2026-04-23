package controller

import (
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupRelayPreconsumeTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.MemoryCacheEnabled = true

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
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
	})

	return db
}

func seedRelayPreconsumeChannel(t *testing.T, db *gorm.DB, id int, group, modelName, tag string) {
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

func withRelayPreconsumeRatioReset(t *testing.T) {
	t.Helper()
	originalGroupRatio := ratio_setting.GroupRatio2JSONString()
	originalTagRatio := ratio_setting.PublicGroupTagRatio2JSONString()
	originalModelPrice := ratio_setting.ModelPrice2JSONString()
	t.Cleanup(func() {
		_ = ratio_setting.UpdateGroupRatioByJSONString(originalGroupRatio)
		_ = ratio_setting.UpdatePublicGroupTagRatioByJSONString(originalTagRatio)
		_ = ratio_setting.UpdateModelPriceByJSONString(originalModelPrice)
	})
}

func TestPrepareRelayFirstAttemptUsesResolvedTagRatioForPreConsume(t *testing.T) {
	db := setupRelayPreconsumeTestDB(t)
	withRelayPreconsumeRatioReset(t)
	seedRelayPreconsumeChannel(t, db, 1, "ask-public", "single-model", "Claude 第三方")
	model.InitChannelCache()

	if err := ratio_setting.UpdateGroupRatioByJSONString(`{"ask-public":1}`); err != nil {
		t.Fatalf("failed to seed group ratio: %v", err)
	}
	if err := ratio_setting.UpdatePublicGroupTagRatioByJSONString(`{"ask-public":{"Claude 第三方":1.8}}`); err != nil {
		t.Fatalf("failed to seed tag ratio: %v", err)
	}
	if err := ratio_setting.UpdateModelPriceByJSONString(`{"single-model":0.01}`); err != nil {
		t.Fatalf("failed to seed model price: %v", err)
	}

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	common.SetContextKey(c, constant.ContextKeyUserGroup, "default")

	retryParam := &service.RetryParam{
		Ctx:        c,
		TokenGroup: "ask-public",
		ModelName:  "single-model",
		Retry:      common.GetPointer(0),
	}
	info := &relaycommon.RelayInfo{
		TokenGroup:      "ask-public",
		UsingGroup:      "ask-public",
		UserGroup:       "default",
		OriginModelName: "single-model",
		ChannelMeta:     &relaycommon.ChannelMeta{},
	}

	channel, priceData, apiErr := prepareRelayFirstAttempt(c, info, retryParam, 0, &types.TokenCountMeta{})
	if apiErr != nil {
		t.Fatalf("expected prepareRelayFirstAttempt to succeed, got error: %v", apiErr)
	}
	if channel == nil {
		t.Fatalf("expected selected channel")
	}
	if got := priceData.GroupRatioInfo.MatchedTag; got != "Claude 第三方" {
		t.Fatalf("expected resolved tag Claude 第三方, got %q", got)
	}
	expectedQuota := int(0.01 * common.QuotaPerUnit * 1.8)
	if priceData.QuotaToPreConsume != expectedQuota {
		t.Fatalf("expected pre-consume quota %d with tag ratio, got %d", expectedQuota, priceData.QuotaToPreConsume)
	}
}
