package router

import (
	"github.com/gin-gonic/gin"

	"github.com/ld/storeinventory/internal/constants"
	"github.com/ld/storeinventory/internal/handler"
	"github.com/ld/storeinventory/internal/middleware"
)

// registerTransferRoutes 调拨单路由：创建启用限流，审批仅总部/管理员。
// 草稿是独立工作流，单独挂载到 /transfer-drafts（Gin 基数树不允许 /transfers 下同时存在
// 静态段 drafts 与参数段 :id，故不使用 /transfers/drafts 嵌套）。
func registerTransferRoutes(v1 *gin.RouterGroup, h *handler.TransferOrderHandler, auth gin.HandlerFunc, managerRoles []constants.UserRole, limiter gin.HandlerFunc) {
	transfers := v1.Group("/transfers", auth)
	transfers.GET("", h.List)
	transfers.POST("", middleware.RequireRole(managerRoles...), limiter, h.Create)
	transfers.PUT("/:id/confirm", middleware.RequireRole(constants.RoleAdmin, constants.RoleHQ), h.Confirm)
	transfers.PUT("/:id/ship", middleware.RequireRole(managerRoles...), h.Ship)
	transfers.PUT("/:id/receive", middleware.RequireRole(managerRoles...), h.Receive)
	transfers.PUT("/:id/cancel", middleware.RequireRole(managerRoles...), h.Cancel)

	// 草稿环节：仅店长角色可访问，且只能操作本人草稿（service 层按 creator_id 强制校验）。
	drafts := v1.Group("/transfer-drafts", auth, middleware.RequireRole(constants.RoleStoreManager))
	drafts.GET("", h.ListDrafts)
	drafts.POST("", limiter, h.SaveDraft)
	drafts.PUT("/:id", h.UpdateDraft)
	drafts.PUT("/:id/void", h.VoidDraft)
	drafts.PUT("/:id/submit", limiter, h.SubmitDraft)
}
