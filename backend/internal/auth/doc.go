// Package auth provides public-key challenge authentication, account routes,
// and JWT middleware for Fiber handlers.
//
// Access tokens are HS256 JWTs with these claims:
//   - iss: configured service issuer
//   - sub: canonical account UUID
//   - iat: issue time
//   - exp: expiration time
//   - jti: random token UUID
//
// RegisterRoutes(router, service) installs the B1 HTTP endpoints. Other
// packages protect handlers with service.Middleware() and retrieve identity
// with AccountIDFromContext(*fiber.Ctx) (uuid.UUID, bool).
package auth
