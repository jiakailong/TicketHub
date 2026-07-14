package security

import (
	"golang.org/x/crypto/bcrypt"
)

type BcryptPasswordHasher struct {
	Cost int
}

func NewBcryptPasswordHasher(cost int) BcryptPasswordHasher {
	if cost < bcrypt.MinCost || cost > bcrypt.MaxCost {
		cost = bcrypt.DefaultCost
	}
	return BcryptPasswordHasher{Cost: cost}
}

func (h BcryptPasswordHasher) Hash(password string) (string, error) {
	value, err := bcrypt.GenerateFromPassword([]byte(password), h.Cost)
	return string(value), err
}

func (h BcryptPasswordHasher) Compare(hash string, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
