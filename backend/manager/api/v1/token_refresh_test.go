package v1

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tbdavid2019/888a2a/backend/manager/api/auth"
)

func TestValidateRefreshToken_Fingerprint(t *testing.T) {
	now := time.Now()
	validToken := refreshStoredToken{
		ID:          1,
		PrincipalID: 10,
		Family:      "family_1",
		State:       1,
		Fingerprint: "hw_fingerprint_abc",
		ExpiresAt:   now.Add(1 * time.Hour),
	}

	dummyParse := func(_ string) (int, string, error) {
		return 1, auth.TokenTypeRefresh, nil
	}
	dummyReuse := func(_ int32) refreshAction {
		return refreshActionProceed
	}
	dummyRevoke := func(_ string) error {
		return nil
	}
	dummyLoad := func(_ int) (refreshPrincipal, error) {
		return refreshPrincipal{ID: 10, TokenVersion: 1}, nil
	}

	t.Run("matching fingerprint succeeds", func(t *testing.T) {
		lookup := func(_ string) (refreshStoredToken, error) {
			return validToken, nil
		}
		p, st, err := validateRefreshToken(
			context.Background(),
			"dummy_token_str",
			"hw_fingerprint_abc",
			"",
			dummyParse,
			lookup,
			dummyReuse,
			dummyRevoke,
			dummyLoad,
		)
		require.NoError(t, err)
		assert.Equal(t, 10, p.ID)
		assert.Equal(t, "hw_fingerprint_abc", st.Fingerprint)
	})

	t.Run("empty fingerprint when stored has fingerprint fails with PermissionDenied", func(t *testing.T) {
		lookup := func(_ string) (refreshStoredToken, error) {
			return validToken, nil
		}
		_, _, err := validateRefreshToken(
			context.Background(),
			"dummy_token_str",
			"",
			"",
			dummyParse,
			lookup,
			dummyReuse,
			dummyRevoke,
			dummyLoad,
		)
		require.Error(t, err)
		var connectErr *connect.Error
		require.ErrorAs(t, err, &connectErr)
		assert.Equal(t, connect.CodePermissionDenied, connectErr.Code())
		assert.Contains(t, connectErr.Message(), "fingerprint mismatch")
	})

	t.Run("mismatch fingerprint fails with PermissionDenied", func(t *testing.T) {
		lookup := func(_ string) (refreshStoredToken, error) {
			return validToken, nil
		}
		_, _, err := validateRefreshToken(
			context.Background(),
			"dummy_token_str",
			"hw_attacker_fp",
			"",
			dummyParse,
			lookup,
			dummyReuse,
			dummyRevoke,
			dummyLoad,
		)
		require.Error(t, err)
		var connectErr *connect.Error
		require.ErrorAs(t, err, &connectErr)
		assert.Equal(t, connect.CodePermissionDenied, connectErr.Code())
		assert.Contains(t, connectErr.Message(), "fingerprint mismatch")
	})

	t.Run("stored without fingerprint allows empty request fingerprint", func(t *testing.T) {
		legacyToken := validToken
		legacyToken.Fingerprint = ""
		lookup := func(_ string) (refreshStoredToken, error) {
			return legacyToken, nil
		}
		p, _, err := validateRefreshToken(
			context.Background(),
			"dummy_token_str",
			"",
			"",
			dummyParse,
			lookup,
			dummyReuse,
			dummyRevoke,
			dummyLoad,
		)
		require.NoError(t, err)
		assert.Equal(t, 10, p.ID)
	})

	t.Run("lookup error fails", func(t *testing.T) {
		errLookup := func(_ string) (refreshStoredToken, error) {
			return refreshStoredToken{}, assert.AnError
		}
		_, _, err := validateRefreshToken(
			context.Background(),
			"dummy_token_str",
			"hw_fingerprint_abc",
			"",
			dummyParse,
			errLookup,
			dummyReuse,
			dummyRevoke,
			dummyLoad,
		)
		require.Error(t, err)
	})
}
