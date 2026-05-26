# 关键词过滤与风控链路 Bug Review PRD

## 1. 背景

- Review 日期：2026-05-26
- Review 范围：当前工作区未提交的关键词过滤相关改动，以及其调用链路、日志查询、前端管理页。
- 关联模块：`backend/internal/service/keyword_filter*.go`、`backend/internal/handler/*keyword*`、`backend/internal/repository/keyword_filter_repo.go`、`frontend/src/views/admin/KeywordFilterView.vue`。
- 验证结果：
  - `go test ./internal/service -run KeywordFilter` 未通过，暴露运行时开关缓存缺陷。
  - `go test ./internal/handler -run KeywordFilter` 通过，但当前没有实际匹配用例执行。
  - `npm.cmd run build` 未执行成功，原因是 `frontend/node_modules` 不存在，`vue-tsc` 未安装。

## 2. 目标

1. 修复关键词过滤可绕过、延迟生效、误判漏判、日志性能和前端交互一致性问题。
2. 为后端服务、WebSocket 网关、前端管理页建立可回归的验收用例。
3. 避免测试工具与保存配置、网关实际行为不一致。

## 3. 非目标

- 不重做完整内容安全系统。
- 不替换 Go `regexp`。Go 标准库正则为 RE2 语义，不存在传统灾难性回溯 ReDoS；本 PRD 只处理输入截断和配置一致性问题。
- 不在本 PRD 内设计多租户审计报表。

## 4. Bug 清单

### P0 BUG-KF-001：`<system-reminder>` 前缀仍可绕过关键词过滤

- 位置：`backend/internal/service/keyword_filter_input.go:174`、`backend/internal/service/keyword_filter_input.go:201`
- 关联风险：`backend/internal/service/content_moderation_input.go:312` 仍使用 `strings.Contains`
- 现象：当前修复把 `Contains` 改成了 `HasPrefix`，但任意用户只要把消息写成 `<system-reminder>敏感词`，该文本段仍会被跳过。
- 影响：攻击者无需权限即可绕过关键词过滤；同类逻辑还可能影响内容审核链路。
- 修复要求：
  1. 不允许基于用户可控文本字面量跳过检测。
  2. 如必须跳过内部 system reminder，必须依赖结构化来源、内部标记或网关注入上下文，而不是消息内容前缀。
  3. OpenAI、Anthropic、Gemini、Responses 协议中的用户文本即使包含 `<system-reminder>` 也必须被扫描。
- 验收标准：
  - 用户消息 `<system-reminder>敏感词` 被关键词规则命中。
  - 用户消息 `正常文本 <system-reminder> 敏感词` 被关键词规则命中。
  - 如存在内部注入的 system reminder，只有内部注入片段被排除，用户片段不被排除。

### P1 BUG-KF-002：`keyword_filter_enabled` 总开关变化不会立即生效

- 位置：`backend/internal/service/keyword_filter.go:484`
- 证据：`go test ./internal/service -run KeywordFilter` 失败于 `TestKeywordFilterService_CheckRespectsSwitches`。
- 现象：`runtimeSnapshot` 在 5 秒 TTL 内直接返回旧快照，快照里包含 `systemEnabled`。管理员开启关键词过滤后，网关可能继续放行；管理员关闭后，网关可能继续拦截。
- 影响：管理后台展示与实际网关行为短时间不一致；紧急启用拦截时存在窗口期。
- 修复要求：
  1. 总开关变更后必须立即影响网关检查。
  2. 可以选择每次检查单独读取总开关，或在设置更新时显式失效关键词过滤运行时缓存。
  3. 保留规则编译缓存，但不能缓存过期的总开关状态。
- 验收标准：
  - 从 `keyword_filter_enabled=false` 切到 `true` 后，下一次请求立即被规则拦截。
  - 从 `true` 切到 `false` 后，下一次请求立即放行。
  - 新增单测覆盖开关双向切换。

### P1 BUG-KF-003：WebSocket 历史窗口手写 JSON 转义不完整

- 位置：`backend/internal/handler/keyword_filter_helper.go:143`
- 现象：`syntheticBody` 只转义反斜杠和双引号，没有处理换行、回车、制表符和其他控制字符。一旦历史窗口中含这些字符，合成 JSON 可能无效，`ExtractKeywordFilterTexts` 返回空，跨帧检测失效。
- 影响：WebSocket 分片绕过修复不稳定；带换行的自然文本也可能漏检。
- 修复要求：
  1. 使用 `encoding/json` 构造合成请求体，禁止手写 JSON 字符串拼接。
  2. 合成体必须与 `ContentModerationProtocolOpenAIResponses` 的提取逻辑完全兼容。
  3. 记录或返回合成体解析失败的可观测信号，不能静默放行。
- 验收标准：
  - WebSocket 连续发送 `敏感\n` 和 `词` 时仍能命中 `敏感词`。
  - 带双引号、反斜杠、换行、制表符的历史窗口不会产生非法 JSON。

### P1 BUG-KF-004：正则规则只扫描前 8192 rune，长输入后半段漏检

- 位置：`backend/internal/service/keyword_filter.go:751`
- 现象：`matchCompiledRegexRules` 将 `RegexText` 截断到前 8192 rune 后再匹配。关键词规则仍扫描全文，正则规则会漏掉后半段的手机号、URL、token 等结构化内容。
- 影响：长 prompt 可绕过正则过滤。
- 修复要求：
  1. 不允许静默截断导致漏检。
  2. 可选方案：分块扫描并保留跨块 overlap；或对超长输入返回明确的阻断/错误策略；或限制单段输入并在网关层统一处理。
  3. 分块 overlap 至少覆盖最大正则规则长度或明确配置上限。
- 验收标准：
  - 在第 9000 个字符之后出现的启用正则规则仍能命中。
  - 边界跨块的手机号/URL 仍能命中。
  - 超长输入处理有日志或指标，不静默降级。

### P2 BUG-KF-005：测试端点与保存配置校验不一致

- 位置：`backend/internal/service/keyword_filter.go:388`、`backend/internal/service/keyword_filter.go:594`
- 现象：`Test` 当前会编译正则，但没有走完整 `validateConfig`。例如纯文本正则、规则数量、分组有效性等可能在测试和保存时表现不同。
- 影响：管理员在测试弹窗中看到“可用/命中”，保存时失败，或测试了一个生产不可保存的配置。
- 修复要求：
  1. 测试端点必须复用保存配置的校验规则。
  2. 测试端点返回的错误码和错误信息应与保存接口一致。
  3. 前端测试按钮不应绕过后端校验。
- 验收标准：
  - 测试接口提交纯文本正则 `敏感词` 返回 `KEYWORD_FILTER_REGEX_LITERAL_PATTERN`。
  - 测试接口提交不存在的分组 ID 返回 `INVALID_KEYWORD_FILTER_GROUP`。
  - 测试接口和保存接口对同一 payload 结果一致。

### P2 BUG-KF-006：日志搜索索引 migration 有上线锁表和缺索引风险

- 位置：`backend/migrations/139_keyword_filter_logs_trgm_indexes.sql`
- 现象：
  - 在普通事务迁移里创建多个 GIN trigram 索引，大表上线可能长时间阻塞写入。
  - 只在 `pg_trgm` 已存在时建索引，不尝试创建扩展。
  - 查询条件包含 `request_id ILIKE`，但 migration 未给 `request_id` 建 trigram 索引。
- 影响：日志量较大时部署和搜索都会有性能风险。
- 修复要求：
  1. 拆分为事务内 `CREATE EXTENSION IF NOT EXISTS pg_trgm` 和 `_notx.sql` 中的 `CREATE INDEX CONCURRENTLY IF NOT EXISTS`。
  2. 覆盖 `request_id`、`user_email`、`api_key_name`、`model`、`rule_name`、`input_excerpt`。
  3. 符合现有 migration runner 对 `_notx.sql` 的限制。
- 验收标准：
  - migration runner 接受新迁移。
  - 100 万级日志表上建索引不阻塞 `CreateLog` 写入。
  - `EXPLAIN` 显示搜索命中 trigram 索引或有可解释的降级路径。

### P2 BUG-KF-007：`all_groups=false` 且 `group_ids=[]` 的后端语义与 UI 不一致

- 位置：`backend/internal/service/keyword_filter.go:1470`
- 现象：UI 禁止保存空分组范围，但后端 `includesGroup` 把空 `GroupIDs` 当作全部分组生效。
- 影响：通过 API 或旧配置导入时，管理员本意可能是“未选择任何分组”，实际变成“全部分组拦截”。
- 修复要求：
  1. 后端保存配置时拒绝 `all_groups=false && group_ids=[]`。
  2. 历史空配置迁移需要明确策略：自动改为 `all_groups=true` 并记录，或保持禁用并提示修复。
  3. UI 与 API 语义必须一致。
- 验收标准：
  - 保存空分组范围返回明确 400。
  - 读取历史异常配置时有确定的兼容行为。

### P2 BUG-KF-008：前端日志请求取消后 loading 状态可能被旧请求覆盖

- 位置：`frontend/src/views/admin/KeywordFilterView.vue:719`
- 现象：新请求会 abort 旧请求，但旧请求的 `finally` 仍会执行 `logsLoading=false`。快速切换筛选条件时，新请求仍在执行，loading 可能已消失。
- 影响：页面状态误导用户；后续错误提示也可能与当前筛选条件不一致。
- 修复要求：
  1. 使用 request sequence 或比较当前 `AbortController`，只有最新请求可以写入 loading 和数据。
  2. 组件卸载时取消请求。
- 验收标准：
  - 快速切换筛选条件时，列表最终只展示最新请求结果。
  - 最新请求完成前 loading 不提前消失。

### P3 BUG-KF-009：白名单 UI 只支持单目标规则

- 位置：`frontend/src/views/admin/KeywordFilterView.vue:600`
- 现象：数据模型支持多个 `target_rule_ids`，UI 只读写 `[0]`。
- 影响：管理员无法通过 UI 配置“同一白名单覆盖多条关键词规则”。
- 修复要求：
  1. 目标规则选择器支持多选。
  2. 保留“全部规则”选项，且与多选互斥。
  3. 保存和导入导出保持 `target_rule_ids` 数组。
- 验收标准：
  - UI 可选择多个目标关键词规则。
  - 保存后刷新页面，多个目标仍保留。

## 5. 已修复项回归清单

以下问题在当前工作区已有修复痕迹，需要保留回归测试：

| 编号 | 回归点 | 文件 |
| --- | --- | --- |
| REG-001 | AC 自动机同一位置应报告所有输出，避免短词被长词吞掉 | `backend/internal/service/keyword_filter_matcher.go` |
| REG-002 | `UpdateConfig` 并发更新不应互相覆盖运行时快照 | `backend/internal/service/keyword_filter.go` |
| REG-003 | 过期日志清理应分批删除，不长时间锁表 | `backend/internal/repository/keyword_filter_repo.go` |
| REG-004 | 导入关键词/白名单时，无 `type` 的文件按入口按钮类型导入 | `frontend/src/views/admin/KeywordFilterView.vue` |
| REG-005 | 首次直达关键词过滤页时，公共设置未加载不应误重定向 | `frontend/src/router/index.ts` |
| REG-006 | 删除关键词、白名单、正则规则前有确认弹窗 | `frontend/src/views/admin/KeywordFilterView.vue` |
| REG-007 | 中文/英文正则校验文案完整，不依赖硬编码 fallback | `frontend/src/i18n/locales/*.ts` |

## 6. 测试矩阵

| 场景 | 覆盖问题 | 优先级 |
| --- | --- | --- |
| 用户文本以 `<system-reminder>` 开头且包含敏感词 | BUG-KF-001 | P0 |
| 用户文本中间包含 `<system-reminder>` 且包含敏感词 | BUG-KF-001 | P0 |
| 开启/关闭 `keyword_filter_enabled` 后立即请求 | BUG-KF-002 | P1 |
| WebSocket 分两帧发送敏感词，第一帧含换行或引号 | BUG-KF-003 | P1 |
| 长输入第 9000 字符后出现手机号或 URL | BUG-KF-004 | P1 |
| 测试接口提交保存接口会拒绝的配置 | BUG-KF-005 | P2 |
| 百万日志表执行 trigram index migration | BUG-KF-006 | P2 |
| API 保存 `all_groups=false` 且空分组 | BUG-KF-007 | P2 |
| 快速切换日志筛选条件 | BUG-KF-008 | P2 |
| 白名单选择多个目标规则并刷新 | BUG-KF-009 | P3 |

## 7. 发布要求

1. P0/P1 修复必须先合并，并补齐单元测试。
2. migration 变更必须在测试库跑完整迁移，并验证 `_notx.sql` 执行路径。
3. 前端需先安装依赖后执行 `npm.cmd run build`。
4. 后端至少执行：
   - `go test ./internal/service -run KeywordFilter`
   - `go test ./internal/handler -run KeywordFilter`
   - 如有数据库环境，执行 repository/migration 相关集成测试。

## 8. 验收出口

- P0/P1 全部关闭。
- P2 至少有修复或明确延期记录。
- `keyword_filter_enabled` 开关行为与管理后台展示一致。
- 关键词过滤、内容审核、WebSocket 历史检测没有基于用户文本字面量的跳过逻辑。
- PR 合并前附上测试输出和 migration 验证结果。
