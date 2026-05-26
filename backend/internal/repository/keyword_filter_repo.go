package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type keywordFilterRepository struct {
	db *sql.DB
}

func NewKeywordFilterRepository(db *sql.DB) service.KeywordFilterRepository {
	return &keywordFilterRepository{db: db}
}

func (r *keywordFilterRepository) CreateLog(ctx context.Context, log *service.KeywordFilterLog) error {
	if r == nil || r.db == nil || log == nil {
		return nil
	}
	var userID any
	if log.UserID != nil {
		userID = *log.UserID
	}
	var apiKeyID any
	if log.APIKeyID != nil {
		apiKeyID = *log.APIKeyID
	}
	var groupID any
	if log.GroupID != nil {
		groupID = *log.GroupID
	}
	err := r.db.QueryRowContext(ctx, `
INSERT INTO keyword_filter_logs (
    request_id, user_id, user_email, api_key_id, api_key_name, group_id, group_name,
    endpoint, provider, model, protocol, match_type, rule_name, matched_text,
    input_excerpt, input_hash, action, block_status
) VALUES (
    $1, $2, $3, $4, $5, $6, $7,
    $8, $9, $10, $11, $12, $13, $14,
    $15, $16, $17, $18
) RETURNING id, created_at`,
		log.RequestID, userID, log.UserEmail, apiKeyID, log.APIKeyName, groupID, log.GroupName,
		log.Endpoint, log.Provider, log.Model, log.Protocol, log.MatchType, log.RuleName, log.MatchedText,
		log.InputExcerpt, log.InputHash, log.Action, log.BlockStatus,
	).Scan(&log.ID, &log.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert keyword filter log: %w", err)
	}
	return nil
}

func (r *keywordFilterRepository) ListLogs(ctx context.Context, filter service.KeywordFilterLogFilter) ([]service.KeywordFilterLog, *pagination.PaginationResult, error) {
	where, args := buildKeywordFilterLogWhere(filter)
	whereSQL := "WHERE " + strings.Join(where, " AND ")

	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM keyword_filter_logs l "+whereSQL, args...).Scan(&total); err != nil {
		return nil, nil, fmt.Errorf("count keyword filter logs: %w", err)
	}

	params := filter.Pagination
	if params.Page <= 0 {
		params.Page = 1
	}
	if params.PageSize <= 0 {
		params.PageSize = 20
	}
	if params.PageSize > 100 {
		params.PageSize = 100
	}
	queryArgs := append([]any{}, args...)
	queryArgs = append(queryArgs, params.Limit(), params.Offset())
	rows, err := r.db.QueryContext(ctx, `
SELECT
    l.id, l.request_id, l.user_id, l.user_email, l.api_key_id, l.api_key_name, l.group_id, l.group_name,
    l.endpoint, l.provider, l.model, l.protocol, l.match_type, l.rule_name, l.matched_text,
    l.input_excerpt, l.input_hash, l.action, l.block_status, COALESCE(u.status, ''), l.created_at
FROM keyword_filter_logs l
LEFT JOIN users u ON u.id = l.user_id `+whereSQL+`
ORDER BY l.created_at DESC, l.id DESC
LIMIT $`+fmt.Sprint(len(queryArgs)-1)+` OFFSET $`+fmt.Sprint(len(queryArgs)),
		queryArgs...,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("list keyword filter logs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := make([]service.KeywordFilterLog, 0)
	for rows.Next() {
		var item service.KeywordFilterLog
		var userID, apiKeyID, groupID sql.NullInt64
		if err := rows.Scan(
			&item.ID,
			&item.RequestID,
			&userID,
			&item.UserEmail,
			&apiKeyID,
			&item.APIKeyName,
			&groupID,
			&item.GroupName,
			&item.Endpoint,
			&item.Provider,
			&item.Model,
			&item.Protocol,
			&item.MatchType,
			&item.RuleName,
			&item.MatchedText,
			&item.InputExcerpt,
			&item.InputHash,
			&item.Action,
			&item.BlockStatus,
			&item.UserStatus,
			&item.CreatedAt,
		); err != nil {
			return nil, nil, fmt.Errorf("scan keyword filter log: %w", err)
		}
		if userID.Valid {
			v := userID.Int64
			item.UserID = &v
		}
		if apiKeyID.Valid {
			v := apiKeyID.Int64
			item.APIKeyID = &v
		}
		if groupID.Valid {
			v := groupID.Int64
			item.GroupID = &v
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate keyword filter logs: %w", err)
	}
	return items, paginationResultFromTotal(total, params), nil
}

func (r *keywordFilterRepository) CleanupExpiredLogs(ctx context.Context, before time.Time) (*service.KeywordFilterCleanupResult, error) {
	result := &service.KeywordFilterCleanupResult{FinishedAt: time.Now()}
	if r == nil || r.db == nil {
		return result, nil
	}
	exec, err := r.db.ExecContext(ctx, `
DELETE FROM keyword_filter_logs
WHERE created_at < $1
`, before)
	if err != nil {
		return nil, fmt.Errorf("delete expired keyword filter logs: %w", err)
	}
	result.Deleted, _ = exec.RowsAffected()
	result.FinishedAt = time.Now()
	return result, nil
}

func buildKeywordFilterLogWhere(filter service.KeywordFilterLogFilter) ([]string, []any) {
	where := []string{"l.id IS NOT NULL"}
	args := make([]any, 0)
	add := func(expr string, value any) {
		args = append(args, value)
		where = append(where, fmt.Sprintf(expr, len(args)))
	}
	switch strings.ToLower(strings.TrimSpace(filter.MatchType)) {
	case service.KeywordFilterMatchTypeKeyword:
		where = append(where, "l.match_type = 'keyword'")
	case service.KeywordFilterMatchTypeRegex:
		where = append(where, "l.match_type = 'regex'")
	}
	if filter.GroupID != nil {
		add("l.group_id = $%d", *filter.GroupID)
	}
	if endpoint := strings.TrimSpace(filter.Endpoint); endpoint != "" {
		add("l.endpoint = $%d", endpoint)
	}
	if search := strings.TrimSpace(filter.Search); search != "" {
		like := "%" + escapePostgresLike(search) + "%"
		args = append(args, like, like, like, like, like, like)
		idx := len(args) - 5
		where = append(where, fmt.Sprintf("(l.request_id ILIKE $%d ESCAPE '\\' OR l.user_email ILIKE $%d ESCAPE '\\' OR l.api_key_name ILIKE $%d ESCAPE '\\' OR l.model ILIKE $%d ESCAPE '\\' OR l.rule_name ILIKE $%d ESCAPE '\\' OR l.input_excerpt ILIKE $%d ESCAPE '\\')", idx, idx+1, idx+2, idx+3, idx+4, idx+5))
	}
	if filter.From != nil && !filter.From.IsZero() {
		add("l.created_at >= $%d", *filter.From)
	}
	if filter.To != nil && !filter.To.IsZero() {
		add("l.created_at <= $%d", *filter.To)
	}
	return where, args
}

func escapePostgresLike(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `%`, `\%`)
	value = strings.ReplaceAll(value, `_`, `\_`)
	return value
}
