package user

import "time"

type RealNameStatus string

const (
	RealNameUnverified RealNameStatus = "unverified"
	RealNameVerified   RealNameStatus = "verified"
)

type User struct {
	ID             int64
	Mobile         string
	PasswordHash   string
	Email          string
	RealNameStatus RealNameStatus
	CreatedAt      time.Time
}

func NewUser(id int64, mobile string, passwordHash string, now time.Time) User {
	return User{
		ID:             id,
		Mobile:         mobile,
		PasswordHash:   passwordHash,
		RealNameStatus: RealNameUnverified,
		CreatedAt:      now,
	}
}

func (u *User) MarkRealNameVerified() {
	u.RealNameStatus = RealNameVerified
}
