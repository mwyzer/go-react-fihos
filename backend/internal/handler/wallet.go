package handler

import (
	"github.com/gin-gonic/gin"

	"fihos/backend/internal/response"
	"fihos/backend/internal/store"
)

func (h *H) GetCustomerWallet(c *gin.Context) {
	tid := h.requireTenant(c)
	if tid == 0 {
		return
	}
	id, err := store.ParseID(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid customer id", nil)
		return
	}
	balance, err := h.St.CustomerBalance(c.Request.Context(), tid, id)
	if err != nil {
		h.fail(c, err, "Customer not found")
		return
	}
	page, size := h.page(c)
	items, total, err := h.St.ListWalletTransactions(c.Request.Context(), tid, id, page, size)
	if err != nil {
		response.Internal(c, err)
		return
	}
	response.Ok(c, 200, gin.H{
		"customer_id":  id,
		"balance":      balance,
		"transactions": gin.H{"items": items, "total": total, "page": page, "size": size},
	})
}

type topUpReq struct {
	Amount float64 `json:"amount"`
}

// PostCustomerTopup starts a wallet top-up through the configured gateway.
// Async providers return a payment_url the owner shares with the customer.
func (h *H) PostCustomerTopup(c *gin.Context) {
	tid := h.requireTenant(c)
	if tid == 0 {
		return
	}
	id, err := store.ParseID(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid customer id", nil)
		return
	}
	var req topUpReq
	if !h.bind(c, &req) {
		return
	}
	if req.Amount <= 0 || req.Amount > 1_000_000_000 {
		response.ValidationFailed(c, gin.H{"amount": "invalid"})
		return
	}
	if _, err := h.St.CustomerBalance(c.Request.Context(), tid, id); err != nil {
		h.fail(c, err, "Customer not found")
		return
	}
	charge, err := h.Billing.ChargeTopUp(c.Request.Context(), tid, id, req.Amount)
	if err != nil {
		h.fail(c, err, "")
		return
	}
	h.audit(c, "customer.topup", "customer", &id, gin.H{"amount": charge.Payment.Amount, "ref": charge.Payment.ExternalRef})
	response.Ok(c, 201, gin.H{
		"payment":     charge.Payment,
		"payment_url": charge.PaymentURL,
		"async":       charge.Async,
	})
}

type adjustWalletReq struct {
	Amount float64 `json:"amount"`
	Note   string  `json:"note"`
}

// PostWalletAdjust applies a manual cash correction to a customer's balance.
// Positive amounts credit, negative amounts debit.
func (h *H) PostWalletAdjust(c *gin.Context) {
	tid := h.requireTenant(c)
	if tid == 0 {
		return
	}
	id, err := store.ParseID(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid customer id", nil)
		return
	}
	var req adjustWalletReq
	if !h.bind(c, &req) {
		return
	}
	if req.Amount == 0 || req.Amount > 1_000_000_000 {
		response.ValidationFailed(c, gin.H{"amount": "invalid"})
		return
	}
	if err := h.St.AdjustWallet(c.Request.Context(), tid, id, req.Amount, req.Note); err != nil {
		h.fail(c, err, "Customer not found")
		return
	}
	h.audit(c, "customer.wallet_adjust", "customer", &id, gin.H{"amount": req.Amount, "note": req.Note})
	balance, _ := h.St.CustomerBalance(c.Request.Context(), tid, id)
	response.Ok(c, 200, gin.H{"id": id, "adjusted": req.Amount, "balance": balance})
}
