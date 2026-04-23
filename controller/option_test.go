package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupOptionControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	gin.SetMode(gin.TestMode)
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
	}

	model.DB = db
	model.LOG_DB = db
	model.InitOptionMap()

	if err := db.AutoMigrate(&model.Option{}); err != nil {
		t.Fatalf("failed to migrate option table: %v", err)
	}

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	return db
}

func newOptionUpdateContext(t *testing.T, payload string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/option/", bytes.NewBufferString(payload))
	ctx.Request.Header.Set("Content-Type", "application/json")
	return ctx, recorder
}

func TestUpdateOptionAcceptsPublicGroupTagRatio(t *testing.T) {
	setupOptionControllerTestDB(t)

	original := ratio_setting.PublicGroupTagRatio2JSONString()
	t.Cleanup(func() {
		_ = ratio_setting.UpdatePublicGroupTagRatioByJSONString(original)
	})

	ctx, recorder := newOptionUpdateContext(t, `{"key":"group_ratio_setting.public_group_tag_ratio","value":"{\"ask-public\":{\"GPT\":1.6}}"}`)
	UpdateOption(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected http 200, got %d", recorder.Code)
	}
	ratio, ok := ratio_setting.GetPublicGroupTagRatio("ask-public", "GPT")
	if !ok {
		t.Fatalf("expected persisted GPT ratio")
	}
	if ratio != 1.6 {
		t.Fatalf("expected ratio 1.6, got %v", ratio)
	}
}

func TestUpdateOptionRejectsNegativePublicGroupTagRatio(t *testing.T) {
	setupOptionControllerTestDB(t)

	ctx, recorder := newOptionUpdateContext(t, `{"key":"group_ratio_setting.public_group_tag_ratio","value":"{\"ask-public\":{\"GPT\":-1}}"}`)
	UpdateOption(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected http 200, got %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), `"success":false`) {
		t.Fatalf("expected validation failure, got %s", recorder.Body.String())
	}
}
