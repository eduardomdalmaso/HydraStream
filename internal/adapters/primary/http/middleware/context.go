package middleware

import (
	"context"
)

type contextKey string

const (
	tenantIDKey contextKey = "tenant_id"
	userIDKey   contextKey = "user_id"
	userRoleKey contextKey = "user_role"
)

// WithTenantContext injects tenant ID into context.
func WithTenantContext(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, tenantIDKey, tenantID)
}

// GetTenantID retrieves tenant ID from context.
func GetTenantID(ctx context.Context) string {
	if val, ok := ctx.Value(tenantIDKey).(string); ok {
		return val
	}
	return ""
}

// WithUserContext injects user ID and role into context.
func WithUserContext(ctx context.Context, userID, role string) context.Context {
	ctx = context.WithValue(ctx, userIDKey, userID)
	return context.WithValue(ctx, userRoleKey, role)
}

// GetUserID retrieves user ID from context.
func GetUserID(ctx context.Context) string {
	if val, ok := ctx.Value(userIDKey).(string); ok {
		return val
	}
	return ""
}

// GetUserRole retrieves user role from context.
func GetUserRole(ctx context.Context) string {
	if val, ok := ctx.Value(userRoleKey).(string); ok {
		return val
	}
	return ""
}
