package schedule

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidate(t *testing.T) {
	cases := []struct {
		name     string
		cronExpr string
		tz       string
		wantErr  bool
	}{
		{"valid standard cron", "0 12 * * *", "UTC", false},
		{"valid every 5 minutes", "*/5 * * * *", "", false},
		{"invalid cron format", "not-a-cron", "", true},
		{"invalid timezone", "0 12 * * *", "Invalid/Timezone", true},
		{"impossible calendar date", "0 0 30 2 *", "UTC", true},
		{"impossible date Feb 31", "0 0 31 2 *", "", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Validate(tc.cronExpr, tc.tz)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestNextFire(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	t.Run("valid next fire", func(t *testing.T) {
		next, err := NextFire("0 12 * * *", "UTC", from)
		require.NoError(t, err)
		assert.False(t, next.IsZero())
		assert.Equal(t, time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC), next)
	})

	t.Run("impossible date returns error", func(t *testing.T) {
		_, err := NextFire("0 0 30 2 *", "UTC", from)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "has no valid fire times")
	})
}
