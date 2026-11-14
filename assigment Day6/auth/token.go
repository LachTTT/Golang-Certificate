package auth

import (
	"errors"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type Claims struct {
	UserID uint `json:"user_id"`
	jwt.RegisteredClaims
}

func secret() []byte { return []byte(os.Getenv("JWT_SECRET")) }

func GenerateToken(userID uint) (string, string, time.Time, error) {
	jti := uuid.NewString()
	exp := time.Now().Add(24 * time.Hour)

	claims := Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,
			ExpiresAt: jwt.NewNumericDate(exp),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	tokenString, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret())
	return tokenString, jti, exp, err
}

func ParseToken(tokenString string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims,
		func(t *jwt.Token) (any, error) { return secret(), nil },
		jwt.WithValidMethods([]string{"HS256"}),
	)
	if err != nil || !token.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}