package payment

import (
	"bytes"
	"context"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/refa-optical/backend/internal/models"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type SnapResult struct {
	Token       string `json:"token"`
	RedirectURL string `json:"redirect_url"`
}
type StatusResult struct {
	OrderID           string `json:"order_id"`
	TransactionID     string `json:"transaction_id"`
	TransactionStatus string `json:"transaction_status"`
	FraudStatus       string `json:"fraud_status"`
	GrossAmount       string `json:"gross_amount"`
}
type Midtrans struct {
	serverKey     string
	baseURL       string
	statusBaseURL string
	httpClient    *http.Client
}

func NewMidtrans(serverKey string, production bool) *Midtrans {
	snapBase := "https://app.sandbox.midtrans.com"
	statusBase := "https://api.sandbox.midtrans.com"
	if production {
		snapBase = "https://app.midtrans.com"
		statusBase = "https://api.midtrans.com"
	}
	return &Midtrans{serverKey: serverKey, baseURL: snapBase, statusBaseURL: statusBase, httpClient: &http.Client{Timeout: 20 * time.Second}}
}

type snapItem struct {
	ID       string `json:"id"`
	Price    int64  `json:"price"`
	Quantity int    `json:"quantity"`
	Name     string `json:"name"`
}
type snapPayload struct {
	TransactionDetails struct {
		OrderID     string `json:"order_id"`
		GrossAmount int64  `json:"gross_amount"`
	} `json:"transaction_details"`
	Items    []snapItem `json:"item_details"`
	Customer struct {
		FirstName string `json:"first_name"`
		Email     string `json:"email"`
		Phone     string `json:"phone"`
	} `json:"customer_details"`
	Callbacks struct {
		Finish string `json:"finish"`
	} `json:"callbacks"`
}

func (m *Midtrans) CreateSnap(ctx context.Context, order models.Order, user models.User, frontendURL string) (SnapResult, error) {
	var body snapPayload
	body.TransactionDetails.OrderID = order.Number
	body.TransactionDetails.GrossAmount = order.Total
	body.Callbacks.Finish = strings.TrimRight(frontendURL, "/") + "/payment/success?order_id=" + url.QueryEscape(order.Number)
	body.Customer.FirstName = user.Name
	body.Customer.Email = user.Email
	body.Customer.Phone = order.Address.Phone
	for _, item := range order.Items {
		body.Items = append(body.Items, snapItem{ID: item.ProductID, Price: item.Price, Quantity: item.Quantity, Name: item.Name})
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return SnapResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.baseURL+"/snap/v1/transactions", bytes.NewReader(raw))
	if err != nil {
		return SnapResult{}, err
	}
	req.SetBasicAuth(m.serverKey, "")
	req.Header.Set("Content-Type", "application/json")
	res, err := m.httpClient.Do(req)
	if err != nil {
		return SnapResult{}, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		var detail map[string]any
		_ = json.NewDecoder(res.Body).Decode(&detail)
		return SnapResult{}, fmt.Errorf("midtrans returned %s: %v", res.Status, detail)
	}
	var result SnapResult
	if err = json.NewDecoder(res.Body).Decode(&result); err != nil {
		return SnapResult{}, err
	}
	if result.Token == "" || result.RedirectURL == "" {
		return SnapResult{}, fmt.Errorf("midtrans response did not include token or redirect url")
	}
	return result, nil
}
func (m *Midtrans) GetStatus(ctx context.Context, orderID string) (StatusResult, error) {
	endpoint := m.statusBaseURL + "/v2/" + url.PathEscape(orderID) + "/status"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return StatusResult{}, err
	}
	req.SetBasicAuth(m.serverKey, "")
	req.Header.Set("Accept", "application/json")
	res, err := m.httpClient.Do(req)
	if err != nil {
		return StatusResult{}, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return StatusResult{}, fmt.Errorf("midtrans status returned %s", res.Status)
	}
	var status StatusResult
	if err = json.NewDecoder(res.Body).Decode(&status); err != nil {
		return StatusResult{}, err
	}
	return status, nil
}
func VerifyNotification(orderID, statusCode, grossAmount, signature, serverKey string) bool {
	sum := sha512.Sum512([]byte(orderID + statusCode + grossAmount + serverKey))
	expected := hex.EncodeToString(sum[:])
	return len(expected) == len(signature) && subtle.ConstantTimeCompare([]byte(expected), []byte(signature)) == 1
}
