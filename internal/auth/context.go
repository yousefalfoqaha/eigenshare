package auth

import (
	"context"

	"eigenshare/internal/user"
)

type userKey struct{}

func WithUser(ctx context.Context, u *user.User) context.Context {
	return context.WithValue(ctx, userKey{}, u)
}

func UserFromContext(ctx context.Context) *user.User {
	u, _ := ctx.Value(userKey{}).(*user.User)
	return u
}
