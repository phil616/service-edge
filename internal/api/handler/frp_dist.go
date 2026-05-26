package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// ListFRPDists returns the list of uploaded frp release tarballs.
func (h *Handler) ListFRPDists(c *gin.Context) {
	rows, err := h.Svc.ListFRPDists()
	if err != nil {
		respondErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": rows})
}

// UploadFRPDist accepts a multipart file upload of a frp release tarball.
// The filename must follow frp_{version}_{os}_{arch}.tar.gz convention.
func (h *Handler) UploadFRPDist(c *gin.Context) {
	fh, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "field 'file' required: " + err.Error()})
		return
	}

	f, err := fh.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "open upload: " + err.Error()})
		return
	}
	defer f.Close()

	if err := h.Svc.UploadFRPDist(fh.Filename, f); err != nil {
		respondErr(c, err)
		return
	}

	h.auditUser(c, "upload_frp_dist", "frp_dist", fh.Filename, "")
	rows, _ := h.Svc.ListFRPDists()
	c.JSON(http.StatusOK, gin.H{"items": rows})
}

// DeleteFRPDist removes an uploaded frp release tarball by ID.
func (h *Handler) DeleteFRPDist(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.Svc.DeleteFRPDist(uint(id)); err != nil {
		respondErr(c, err)
		return
	}
	h.auditUser(c, "delete_frp_dist", "frp_dist", c.Param("id"), "")
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
