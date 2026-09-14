package response

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func withRouter(t *testing.T, setup func(g *gin.Engine)) (*http.Response, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	setup(r)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	r.ServeHTTP(w, req)
	return w.Result(), w.Body.String()
}

func TestPaginatedNilItemsBecomesArray(t *testing.T) {
	resp, body := withRouter(t, func(r *gin.Engine) {
		r.GET("/", func(c *gin.Context) {
			var nilItems []string
			Paginated(c, nilItems, 0, 1, 25)
		})
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if strings.Contains(body, "null") {
		t.Fatalf("body must not contain null: %s", body)
	}
	var parsed struct {
		Items []string `json:"items"`
		Total int64    `json:"total"`
	}
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if parsed.Items == nil {
		t.Fatal("items must be [] not null")
	}
	if len(parsed.Items) != 0 {
		t.Fatalf("expected empty items, got %+v", parsed.Items)
	}
}

func TestPaginatedKeepsProvidedItems(t *testing.T) {
	_, body := withRouter(t, func(r *gin.Engine) {
		r.GET("/", func(c *gin.Context) {
			Paginated(c, []int{1, 2, 3}, 3, 1, 25)
		})
	})
	if !strings.Contains(body, "\"items\":[1,2,3]") && !strings.Contains(body, `"items":[1,2,3]`) {
		t.Fatalf("expected items preserved: %s", body)
	}
	if !strings.Contains(body, `"total":3`) {
		t.Fatalf("expected total 3: %s", body)
	}
}

func TestErrorEnvelopeShape(t *testing.T) {
	_, body := withRouter(t, func(r *gin.Engine) {
		r.GET("/", func(c *gin.Context) {
			Forbidden(c)
		})
	})
	if !strings.Contains(body, `"code":"forbidden"`) {
		t.Fatalf("missing code: %s", body)
	}
	if !strings.Contains(body, `"message":"Insufficient permissions"`) {
		t.Fatalf("missing message: %s", body)
	}
}