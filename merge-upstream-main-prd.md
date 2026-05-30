# Fork 合并上游 main PRD

## 1. 背景

当前仓库是 `rayku-firstcorps/sub2api` 对源仓库 `Wei-Shaw/sub2api` 的 fork。源仓库 `upstream/main` 已有新提交，需要合并到当前 fork 的 `main` 分支，并保留 fork 已实现的 Kiro、关键词过滤、调度优化等能力。

本次分析基于 2026-05-30 本地执行 `git fetch --all --prune` 后的状态：

- 当前分支：`main`
- 当前 fork HEAD：`99b3f7b0`，提交信息：`优化调度`
- 上游 HEAD：`f18451e5`，提交信息：`Merge pull request #2891 from DaydreamCoding/feat/platform-quota-conn-optimization`
- merge-base：`f7ac5e5931aaa4b3f7ec5c683dda12c01efb2164`
- 分叉差异：当前 fork 领先上游 29 个提交，上游领先当前 fork 64 个提交
- 工作区状态：存在未提交修改 `deploy/keyword_filter_rules.json`

## 2. 合并目标

1. 将 `upstream/main` 合并到 fork 的 `main`。
2. 保留 fork 特性：
   - Kiro 平台适配与代理链路。
   - 关键词过滤服务、规则导入、管理页面和运行时拦截。
   - 使用量请求上下文记录。
   - 本地调度和 WebSocket 相关优化。
3. 引入上游特性和修复：
   - `v0.1.133` 版本更新。
   - group 自定义 `/v1/models` 模型列表。
   - OpenAI embeddings gateway。
   - WebSocket / OAuth / Responses 兼容性修复。
   - user x platform 配额写入聚合 flusher。
   - OpenAI endpoint capability gating。
   - 账号配额阈值自动暂停。
   - 多项 billing、content moderation、concurrency、pricing 修复。

## 3. 当前冲突判断

通过 `git merge-tree --write-tree main upstream/main` 和临时 worktree 执行 `git merge --no-commit --no-ff upstream/main` 验证，Git 层面的真实内容冲突只有 3 个文件：

| 文件 | 冲突类型 | 原因 | 处理原则 |
| --- | --- | --- | --- |
| `backend/cmd/server/wire.go` | 内容冲突 | fork 新增 `KeywordFilterService` 清理；上游新增 `UserPlatformQuotaUsageFlusher` 清理 | 两者都保留，`provideCleanup` 参数和 `parallelSteps` 都加入 |
| `backend/cmd/server/wire_gen.go` | 内容冲突 | 由 `wire.go` 生成的同类冲突 | 先按 `wire.go` 解决，再运行 wire 重新生成 |
| `backend/internal/handler/openai_gateway_handler.go` | 内容冲突 | fork 的关键词过滤、请求上下文记录，与上游的 WS failover、capability 选择、sticky session、usage task parent context 改动交叠 | 以合并功能为目标，不二选一，保留双方行为 |

除上述 3 个文件外，Git 可自动合并大量后端、前端、测试和迁移文件，但仍需要重点验证语义兼容。

## 4. 关键冲突解决方案

### 4.1 `backend/cmd/server/wire.go`

`provideCleanup` 的参数列表需要同时包含：

- `keywordFilterService *service.KeywordFilterService`
- `quotaFlusher *service.UserPlatformQuotaUsageFlusher`

`parallelSteps` 中需要同时关闭：

- `KeywordFilterService`
- `UserPlatformQuotaUsageFlusher`

推荐顺序：保持现有服务清理顺序，在 `ChannelMonitorRunner` 后依次加入 `KeywordFilterService` 和 `UserPlatformQuotaUsageFlusher`。两者都需要 nil 判断。

### 4.2 `backend/cmd/server/wire_gen.go`

不要手工长期维护该文件。解决方式：

1. 先修复 `wire.go`。
2. 执行 Wire 生成命令，按仓库现有方式生成 `wire_gen.go`。
3. 检查生成文件无冲突标记。

建议命令：

```powershell
cd backend
go generate ./cmd/server
```

如果仓库没有配置对应 generate，可用：

```powershell
cd backend
go run github.com/google/wire/cmd/wire ./cmd/server
```

### 4.3 `backend/internal/handler/openai_gateway_handler.go`

冲突集中在 `ResponsesWebSocket`。

必须保留 fork 行为：

- 首条 WS 消息执行 `wsKwSession.checkWithHistory(...)`。
- 后续 turn 的 `BeforeRequest` 继续执行关键词过滤。
- 被关键词过滤阻断时调用 `writeKeywordFilterWSError(...)` 并返回 `OpenAIWSClientCloseError`。
- `AfterTurn` 记录 usage 时保留 `PrepareUsageLogRequestContext(firstMessage)` 产生的：
  - `RequestContextJSON`
  - `RequestContextTruncated`
  - `RequestContextBytes`

必须保留上游行为：

- 使用 `SelectAccountWithSchedulerForCapability(...)`，并传入 `service.OpenAIEndpointCapabilityChatCompletions`。
- 支持 `failedAccountIDs`、`maxAccountSwitches`、`lastFailoverErr` 的 WS upstream failover 循环。
- 账号槽释放使用 `releaseAccountSlot()`，failover 后可继续选择下一个账号。
- 保留 `ensureUserSlotHeld()`，避免 failover 重试时用户并发槽丢失。
- 保留 `BindStickySession(...)`。
- usage record 提交使用新版签名 `submitOpenAIUsageRecordTask(ctx, result, task)`。
- 保留 OAuth 账号 `UpdateCodexUsageSnapshotFromHeaders(...)`。

推荐合并结构：

1. 保留上游 `for { ... SelectAccountWithSchedulerForCapability ... }` 外层循环。
2. 在每轮选中账号后，按上游逻辑处理账号并发槽、sticky session、access token。
3. 构造 `OpenAIWSIngressHooks` 时，把 fork 的关键词过滤检查加回 `BeforeRequest`，位置放在 content moderation 之前。
4. `AfterTurn` 中使用上游新版 `submitOpenAIUsageRecordTask(ctx, result, ...)`，同时补回 fork 的请求上下文字段。
5. `ProxyResponsesWebSocketFromClient(...)` 出错时保留上游 failover 分支，非 failover 错误才关闭客户端。

## 5. 语义风险

1. 当前工作区有未提交的 `deploy/keyword_filter_rules.json` 修改。合并前必须先 commit 或 stash，避免本地规则扩展在合并过程中被覆盖或干扰检查。
2. 迁移文件存在多个重复编号段，例如 `136_*`、`137_*`、`138_*`、`139_*`、`140_*`。本次上游新增 `143_group_models_list_config.sql` 和 `144_add_opus48_to_model_mapping.sql`，没有直接文件名冲突，但需要确认迁移执行器是否只按文件名排序且允许同编号多文件。
3. DI 生成文件较多，`wire.go`、`wire_gen.go`、`backend/internal/service/wire.go`、`backend/internal/repository/wire.go` 均需编译验证。
4. 前端 `RiskControlView`、`SettingsView`、`AccountsView`、`GroupsView` 均有自动合并，需跑 typecheck 和相关 vitest，重点验证关键词过滤入口和上游新增 group models list UI 是否共存。
5. OpenAI WebSocket 合并属于高风险路径，需覆盖：
   - 首条消息关键词过滤阻断。
   - 后续 turn 关键词过滤阻断。
   - content moderation 阻断。
   - 429/上游 failover 切换账号。
   - usage record 包含 request context。
   - OAuth 账号 usage header 快照更新。

## 6. 执行计划

### 6.1 合并前准备

```powershell
git status --short --branch
git fetch --all --prune
git rev-list --left-right --count main...upstream/main
```

处理未提交文件：

```powershell
git add deploy/keyword_filter_rules.json
git commit -m "chore: expand keyword filter rules"
```

如果暂不想提交：

```powershell
git stash push -m "wip keyword filter rules" -- deploy/keyword_filter_rules.json
```

### 6.2 创建合并分支

```powershell
git switch main
git switch -c merge-upstream-main-20260530
git merge --no-ff upstream/main
```

### 6.3 解决冲突

按第 4 节处理 3 个冲突文件。

检查冲突标记：

```powershell
rg -n "^<<<<<<<|^=======|^>>>>>>>" .
git diff --check
```

重新生成 DI：

```powershell
cd backend
go generate ./cmd/server
```

### 6.4 验证

后端验证：

```powershell
cd backend
go test ./...
```

前端验证：

```powershell
cd frontend
pnpm install
pnpm typecheck
pnpm test:run
pnpm build
```

针对冲突路径建议追加重点验证：

```powershell
cd backend
go test ./internal/handler -run "OpenAI|WebSocket|Usage|Moderation|Keyword"
go test ./internal/service -run "OpenAI|WebSocket|Failover|Keyword|Quota|Billing"
go test ./internal/repository -run "Keyword|Quota|Usage"
```

### 6.5 合并完成

```powershell
git status --short
git log --oneline --decorate --graph --max-count=20
git push origin merge-upstream-main-20260530
```

## 7. 验收标准

1. `git status --short` 无未解决冲突。
2. `rg -n "^<<<<<<<|^=======|^>>>>>>>" .` 无输出。
3. `go test ./...` 通过，或失败项有明确非本次合并引入的说明。
4. `pnpm typecheck`、`pnpm test:run`、`pnpm build` 通过。
5. OpenAI Responses WebSocket 仍支持：
   - 关键词过滤。
   - 内容审计。
   - account failover。
   - sticky session。
   - usage request context 记录。
6. 管理后台仍支持：
   - 关键词过滤配置。
   - 风控设置。
   - 上游新增的 group models list 配置。
   - 账号配额阈值相关字段。

## 8. 不在本次范围

1. 不重构关键词过滤架构。
2. 不调整历史迁移编号策略，除非验证发现迁移执行器无法处理当前编号重复。
3. 不变更 fork 的远端默认分支和发布流程。
4. 不删除 fork 已有 Kiro 支持代码。
