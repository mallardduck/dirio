package auth

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mallardduck/dirio/internal/persistence/metadata"
	"github.com/mallardduck/dirio/pkg/iam"
)

func TestAuthenticator_ValidateCredentials(t *testing.T) {
	tests := []struct {
		name          string
		setupAuth     func() *Authenticator
		testAccessKey string
		testSecretKey string
		wantValid     bool
	}{
		{
			name: "primary root credentials work",
			setupAuth: func() *Authenticator {
				return New(nil, "primary-key", "primary-secret")
			},
			testAccessKey: "primary-key",
			testSecretKey: "primary-secret",
			wantValid:     true,
		},
		{
			name: "alternative root credentials work",
			setupAuth: func() *Authenticator {
				auth := New(nil, "primary-key", "primary-secret")
				return auth.WithAlternativeRoot("alt-key", "alt-secret")
			},
			testAccessKey: "alt-key",
			testSecretKey: "alt-secret",
			wantValid:     true,
		},
		{
			name: "both primary and alternative credentials work when different",
			setupAuth: func() *Authenticator {
				auth := New(nil, "primary-key", "primary-secret")
				return auth.WithAlternativeRoot("alt-key", "alt-secret")
			},
			testAccessKey: "primary-key",
			testSecretKey: "primary-secret",
			wantValid:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth := tt.setupAuth()
			got := auth.ValidateCredentials(context.Background(), tt.testAccessKey, tt.testSecretKey)
			assert.Equal(t, tt.wantValid, got, "ValidateCredentials result mismatch")
		})
	}
}

func TestAuthenticator_GetUserForAccessKey(t *testing.T) {
	tests := []struct {
		name          string
		setupAuth     func() *Authenticator
		testAccessKey string
		wantUser      *metadata.User
		wantErr       bool
	}{
		{
			name: "primary root access key returns user",
			setupAuth: func() *Authenticator {
				return New(nil, "primary-key", "primary-secret")
			},
			testAccessKey: "primary-key",
			wantUser: &metadata.User{
				UUID:      iam.AdminUserUUID,
				Username:  "admin",
				AccessKey: "primary-key",
				SecretKey: "primary-secret",
				Status:    "on",
			},
			wantErr: false,
		},
		{
			name: "alternative root access key returns user",
			setupAuth: func() *Authenticator {
				auth := New(nil, "primary-key", "primary-secret")
				return auth.WithAlternativeRoot("alt-key", "alt-secret")
			},
			testAccessKey: "alt-key",
			wantUser: &metadata.User{
				UUID:      iam.AdminUserUUID,
				Username:  "admin",
				AccessKey: "alt-key",
				SecretKey: "alt-secret",
				Status:    "on",
			},
			wantErr: false,
		},
		{
			name: "both primary and alternative return correct user",
			setupAuth: func() *Authenticator {
				auth := New(nil, "primary-key", "primary-secret")
				return auth.WithAlternativeRoot("alt-key", "alt-secret")
			},
			testAccessKey: "primary-key",
			wantUser: &metadata.User{
				UUID:      iam.AdminUserUUID,
				Username:  "admin",
				AccessKey: "primary-key",
				SecretKey: "primary-secret",
				Status:    "on",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth := tt.setupAuth()
			got, err := auth.GetUserForAccessKey(context.Background(), tt.testAccessKey)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantUser, got)
		})
	}
}

func TestAuthenticator_WithAlternativeRoot(t *testing.T) {
	t.Run("alternative root can be added", func(t *testing.T) {
		auth := New(nil, "primary-key", "primary-secret")
		assert.Empty(t, auth.altRootAccessKey)
		assert.Empty(t, auth.altRootSecretKey)

		auth = auth.WithAlternativeRoot("alt-key", "alt-secret")
		assert.Equal(t, "alt-key", auth.altRootAccessKey)
		assert.Equal(t, "alt-secret", auth.altRootSecretKey)
	})

	t.Run("primary root credentials unchanged after adding alternative", func(t *testing.T) {
		auth := New(nil, "primary-key", "primary-secret")
		auth = auth.WithAlternativeRoot("alt-key", "alt-secret")

		assert.Equal(t, "primary-key", auth.rootAccessKey)
		assert.Equal(t, "primary-secret", auth.rootSecretKey)
	})
}
