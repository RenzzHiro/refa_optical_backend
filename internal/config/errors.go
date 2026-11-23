package config

import "errors"

var ErrJWTSecret = errors.New("JWT_SECRET must contain at least 32 characters")
var ErrInvalidConfig = errors.New("token TTL and upload size must be positive")
