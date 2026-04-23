package service

import (
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func initModelColumnsForRealtimeQuotaTest(t *testing.T) {
	t.Helper()

	originalSQLitePath := common.SQLitePath
	originalSQLDSN, hadSQLDSN := os.LookupEnv("SQL_DSN")

	common.SQLitePath = fmt.Sprintf("file:%s_init?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	if err := os.Setenv("SQL_DSN", "local"); err != nil {
		t.Fatalf("failed to set SQL_DSN: %v", err)
	}
	if err := model.InitDB(); err != nil {
		t.Fatalf("failed to initialize model column helpers: %v", err)
	}
	if model.DB != nil {
		sqlDB, err := model.DB.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	}

	common.SQLitePath = originalSQLitePath
	if hadSQLDSN {
		_ = os.Setenv("SQL_DSN", originalSQLDSN)
	} else {
		_ = os.Unsetenv("SQL_DSN")
	}
}

func setupRealtimeQuotaTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	previousDB := model.DB
	previousLogDB := model.LOG_DB

	initModelColumnsForRealtimeQuotaTest(t)

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
	}

	model.DB = db
	model.LOG_DB = db

	if err := db.AutoMigrate(&model.User{}, &model.Token{}, &model.Log{}); err != nil {
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

func TestPreWssConsumeQuotaKeepsZeroGroupRatioFree(t *testing.T) {
	db := setupRealtimeQuotaTestDB(t)

	user := &model.User{Id: 1, Username: "test_user", Quota: 100, Status: common.UserStatusEnabled}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("failed to seed user: %v", err)
	}
	token := &model.Token{
		Id:          1,
		UserId:      1,
		Key:         "rt-test",
		Name:        "test_token",
		Status:      common.TokenStatusEnabled,
		RemainQuota: 100,
	}
	if err := db.Create(token).Error; err != nil {
		t.Fatalf("failed to seed token: %v", err)
	}

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	relayInfo := &relaycommon.RelayInfo{
		UserId:          1,
		TokenId:         1,
		TokenKey:        "sk-rt-test",
		UsingGroup:      "vip",
		UserGroup:       "default",
		OriginModelName: "gpt-4o-mini-realtime-preview",
		PriceData: types.PriceData{
			GroupRatioInfo: types.GroupRatioInfo{
				GroupRatio: 0,
			},
		},
	}
	usage := &dto.RealtimeUsage{
		TotalTokens: 10,
		InputTokenDetails: dto.InputTokenDetails{
			TextTokens: 10,
		},
	}

	if err := PreWssConsumeQuota(ctx, relayInfo, usage); err != nil {
		t.Fatalf("expected zero group ratio to remain free, got error: %v", err)
	}

	userQuota, err := model.GetUserQuota(1, false)
	if err != nil {
		t.Fatalf("failed to query user quota: %v", err)
	}
	if userQuota != 100 {
		t.Fatalf("expected zero-ratio realtime pre-consume to keep user quota unchanged, got %d", userQuota)
	}

	gotToken, err := model.GetTokenByKey("rt-test", true)
	if err != nil {
		t.Fatalf("failed to query token: %v", err)
	}
	if gotToken.RemainQuota != 100 {
		t.Fatalf("expected zero-ratio realtime pre-consume to keep token quota unchanged, got %d", gotToken.RemainQuota)
	}
}
