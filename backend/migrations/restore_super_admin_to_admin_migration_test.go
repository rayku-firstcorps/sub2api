package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRestoreSuperAdminToAdminMigration(t *testing.T) {
	content, err := FS.ReadFile("239_restore_super_admin_to_admin.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "UPDATE users")
	require.Contains(t, sql, "role = 'admin'")
	require.Contains(t, sql, "WHERE role = 'super_admin'")
	require.NotContains(t, sql, "DROP TABLE")
}
