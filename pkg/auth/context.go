package auth

import "context"

const (
	UserIDHeader = "X-TicketHub-User-ID"
	RoleHeader   = "X-TicketHub-Role"
)

type UserContext struct {
	UserID int64
	Role   string
}

type userContextKey struct{}

func WithUser(ctx context.Context, user UserContext) context.Context {
	return context.WithValue(ctx, userContextKey{}, user)
}

func UserFromContext(ctx context.Context) (UserContext, bool) {
	user, ok := ctx.Value(userContextKey{}).(UserContext)
	return user, ok
}
