package middleware

import (
	"context"
	jwtutil "sagiri-guard/backend/app/jwt"
)

func GetClaims(ctx context.Context) *jwtutil.Claims {
	if v := ctx.Value(ClaimsKey); v != nil {
		if c, ok := v.(*jwtutil.Claims); ok {
			return c
		}
	}
	return nil
}
