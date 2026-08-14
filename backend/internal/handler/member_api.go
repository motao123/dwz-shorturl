package handler

import (
	"errors"
	"net/http"
	"strconv"

	"dwz-admin/internal/pkg"
	"dwz-admin/internal/service"

	"github.com/gin-gonic/gin"
)

type MemberApiHandler struct {
	svc service.MemberApiService
}

func NewMemberApiHandler(svc service.MemberApiService) *MemberApiHandler {
	return &MemberApiHandler{svc: svc}
}

type MemberCreateLinkRequest struct {
	URL        string  `json:"url" binding:"required"`
	Title      string  `json:"title"`
	Custom     string  `json:"custom"`
	ExpireDays int     `json:"expire_days"`
	Password   string  `json:"password"`
}

func (h *MemberApiHandler) Me(c *gin.Context) {
	memberID := c.GetUint64("member_id")
	member, err := h.svc.Me(memberID)
	if err != nil {
		pkg.Fail(c, http.StatusNotFound, pkg.CodeNotFound, "member not found")
		return
	}
	pkg.Success(c, member)
}

func (h *MemberApiHandler) ExportLinks(c *gin.Context) {
	memberID := c.GetUint64("member_id")
	data, err := h.svc.ExportLinks(memberID)
	if err != nil {
		pkg.Fail(c, http.StatusInternalServerError, pkg.CodeInternalError, "export failed")
		return
	}
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="my_links.csv"`)
	c.Data(http.StatusOK, "text/csv; charset=utf-8", data)
}

func (h *MemberApiHandler) Summary(c *gin.Context) {
	memberID := c.GetUint64("member_id")
	summary, err := h.svc.Summary(memberID)
	if err != nil {
		pkg.Fail(c, http.StatusInternalServerError, pkg.CodeInternalError, "query failed")
		return
	}
	pkg.Success(c, summary)
}

func (h *MemberApiHandler) ListLinks(c *gin.Context) {
	memberID := c.GetUint64("member_id")
	page, perPage := pkg.ParsePagination(c)
	list, total, err := h.svc.ListLinks(memberID, page, perPage, c.Query("keyword"), c.Query("status"))
	if err != nil {
		pkg.Fail(c, http.StatusInternalServerError, pkg.CodeInternalError, "query failed")
		return
	}
	pkg.Paginated(c, list, total, page, perPage)
}

func (h *MemberApiHandler) CreateLink(c *gin.Context) {
	memberID := c.GetUint64("member_id")
	var req MemberCreateLinkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeValidation, "url is required")
		return
	}
	record, err := h.svc.CreateLink(memberID, req.URL, req.Title, req.Custom, req.ExpireDays, c.ClientIP(), req.Password)
	if err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, err.Error())
		return
	}
	pkg.Success(c, gin.H{
		"id":        record.ID,
		"uid":       record.UID,
		"short_url": service.PublicShortURL(record.UID),
		"long_url":  record.LongURL,
		"title":     record.Title,
	})
}

func (h *MemberApiHandler) GetLinkStats(c *gin.Context) {
	memberID := c.GetUint64("member_id")
	uid := c.Param("uid")
	if uid == "" {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, "invalid uid")
		return
	}
	stat, err := h.svc.GetLinkStats(memberID, uid)
	if err != nil {
		if errors.Is(err, service.ErrMemberLinkNotFound) {
			pkg.Fail(c, http.StatusNotFound, pkg.CodeNotFound, "link not found")
			return
		}
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, err.Error())
		return
	}
	pkg.Success(c, stat)
}

type ForgotPasswordRequest struct {
	Email string `json:"email" binding:"required,email"`
}

type SendVerificationRequest struct {
	Email string `json:"email" binding:"required,email"`
}

type VerifyEmailRequest struct {
	Token string `json:"token" binding:"required"`
}

func (h *MemberApiHandler) SendVerification(c *gin.Context) {
	var req SendVerificationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeValidation, "email is required")
		return
	}
	if err := h.svc.SendVerification(req.Email); err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, err.Error())
		return
	}
	pkg.Success(c, gin.H{"sent": true})
}

func (h *MemberApiHandler) VerifyEmail(c *gin.Context) {
	var req VerifyEmailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeValidation, "token is required")
		return
	}
	if err := h.svc.VerifyEmail(req.Token); err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, err.Error())
		return
	}
	pkg.Success(c, nil)
}

type MemberResetPasswordRequest struct {
	Token    string `json:"token" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// RequestPasswordReset sends a reset email (public, no auth).
func (h *MemberApiHandler) RequestPasswordReset(c *gin.Context) {
	var req ForgotPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeValidation, "email is required")
		return
	}
	if err := h.svc.RequestPasswordReset(req.Email); err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, err.Error())
		return
	}
	// 统一成功，避免枚举
	pkg.Success(c, gin.H{"sent": true})
}

func (h *MemberApiHandler) ResetPassword(c *gin.Context) {
	var req MemberResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeValidation, "token and password are required")
		return
	}
	if err := h.svc.ResetPassword(req.Token, req.Password); err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, err.Error())
		return
	}
	pkg.Success(c, nil)
}

type FetchTitleRequest struct {
	URL string `json:"url" binding:"required"`
}

func (h *MemberApiHandler) FetchTitle(c *gin.Context) {
	var req FetchTitleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeValidation, "url is required")
		return
	}
	title, err := h.svc.FetchTitle(req.URL)
	if err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, err.Error())
		return
	}
	pkg.Success(c, gin.H{"title": title})
}

type MemberImportRequest struct {
	Content string `json:"content" binding:"required"`
}

func (h *MemberApiHandler) ImportLinks(c *gin.Context) {
	memberID := c.GetUint64("member_id")
	var req MemberImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeValidation, "content is required")
		return
	}
	results, err := h.svc.ImportLinks(memberID, req.Content, c.ClientIP())
	if err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, err.Error())
		return
	}
	pkg.Success(c, results)
}

type MemberBatchCreateRequest struct {
	URLs []string `json:"urls" binding:"required"`
}

func (h *MemberApiHandler) BatchCreateLinks(c *gin.Context) {
	memberID := c.GetUint64("member_id")
	var req MemberBatchCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeValidation, "urls is required")
		return
	}
	if len(req.URLs) > 100 {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, "too many urls; maximum is 100")
		return
	}
	results, err := h.svc.BatchCreateLinks(memberID, req.URLs, c.ClientIP())
	if err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, err.Error())
		return
	}
	pkg.Success(c, results)
}

type RenewExpiringRequest struct {
	ExpireDays int `json:"expire_days"`
}

func (h *MemberApiHandler) RenewExpiring(c *gin.Context) {
	memberID := c.GetUint64("member_id")
	var req RenewExpiringRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.ExpireDays <= 0 {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeValidation, "expire_days is required")
		return
	}
	renewed, err := h.svc.RenewExpiring(memberID, req.ExpireDays)
	if err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, err.Error())
		return
	}
	pkg.Success(c, gin.H{"renewed": renewed})
}

type MemberUpdateLinkRequest struct {
	LongURL    string `json:"long_url"`
	Title      string `json:"title"`
	ExpireDays *int   `json:"expire_days"`
}

func (h *MemberApiHandler) UpdateLink(c *gin.Context) {
	memberID := c.GetUint64("member_id")
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, "invalid id")
		return
	}
	var req MemberUpdateLinkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeValidation, "invalid request")
		return
	}
	record, err := h.svc.UpdateLink(memberID, id, req.LongURL, req.Title, req.ExpireDays)
	if err != nil {
		if errors.Is(err, service.ErrMemberLinkNotFound) {
			pkg.Fail(c, http.StatusNotFound, pkg.CodeNotFound, "link not found")
			return
		}
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, err.Error())
		return
	}
	pkg.Success(c, record)
}

type UpdateLinkExpiryRequest struct {
	ExpireDays int `json:"expire_days"`
}

func (h *MemberApiHandler) UpdateLinkExpiry(c *gin.Context) {
	memberID := c.GetUint64("member_id")
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, "invalid id")
		return
	}
	var req UpdateLinkExpiryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeValidation, "invalid request")
		return
	}
	record, err := h.svc.UpdateLinkExpiry(memberID, id, req.ExpireDays)
	if err != nil {
		if errors.Is(err, service.ErrMemberLinkNotFound) {
			pkg.Fail(c, http.StatusNotFound, pkg.CodeNotFound, "link not found")
			return
		}
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, err.Error())
		return
	}
	pkg.Success(c, gin.H{
		"id":        record.ID,
		"uid":       record.UID,
		"expire_at": record.ExpireAt,
	})
}

func (h *MemberApiHandler) DeleteLink(c *gin.Context) {
	memberID := c.GetUint64("member_id")
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, "invalid id")
		return
	}
	if err := h.svc.DeleteLink(memberID, id); err != nil {
		if errors.Is(err, service.ErrMemberLinkNotFound) {
			pkg.Fail(c, http.StatusNotFound, pkg.CodeNotFound, "link not found")
			return
		}
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, err.Error())
		return
	}
	pkg.Success(c, nil)
}