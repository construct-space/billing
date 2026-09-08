package polar

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const baseURL = "https://api.polar.sh/v1"

type Client struct {
	Token string
	HTTP  *http.Client
}

func New(token string) *Client {
	return &Client{
		Token: token,
		HTTP:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) do(method, path string, body any) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal: %w", err)
		}
		reader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, baseURL+path, reader)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("polar api %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

func (c *Client) CreateProduct(name, description string, priceInCents int) (*Product, error) {
	payload := map[string]any{
		"name":        name,
		"description": description,
		"prices": []map[string]any{
			{
				"amount_type":    "fixed",
				"price_amount":   priceInCents,
				"price_currency": "eur",
			},
		},
	}
	data, err := c.do("POST", "/products/", payload)
	if err != nil {
		return nil, err
	}
	var product Product
	return &product, json.Unmarshal(data, &product)
}

func (c *Client) ListProducts() (*ProductList, error) {
	data, err := c.do("GET", "/products/?limit=100", nil)
	if err != nil {
		return nil, err
	}
	var list ProductList
	return &list, json.Unmarshal(data, &list)
}

func (c *Client) CreateCheckout(req *CheckoutRequest) (*Checkout, error) {
	data, err := c.do("POST", "/checkouts/", req)
	if err != nil {
		return nil, err
	}
	var checkout Checkout
	return &checkout, json.Unmarshal(data, &checkout)
}

func (c *Client) CreateCustomerSession(externalCustomerID, returnURL string) (*CustomerSession, error) {
	payload := CustomerSessionRequest{
		ExternalCustomerID: externalCustomerID,
		ReturnURL:          returnURL,
	}
	data, err := c.do("POST", "/customer-sessions/", payload)
	if err != nil {
		return nil, err
	}
	var session CustomerSession
	return &session, json.Unmarshal(data, &session)
}

func (c *Client) GetCheckout(id string) (*Checkout, error) {
	data, err := c.do("GET", "/checkouts/"+id, nil)
	if err != nil {
		return nil, err
	}
	var checkout Checkout
	return &checkout, json.Unmarshal(data, &checkout)
}

func (c *Client) ListOrders(query string) (*OrderList, error) {
	path := "/orders/?limit=100"
	if query != "" {
		path += "&" + query
	}
	data, err := c.do("GET", path, nil)
	if err != nil {
		return nil, err
	}
	var list OrderList
	return &list, json.Unmarshal(data, &list)
}

func (c *Client) GetOrder(id string) (*Order, error) {
	data, err := c.do("GET", "/orders/"+id, nil)
	if err != nil {
		return nil, err
	}
	var order Order
	return &order, json.Unmarshal(data, &order)
}
