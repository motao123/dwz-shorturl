package handler

import (
	"net/http"
	"strconv"

	"dwz-admin/internal/pkg"
	"dwz-admin/internal/service"

	"github.com/gin-gonic/gin"
)

type DomainHandler struct {
	svc service.DomainService
}

func NewDomainHandler(svc service.DomainService) *DomainHandler {
	return &DomainHandler{svc: svc}
}

type CreateDomainRequest struct {
	Domain   string `json:"domain" binding:"required"`
	Scheme   string `json:"scheme"`
	Name     string `json:"name"`
	Project  string `json:"project"`
	Priority int    `json:"priority"`
}

type UpdateDomainRequest struct {
	Domain   string `json:"domain"`
	Scheme   string `json:"scheme"`
	Name     string `json:"name"`
	Project  string `json:"project"`
	Status   *int8  `json:"status"`
	Priority *int   `json:"priority"`
}

type BatchStatusRequest struct {
	IDs    []uint64 `json:"ids" binding:"required,min=1"`
	Status int8     `json:"status"`
}

// ActiveDomain is the public projection returned by the /active endpoint.
type ActiveDomain struct {
	ID       uint64 `json:"id"`
	Domain   string `json:"domain"`
	Scheme   string `json:"scheme"`
	Name     string `json:"name"`
	Priority int    `json:"priority"`
}

func (h *DomainHandler) List(c *gin.Context) {
	var status *int8
	if s := c.Query("status"); s != "" {
		if v, err := strconv.ParseInt(s, 10, 8); err == nil {
			st := int8(v)
			status = &st
		}
	}

	domains, err := h.svc.List(status)
	if err != nil {
		pkg.Fail(c, http.StatusInternalServerError, pkg.CodeInternalError, "query failed")
		return
	}

	pkg.Success(c, domains)
}

func (h *DomainHandler) GetByID(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, "invalid id")
		return
	}

	domain, err := h.svc.GetByID(id)
	if err != nil {
		pkg.Fail(c, http.StatusNotFound, pkg.CodeNotFound, "domain not found")
		return
	}

	pkg.Success(c, domain)
}

func (h *DomainHandler) Create(c *gin.Context) {
	var req CreateDomainRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeValidation, "domain is required")
		return
	}

	domain, err := h.svc.Create(req.Domain, req.Scheme, req.Name, req.Project, req.Priority)
	if err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, err.Error())
		return
	}

	pkg.Success(c, domain)
}

func (h *DomainHandler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, "invalid id")
		return
	}

	var req UpdateDomainRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeValidation, "invalid request body")
		return
	}

	domain, err := h.svc.Update(id, req.Domain, req.Scheme, req.Name, req.Project, req.Status, req.Priority)
	if err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, err.Error())
		return
	}

	pkg.Success(c, domain)
}

func (h *DomainHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, "invalid id")
		return
	}

	if err := h.svc.Delete(id); err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, err.Error())
		return
	}

	pkg.Success(c, nil)
}

func (h *DomainHandler) Check(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, "invalid id")
		return
	}

	// Re-fetch after the (synchronous) check to return fresh statuses.
	if err := h.svc.CheckDomain(id); err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, err.Error())
		return
	}

	domain, err := h.svc.GetByID(id)
	if err != nil {
		pkg.Success(c, gin.H{"id": id})
		return
	}

	pkg.Success(c, domain)
}

func (h *DomainHandler) BatchStatus(c *gin.Context) {
	var req BatchStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeValidation, "ids array is required")
		return
	}

	var failed []uint64
	for _, id := range req.IDs {
		_, err := h.svc.Update(id, "", "", "", "", &req.Status, nil)
		if err != nil {
			failed = append(failed, id)
		}
	}

	pkg.Success(c, gin.H{
		"updated": len(req.IDs) - len(failed),
		"failed":  failed,
	})
}

// Active is a public endpoint returning the list of active domains with a
// minimal projection (id, domain, scheme, name, priority).
func (h *DomainHandler) Active(c *gin.Context) {
	active := int8(1)
	domains, err := h.svc.List(&active)
	if err != nil {
		pkg.Fail(c, http.StatusInternalServerError, pkg.CodeInternalError, "query failed")
		return
	}

	list := make([]ActiveDomain, 0, len(domains))
	for _, d := range domains {
		scheme := d.Scheme
		if scheme == "" {
			scheme = "https"
		}
		list = append(list, ActiveDomain{
			ID:       d.ID,
			Domain:   d.Domain,
			Scheme:   scheme,
			Name:     d.Name,
			Priority: d.Priority,
		})
	}

	pkg.Success(c, list)
}
