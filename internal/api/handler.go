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
	return &Handler{
		repo: repo,
	}
}

func (h *Handler) GetAddress(c *gin.Context) {
	address := c.Param("address")
	if address == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "address is required",
		})
		return
	}

	ctx := c.Request.Context()

	// --------------------------------------------------
	// Query params
	// --------------------------------------------------

	limit, err := strconv.Atoi(c.DefaultQuery("limit", "100"))
	if err != nil || limit <= 0 {
		limit = 100
	}

	// Hard limit protection
	if limit > 500 {
		limit = 500
	}

	offset, err := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if err != nil || offset < 0 {
		offset = 0
	}

	direction := c.Query("direction")
	if direction == "" {
		direction = c.Query("role")
	}

	// --------------------------------------------------
	// Mode
	//
	// compact -> use cached address_balances only
	// compute -> recompute full stats dynamically
	//
	// default = compact
	// --------------------------------------------------

	mode := c.DefaultQuery("mode", "compact")

	var (
		info *AddressInfo
	)

	switch mode {

	case "compute":

		// Full expensive recomputation
		info, err = h.repo.GetAddressInfoCompute(
			ctx,
			address,
		)

	default:

		// Fast cached mode
		info, err = h.repo.GetAddressInfoCompact(
			ctx,
			address,
		)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	if info == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "address not found",
		})
		return
	}

	// --------------------------------------------------
	// Transactions
	// --------------------------------------------------

	txs, err := h.repo.GetAddressTransactions(
		ctx,
		address,
		direction,
		limit,
		offset,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	// --------------------------------------------------
	// Response
	// --------------------------------------------------

	c.JSON(http.StatusOK, gin.H{
		"mode":         mode,
		"info":         info,
		"transactions": txs,
		"limit":        limit,
		"offset":       offset,
	})
}
func (h *Handler) GetTransaction(c *gin.Context) {
	txid := c.Param("txid")
	if txid == "" {
		c.AbortWithStatusJSON(
			http.StatusBadRequest,
			gin.H{"error": "txid is required"},
		)
		return
	}

	tx, err := h.repo.GetTransaction(
		c.Request.Context(),
		txid,
	)
	if err != nil {
		c.AbortWithStatusJSON(
			http.StatusInternalServerError,
			gin.H{"error": err.Error()},
		)
		return
	}

	if tx == nil {
		c.AbortWithStatusJSON(
			http.StatusNotFound,
			gin.H{"error": "transaction not found"},
		)
		return
	}

	c.JSON(http.StatusOK, tx)
}

func (h *Handler) GetTrace(c *gin.Context) {
	txid := c.Param("txid")
	if txid == "" {
		c.AbortWithStatusJSON(
			http.StatusBadRequest,
			gin.H{"error": "txid is required"},
		)
		return
	}

	descendants, err := h.repo.GetTrace(
		c.Request.Context(),
		txid,
	)
	if err != nil {
		c.AbortWithStatusJSON(
			http.StatusInternalServerError,
			gin.H{"error": err.Error()},
		)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"txid":        txid,
		"descendants": descendants,
	})
}
