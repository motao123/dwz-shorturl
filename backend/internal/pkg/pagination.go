package pkg

import (
	"strconv"

	"github.com/gin-gonic/gin"
)

func ParsePagination(c *gin.Context) (page, perPage int) {
	page = 1
	perPage = 20

	if p := c.Query("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			page = v
		}
	}

	if pp := c.Query("per_page"); pp != "" {
		if v, err := strconv.Atoi(pp); err == nil && v > 0 {
			perPage = v
		}
	}

	if perPage > 100 {
		perPage = 100
	}

	return page, perPage
}
