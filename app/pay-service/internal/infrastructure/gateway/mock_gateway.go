package gateway

import (
	"fmt"

	"tickethub/app/pay-service/internal/domain/pay"
)

type MockGateway struct {
	BaseURL string
}

func NewMockGateway(baseURL string) MockGateway {
	if baseURL == "" {
		baseURL = "https://pay.local/tickethub"
	}
	return MockGateway{BaseURL: baseURL}
}

func (g MockGateway) Pay(payment pay.Payment) (string, error) {
	return fmt.Sprintf("%s/pay?order_number=%d&amount_cent=%d", g.BaseURL, payment.OrderNumber, payment.AmountCent), nil
}

func (g MockGateway) Check(orderNumber int64, channel string) (pay.Trade, error) {
	return pay.Trade{OrderNumber: orderNumber, Channel: channel, Paid: false}, nil
}

func (g MockGateway) Refund(refund pay.Refund) error {
	return nil
}
