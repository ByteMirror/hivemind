package memory

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenDB_CreatesAllTables(t *testing.T) {
	db, err := openDB(t.TempDir() + "/test.db")
	require.NoError(t, err)
	defer db.Close()

	// Verify all tables exist
	tables := []string{"files", "chunks", "chunks_fts", "chunks_vec", "embedding_cache"}
	for _, table := range tables {
		var name string
		row := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table)
		err := row.Scan(&name)
		require.NoError(t, err, "table %q should exist", table)
	}
}
