package jwtutil

import (
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UserID   uint   `json:"uid"`
	Username string `json:"uname"`
	Role     string `json:"role"`
	DeviceID string `json:"device_id"`
	jwt.RegisteredClaims
}

type Signer struct {
	Secret []byte
	Issuer string
	ExpMin int
}

func (s *Signer) Sign(userID uint, username, role string) (string, error) {
	now := time.Now()
	exp := now.Add(time.Duration(s.ExpMin) * time.Minute)
	claims := Claims{
		UserID: userID, Username: username, Role: role,
		RegisteredClaims: jwt.RegisteredClaims{Issuer: s.Issuer, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(exp)},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.Secret)
}

func (s *Signer) Parse(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) { return s.Secret, nil })
	if err != nil {
		return nil, err
	}
	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}
	return nil, jwt.ErrTokenInvalidClaims
}
