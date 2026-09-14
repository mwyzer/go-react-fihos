package handler

import (
	"time"

	"github.com/gin-gonic/gin"

	"fihos/backend/internal/response"
	"fihos/backend/internal/store"
)

var customerStatuses = map[string]bool{
	"active": true, "token": true, "expired": true, "suspended": true,
}

var windowStatuses = map[string]bool{
	"draft": true, "issued": true, "paid": true, "overdue": true,
}

func (h *H) ListCustomers(c *gin.Context) {
	tid := h.requireTenant(c)
	if tid == 0 {
		return
	}
	page, size := h.page(c)
	items, total, err := h.St.ListCustomers(c.Request.Context(), tid, c.Query("status"), page, size)
	if err != nil {
		response.Internal(c, err)
		return
	}
	response.Paginated(c, items, total, page, size)
}

func (h *H) CustomerOverview(c *gin.Context) {
	tid := h.requireTenant(c)
	if tid == 0 {
		return
	}
	counts, err := h.St.CountCustomersByStatus(c.Request.Context(), tid)
	if err != nil {
		response.Internal(c, err)
		return
	}
	response.Ok(c, 200, counts)
}

type customerReq struct {
	HotspotID *int64 `json:"hotspot_id"`
	Name      string `json:"name"`
	Phone     string `json:"phone"`
	Address   string `json:"address"`
	Status    string `json:"status"`
}

func (h *H) PostCustomer(c *gin.Context) {
	tid := h.requireTenant(c)
	if tid == 0 {
		return
	}
	var req customerReq
	if !h.bind(c, &req) {
		return
	}
	if req.Name == "" {
		response.ValidationFailed(c, gin.H{"name": "required"})
		return
	}
	if req.Status == "" {
		req.Status = "active"
	}
	if !customerStatuses[req.Status] {
		response.ValidationFailed(c, gin.H{"status": "invalid"})
		return
	}
	created, err := h.St.CreateCustomer(c.Request.Context(), tid, &store.Customer{
		HotspotID: req.HotspotID, Name: req.Name, Phone: req.Phone, Address: req.Address, Status: req.Status,
	})
	if err != nil {
		h.fail(c, err, "")
		return
	}
	h.audit(c, "customer.created", "customer", &created.ID, nil)
	response.Ok(c, 201, created)
}

func (h *H) PatchCustomerStatus(c *gin.Context) {
	tid := h.requireTenant(c)
	if tid == 0 {
		return
	}
	id, err := store.ParseID(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid customer id", nil)
		return
	}
	var req struct {
		Status string `json:"status"`
	}
	if !h.bind(c, &req) {
		return
	}
	if !customerStatuses[req.Status] {
		response.ValidationFailed(c, gin.H{"status": "invalid"})
		return
	}
	if err := h.St.PatchCustomerStatus(c.Request.Context(), tid, id, req.Status); err != nil {
		h.fail(c, err, "Customer not found")
		return
	}
	h.audit(c, "customer.status_changed", "customer", &id, gin.H{"status": req.Status})
	response.Ok(c, 200, gin.H{"id": id, "status": req.Status})
}

func (h *H) ListBillingWindows(c *gin.Context) {
	tid := h.requireTenant(c)
	if tid == 0 {
		return
	}
	customerID, _ := store.ParseID(c.Query("customer_id"))
	page, size := h.page(c)
	items, total, err := h.St.ListBillingWindows(c.Request.Context(), tid, customerID, c.Query("status"), page, size)
	if err != nil {
		response.Internal(c, err)
		return
	}
	response.Paginated(c, items, total, page, size)
}

func (h *H) PostBillingGenerate(c *gin.Context) {
	tid := h.requireTenant(c)
	if tid == 0 {
		return
	}
	n, err := h.St.GenerateMonthlyWindows(c.Request.Context(), tid, time.Now())
	if err != nil {
		response.Internal(c, err)
		return
	}
	h.audit(c, "billing.generate", "billing_window", nil, gin.H{"created": n})
	response.Ok(c, 201, gin.H{"generated": n})
}

func (h *H) PostBillingPay(c *gin.Context) {
	tid := h.requireTenant(c)
	if tid == 0 {
		return
	}
	id, err := store.ParseID(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid window id", nil)
		return
	}
	w, err := h.St.MarkWindowPaid(c.Request.Context(), tid, id)
	if err != nil {
		h.fail(c, err, "Billing window not found")
		return
	}
	h.audit(c, "billing.paid", "billing_window", &w.ID, nil)
	response.Ok(c, 200, w)
}

func (h *H) GetBillingOverview(c *gin.Context) {
	tid := h.requireTenant(c)
	if tid == 0 {
		return
	}
	pending, err := h.St.ListPendingWindows(c.Request.Context(), tid)
	if err != nil {
		response.Internal(c, err)
		return
	}
	fee, _ := h.St.MonthlyFee(c.Request.Context(), tid)
	counts := map[string]int64{}
	var totalDue float64
	for _, w := range pending {
		counts[w.Status]++
		totalDue += w.Amount
	}
	response.Ok(c, 200, gin.H{"monthly_fee": fee, "pending": counts, "amount_due": totalDue})
}
