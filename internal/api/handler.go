package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	repo *Repository
}

func NewHandler(repo *Repository) *Handler {
	return &Handler{repo: repo}
}

func (h *Handler) GetAddress(c *gin.Context) {
	address := c.Param("address")
	if address == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "address is required"})
		return
	}

	info, err := h.repo.GetAddressInfo(c.Request.Context(), address)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if info == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "address not found"})
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	direction := c.Query("direction")
	if direction == "" {
		direction = c.Query("role") // fallback to 'role' if 'direction' is not provided
	}

	txs, err := h.repo.GetAddressTransactions(c.Request.Context(), address, direction, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"info":         info,
		"transactions": txs,
	})
}

func (h *Handler) GetTransaction(c *gin.Context) {
	txid := c.Param("txid")
	if txid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "txid is required"})
		return
	}

	tx, err := h.repo.GetTransaction(c.Request.Context(), txid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if tx == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "transaction not found"})
		return
	}

	c.JSON(http.StatusOK, tx)
}

func (h *Handler) GetTrace(c *gin.Context) {
	txid := c.Param("txid")
	if txid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "txid is required"})
		return
	}

	descendants, err := h.repo.GetTrace(c.Request.Context(), txid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"txid":        txid,
		"descendants": descendants,
	})
}
