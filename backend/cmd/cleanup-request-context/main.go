package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"time"

	_ "github.com/Wei-Shaw/sub2api/ent/runtime"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/lib/pq"
)

type cleanupStats struct {
	matched     int64
	cleared     int64
	storedBytes int64
}

const (
	maxBatchSize   = 500
	batchTimeout   = 15 * time.Second
	previewTimeout = 30 * time.Second
	batchPause     = 150 * time.Millisecond
)

func main() {
	beforeRaw := flag.String("before", "", "required RFC3339 cutoff; only older usage logs are considered")
	execute := flag.Bool("execute", false, "clear matched request contexts (default is dry-run)")
	batchSize := flag.Int("batch-size", 500, "rows updated per batch (1-500)")
	flag.Parse()

	if *beforeRaw == "" {
		log.Fatal("--before is required")
	}
	before, err := time.Parse(time.RFC3339, *beforeRaw)
	if err != nil {
		log.Fatalf("invalid --before: %v", err)
	}
	if *batchSize < 1 || *batchSize > maxBatchSize {
		log.Fatal("--batch-size must be between 1 and 500")
	}

	cfg, err := config.LoadForBootstrap()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	client, db, err := repository.InitEnt(cfg)
	if err != nil {
		log.Fatalf("initialize database: %v", err)
	}
	defer func() { _ = client.Close() }()

	stats, err := cleanup(context.Background(), db, before, *batchSize, *execute)
	if err != nil {
		log.Fatalf("cleanup failed: %v", err)
	}

	mode := "dry-run"
	if *execute {
		mode = "execute"
	}
	fmt.Printf("mode=%s before=%s matched=%d cleared=%d stored_bytes=%d\n",
		mode, before.UTC().Format(time.RFC3339), stats.matched, stats.cleared, stats.storedBytes)
	if *execute && stats.cleared > 0 {
		fmt.Println("cleanup complete; run VACUUM (ANALYZE) usage_logs during normal maintenance")
		fmt.Println("to return disk space to the OS, schedule VACUUM FULL usage_logs separately; it requires an exclusive table lock")
	}
}

func cleanup(ctx context.Context, db *sql.DB, before time.Time, batchSize int, execute bool) (cleanupStats, error) {
	if !execute {
		var stats cleanupStats
		queryCtx, cancel := context.WithTimeout(ctx, previewTimeout)
		defer cancel()
		err := db.QueryRowContext(queryCtx, `
			SELECT COUNT(*), COALESCE(SUM(pg_column_size(request_context_json)), 0)
			FROM usage_logs
			WHERE request_context_json IS NOT NULL
			  AND created_at < $1`, before).Scan(&stats.matched, &stats.storedBytes)
		return stats, err
	}

	var stats cleanupStats
	var cursor int64
	for {
		batchCtx, cancel := context.WithTimeout(ctx, batchTimeout)
		rows, err := db.QueryContext(batchCtx, `
			SELECT id, pg_column_size(request_context_json)
			FROM usage_logs
			WHERE id > $1
			  AND created_at < $2
			  AND request_context_json IS NOT NULL
			ORDER BY id ASC
			LIMIT $3`, cursor, before, batchSize)
		if err != nil {
			cancel()
			return stats, err
		}

		ids := make([]int64, 0, batchSize)
		var batchBytes int64
		for rows.Next() {
			var id, storedBytes int64
			if err := rows.Scan(&id, &storedBytes); err != nil {
				_ = rows.Close()
				cancel()
				return stats, err
			}
			ids = append(ids, id)
			batchBytes += storedBytes
			cursor = id
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			cancel()
			return stats, err
		}
		_ = rows.Close()
		if len(ids) == 0 {
			cancel()
			break
		}

		stats.matched += int64(len(ids))
		stats.storedBytes += batchBytes
		result, err := db.ExecContext(batchCtx, `
			UPDATE usage_logs
			SET request_context_json = NULL,
			    request_context_truncated = FALSE,
			    request_context_bytes = NULL
			WHERE id = ANY($1)
			  AND created_at < $2
			  AND request_context_json IS NOT NULL`, pq.Array(ids), before)
		if err != nil {
			cancel()
			return stats, err
		}
		cleared, err := result.RowsAffected()
		if err != nil {
			cancel()
			return stats, err
		}
		stats.cleared += cleared
		cancel()
		select {
		case <-ctx.Done():
			return stats, ctx.Err()
		case <-time.After(batchPause):
		}
	}

	return stats, nil
}
