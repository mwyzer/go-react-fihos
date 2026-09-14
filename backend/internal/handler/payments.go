package handler

import (
	"github.com/gin-gonic/gin"

	"fihos/backend/internal/response"
	"fihos/backend/internal/service"
)

// PostPaymentComplete settles a pending payment on gateway callback. Real
// aggregation gateways sign their webhooks; this endpoint verifies an
// HMAC-SHA256 signature over "ref|amount" before touching the row.
func (h *H) PostPaymentComplete(c *gin.Context) {
	ref := c.Param("ref")
	var req struct {
		Amount    float64 `json:"amount"`
		Signature string  `json:"signature"`
	}
	if !h.bind(c, &req) {
		return
	}
	if req.Signature == "" || !service.VerifyPaymentSignature(h.PaymentSecret, ref, req.Amount, req.Signature) {
		response.Error(c, 401, "invalid_signature", "Payment signature mismatch", nil)
		return
	}
	p, err := h.Billing.Settle(c.Request.Context(), ref)
	if err != nil {
		h.fail(c, err, "Payment not found")
		return
	}
	response.Ok(c, 200, p)
}

// GetPaymentStatus exposes settlement status by external ref for clients that
// poll before connecting (QRIS/VA flows).
func (h *H) GetPaymentStatus(c *gin.Context) {
	ref := c.Param("ref")
	p, err := h.St.PaymentByRefGlobal(c.Request.Context(), ref)
	if err != nil {
		h.fail(c, err, "Payment not found")
		return
	}
	response.Ok(c, 200, p)
}
