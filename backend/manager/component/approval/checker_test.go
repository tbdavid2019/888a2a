package approval

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tbdavid2019/888a2a/backend/a2a"
	"github.com/tbdavid2019/888a2a/backend/manager/store"
)

func TestCheckerFailsClosedWhenDurableApprovalIsUnavailable(t *testing.T) {
	checker := NewChecker(nil, store.ApprovalBinding{
		OrganizationID: "org-a", RequesterPrincipal: "user-a", PolicyName: "policy", PolicyVersion: "1",
	})
	result, err := checker(context.Background(), a2a.PermissionRequest{ActionKind: a2a.ActionShell, Command: "git status"})
	require.Error(t, err)
	require.Equal(t, a2a.DecisionDeny, result.Decision)
}

func TestCheckerRejectsIncompleteBinding(t *testing.T) {
	checker := NewChecker(store.NewApprovalStore(nil), store.ApprovalBinding{})
	_, err := checker(context.Background(), a2a.PermissionRequest{ActionKind: a2a.ActionShell})
	require.Error(t, err)
}
