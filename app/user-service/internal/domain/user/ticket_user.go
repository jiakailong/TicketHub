package user

type TicketUser struct {
	ID            int64
	UserID        int64
	Name          string
	CertificateNo string
	Mobile        string
}

func (t TicketUser) BelongsTo(userID int64) bool {
	return t.UserID == userID
}
