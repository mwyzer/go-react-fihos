package handler

import (
	"time"

	"github.com/gin-gonic/gin"

	"fihos/backend/internal/middleware"
	"fihos/backend/internal/response"
	"fihos/backend/internal/service"
	"fihos/backend/internal/store"
)

type batchReq struct {
	Name        string    `json:"name"`
	Quantity    int       `json:"quantity"`
	Price       *float64  `json:"price"`
	Duration    int       `json:"duration"`
	ProfileID   int64     `json:"profile_id"`
	ValidFrom   time.Time `json:"valid_from"`
	ValidTo     time.Time `json:"valid_to"`
	GenerateNow bool      `json:"generate_now"`
}

func (h *H) PostBatch(c *gin.Context) {
	tid := h.requireTenant(c)
	if tid == 0 {
		return
	}
	var req batchReq
	if !h.bind(c, &req) {
		return
	}
	if req.Quantity <= 0 || req.Quantity > 500 {
		response.ValidationFailed(c, gin.H{"quantity": "must be 1..500"})
		return
	}
	if req.Duration <= 0 {
		response.ValidationFailed(c, gin.H{"duration": "minutes must be positive"})
		return
	}
	if req.ProfileID == 0 {
		response.ValidationFailed(c, gin.H{"profile_id": "required"})
		return
	}
	if req.Name == "" {
		req.Name = "Batch " + time.Now().Format("20060102-1504")
	}
	if req.Price == nil {
		z := 0.0
		req.Price = &z
	}

	uid := middleware.UserID(c)
	codesRes, err := service.GenerateBatchCodes(req.Quantity)
	if err != nil {
		response.Internal(c, err)
		return
	}
	var vf, vt *time.Time
	if !req.ValidFrom.IsZero() {
		vf = &req.ValidFrom
	}
	if !req.ValidTo.IsZero() {
		vt = &req.ValidTo
	}
	batch, vouchers, err := h.St.CreateBatch(c.Request.Context(), tid, &store.Batch{
		Name:        req.Name,
		Quantity:    req.Quantity,
		Price:       req.Price,
		Duration:    req.Duration,
		ProfileID:   req.ProfileID,
		ValidFrom:   vf,
		ValidTo:     vt,
		CreatedBy:   &uid,
	}, codesRes)
	if err != nil {
		h.fail(c, err, "")
		return
	}
	h.audit(c, "batch.create", "batch", &batch.ID, gin.H{"quantity": req.Quantity, "price": req.Price})
	response.Ok(c, 201, gin.H{"batch": batch, "generated": len(vouchers)})
}

func (h *H) ListBatches(c *gin.Context) {
	tid := h.requireTenant(c)
	if tid == 0 {
		return
	}
	page, size := h.page(c)
	items, total, err := h.St.ListBatches(c.Request.Context(), tid, page, size)
	if err != nil {
		response.Internal(c, err)
		return
	}
	response.Paginated(c, items, total, page, size)
}

func (h *H) GetBatch(c *gin.Context) {
	tid := h.requireTenant(c)
	if tid == 0 {
		return
	}
	id, ok := middleware.ParamID(c, "id")
	if !ok {
		return
	}
	batch, err := h.St.BatchSales(c.Request.Context(), tid, id)
	if err != nil {
		h.fail(c, err, "Batch not found")
		return
	}
	response.Ok(c, 200, batch)
}

func (h *H) ListBatchVouchers(c *gin.Context) {
	tid := h.requireTenant(c)
	if tid == 0 {
		return
	}
	id, ok := middleware.ParamID(c, "id")
	if !ok {
		return
	}
	page, size := h.page(c)
	items, total, err := h.St.ListVouchers(c.Request.Context(), tid, id, c.Query("status"), c.Query("q"), page, size)
	if err != nil {
		response.Internal(c, err)
		return
	}
	response.Paginated(c, items, total, page, size)
}

func (h *H) GetVoucher(c *gin.Context) {
	tid := h.requireTenant(c)
	if tid == 0 {
		return
	}
	id, ok := middleware.ParamID(c, "id")
	if !ok {
		return
	}
	v, err := h.St.VoucherByID(c.Request.Context(), tid, id)
	if err != nil {
		h.fail(c, err, "Voucher not found")
		return
	}
	response.Ok(c, 200, v)
}

func (h *H) PostVoucherRevoke(c *gin.Context) {
	tid := h.requireTenant(c)
	if tid == 0 {
		return
	}
	id, ok := middleware.ParamID(c, "id")
	if !ok {
		return
	}
	if err := h.St.RevokeVoucher(c.Request.Context(), tid, id); err != nil {
		h.fail(c, err, "Voucher not found or already claimed")
		return
	}
	h.audit(c, "voucher.revoke", "voucher", &id, nil)
	c.Status(204)
}