package pay

type PaymentStatus string

const (
	PaymentCreated  PaymentStatus = "CREATED"
	PaymentPaid     PaymentStatus = "PAID"
	PaymentClosed   PaymentStatus = "CLOSED"
	PaymentRefunded PaymentStatus = "REFUNDED"
)

type Payment struct {
	OrderNumber int64
	AmountCent  int64
	Channel     string
	Status      PaymentStatus
}

type Refund struct {
	RequestID   string
	OrderNumber int64
	AmountCent  int64
	Reason      string
	Success     bool
}

type Trade struct {
	OrderNumber int64
	Channel     string
	Paid        bool
}

type Gateway interface {
	Pay(payment Payment) (string, error)
	Check(orderNumber int64, channel string) (Trade, error)
	Refund(refund Refund) error
}
