# 关键词过滤与风控系统 — 业务逻辑 PRD

## 1. 概述

关键词过滤是 sub2api 网关的实时内容风控模块，对所有经过网关的 AI 请求进行关键词和正则规则扫描，在请求到达上游模型之前拦截违规内容。

**核心能力：**
- 多协议支持（OpenAI Chat、Anthropic Messages、Gemini、OpenAI Responses）
- 多匹配模式（精确短语、模糊、分词、CJK 分词、正则）
- 白名单豁免机制（全局或定向）
- WebSocket 跨帧检测
- 管理后台配置、测试、日志审计

---

## 2. 系统架构

```
用户请求 → API 网关 → 关键词过滤检查 → 上游模型
                          ↓ (命中)
                     返回拦截响应 + 记录日志
```

**关键路径：**
1. HTTP 请求：网关 handler 调用 `Check()` → 命中则返回配置的 HTTP 状态码和消息
2. WebSocket 请求：维护滑动窗口（512 rune），检测跨帧拆分的敏感词

---

## 3. 配置模型

### 3.1 全局配置

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `enabled` | bool | false | 页面级开关（需配合系统总开关 `keyword_filter_enabled`） |
| `all_groups` | bool | true | 是否对所有分组生效 |
| `group_ids` | int64[] | [] | 指定生效的分组 ID（`all_groups=false` 时必填） |
| `block_status` | int | 403 | 拦截时返回的 HTTP 状态码（400-599） |
| `block_message` | string | "输入内容命中关键词过滤规则，请调整后重试" | 拦截时返回的错误消息 |
| `hit_retention_days` | int | 180 | 命中日志保留天数（最大 3650） |

### 3.2 关键词规则

| 字段 | 说明 |
|------|------|
| `id` | 自动生成的唯一标识 |
| `pattern` | 关键词模式（最长 256 rune） |
| `match_mode` | 匹配模式（见 3.4） |
| `enabled` | 是否启用 |
| `action` | 动作（当前仅 `block`） |

### 3.3 白名单规则

| 字段 | 说明 |
|------|------|
| `id` | 自动生成的唯一标识 |
| `pattern` | 白名单模式（最长 256 rune） |
| `match_mode` | 匹配模式 |
| `target_rule_ids` | 目标关键词规则 ID（空 = 全局白名单） |
| `enabled` | 是否启用 |

### 3.4 正则规则

| 字段 | 说明 |
|------|------|
| `name` | 规则名称（必填） |
| `pattern` | 正则表达式（最长 512 rune，必须包含正则语法） |
| `enabled` | 是否启用 |
| `builtin` | 是否为内置规则（phone_cn、url） |

### 3.5 匹配模式

| 模式 | 标识 | 适用场景 | 行为 |
|------|------|----------|------|
| 自动推断 | `auto` | 默认 | 根据模式内容自动选择最佳匹配方式 |
| 包含 | `contains` | 长词、短语 | 文本中任意位置出现即命中 |
| 模糊 | `fuzzy` | 变体绕过防护 | 允许跨越弱标点符号匹配 |
| 分词 | `token` | 英文单词 | 要求拉丁/数字词边界 |
| 精确短语 | `exact_phrase` | 混合语言短语 | 尊重词边界，处理中英混排 |
| CJK 分词 | `cjk_token` | 短中文词（1-2 字） | 要求前后无相邻汉字 |

**自动推断逻辑：**
- 含空格/标点分隔符 → `exact_phrase`
- 中英混合 → `exact_phrase`
- 纯汉字 1-2 字 → `cjk_token`
- 纯汉字 3-4 字 → `exact_phrase`
- 纯汉字 5+ 字 → `contains`
- 纯拉丁/数字 → `token`
- 其他 → `contains`

---

## 4. 检查流程

### 4.1 主流程

```
Check(input) →
  1. 系统总开关检查（isKeywordFilterEnabled，实时读取）
  2. 加载运行时快照（5s TTL 缓存，含编译后的规则）
  3. 快照一致性检查（若快照中 systemEnabled=false 但实际已开启，强制刷新）
  4. 页面开关检查（config.Enabled）
  5. 分组范围检查（includesGroup）
  6. 提取文本段（按协议解析请求体）
  7. 文本归一化（繁→简、全角→半角、小写化）
  8. AC 自动机扫描（关键词规则）
  9. 正则规则扫描（全文匹配，无截断）
  10. 白名单豁免检查
  11. 记录命中日志
  12. 返回拦截决策
```

### 4.2 文本提取

| 协议 | 提取路径 | 说明 |
|------|----------|------|
| Anthropic Messages | `messages[role=user].content` | 支持 string 和 content array |
| OpenAI Chat | `messages[role=user].content` | 支持 string 和 content array |
| OpenAI Responses | `input` | 支持 string、array、object |
| Gemini | `contents[role=user].parts[].text` | 支持空 role（默认 user） |
| 未知协议 | 尝试所有路径 | 兼容处理 |

**提取特性：**
- 只提取 `role=user` 的消息内容
- 每段文本记录 `messageIndex`、`partIndex` 用于精确定位
- 空文本自动跳过

### 4.3 文本归一化

1. **繁体→简体转换**（OpenCC t2s）
2. **全角→半角**（Unicode width folding）
3. **小写化**
4. **字符过滤**：
   - 关键词匹配文本（`Text`）：仅保留字母和数字
   - 正则匹配文本（`RegexText`）：保留所有字符，仅做宽度归一化
5. **位置映射**：维护归一化文本到原始文本的 span 映射

### 4.4 AC 自动机匹配

- **构建**：将所有启用的关键词规则归一化后构建 Aho-Corasick 自动机
- **扫描**：线性时间扫描全文，同一位置报告所有匹配（短词不被长词吞没）
- **匹配模式验证**：AC 命中后，根据规则的 `match_mode` 验证边界条件

### 4.5 正则匹配

- 使用 Go `regexp` 包（RE2 语义，保证线性时间）
- 对全文执行匹配，无长度限制
- 匹配位置通过 span 映射回原始文本

### 4.6 白名单豁免

白名单规则在关键词命中后执行：
1. 检查白名单模式是否覆盖命中位置
2. 全局白名单（`target_rule_ids` 为空）：豁免所有关键词规则的命中
3. 定向白名单：仅豁免指定 `target_rule_ids` 中的规则命中

### 4.7 WebSocket 跨帧检测

```
帧1: "敏感"  → 单帧检查（未命中）→ 加入窗口
帧2: "词"    → 单帧检查（未命中）→ 加入窗口 → 窗口合并检查 → 命中 "敏感词"
```

- 滑动窗口大小：512 rune
- 每帧先独立检查，再与历史窗口合并检查
- 合成请求体使用 `json.Marshal` 确保 JSON 合法性

---

## 5. 拦截响应

命中时返回：
```json
{
  "error": {
    "message": "输入内容命中关键词过滤规则，请调整后重试",
    "type": "keyword_filter_blocked",
    "code": "keyword_filter_blocked"
  }
}
```

HTTP 状态码由 `block_status` 配置决定（默认 403）。

---

## 6. 日志系统

### 6.1 记录字段

| 字段 | 说明 |
|------|------|
| `request_id` | 请求唯一标识 |
| `user_id` / `user_email` | 触发用户 |
| `api_key_id` / `api_key_name` | 使用的 API Key |
| `group_id` / `group_name` | 所属分组 |
| `endpoint` | 请求端点 |
| `provider` / `model` | 上游提供商和模型 |
| `protocol` | 协议类型 |
| `match_type` | 命中类型（keyword / regex） |
| `rule_name` | 命中的规则名称 |
| `matched_text` | 命中的文本片段 |
| `input_excerpt` | 输入摘要（最长 240 rune） |
| `input_hash` | 输入文本 SHA256 哈希 |
| `action` | 执行动作（block） |
| `block_status` | 拦截状态码 |
| `created_at` | 记录时间 |

### 6.2 日志搜索

支持 trigram 索引加速的 ILIKE 搜索，覆盖字段：
- `user_email`、`api_key_name`、`model`、`rule_name`、`input_excerpt`、`request_id`

### 6.3 自动清理

- 每 24 小时执行一次
- 分批删除（每批 5000 条）
- 按 `hit_retention_days` 配置的保留期清理
- 尊重 context 取消，超时 30 分钟

---

## 7. 管理后台

### 7.1 配置管理

- 全局开关、拦截状态码、拦截消息、保留天数
- 分组范围选择（全部 / 指定分组）

### 7.2 规则管理

**关键词规则表：**
- 新增、编辑、删除、启用/禁用
- 分页、搜索、按匹配模式筛选、按启用状态筛选
- 批量导入（CSV / JSON）、导出

**白名单规则表：**
- 同关键词规则 + 目标规则多选
- "全部规则"选项与多选互斥

**正则规则：**
- 名称 + 正则表达式
- 内置规则（手机号、URL）不可删除

### 7.3 测试功能

- 输入测试文本
- 基于当前（未保存）配置实时测试
- 返回：是否命中、规则 ID、匹配类型、解析后的匹配模式、命中文本、段落信息
- 测试端点与保存端点使用相同校验逻辑

### 7.4 导入导出

**CSV 格式：**
```csv
pattern,type,match_mode,enabled,target_patterns
敏感词,keyword,auto,true,
白名单词,whitelist,auto,true,敏感词
```

**JSON 格式：**
```json
{
  "keyword_rules": [{"pattern": "敏感词", "match_mode": "auto", "enabled": true}],
  "whitelist_rules": [{"pattern": "白名单词", "target_rule_ids": ["rule_id"]}]
}
```

无 `type` 字段时按导入按钮上下文（关键词/白名单）推断。

### 7.5 日志查看

- 筛选：匹配类型、分组、端点、搜索关键词、时间范围
- 分页浏览
- 请求竞态保护（sequence counter）
- 页面卸载时自动取消请求

---

## 8. 运行时行为

### 8.1 缓存策略

| 缓存项 | TTL | 失效条件 |
|--------|-----|----------|
| 运行时快照（规则编译结果） | 5 秒 | TTL 过期 / UpdateConfig 调用 |
| 系统总开关 | 无缓存 | 每次 Check 实时读取 |
| 规则哈希 | 随快照 | 规则变更时重新编译 |

### 8.2 并发安全

- `configMu` 互斥锁保护配置更新
- `atomic.Value` 存储运行时快照（无锁读取）
- `singleflight.Group` 防止缓存击穿
- WebSocket 会话使用 `sync.Mutex` 保护滑动窗口

### 8.3 性能特性

- AC 自动机：O(n) 线性扫描，n = 文本长度
- 正则匹配：RE2 保证线性时间，无回溯风险
- 规则编译结果缓存，仅在规则哈希变化时重新编译
- 日志清理分批执行，不长时间锁表

---

## 9. 数据库 Migration

| 编号 | 文件 | 内容 |
|------|------|------|
| 137 | `137_keyword_filter.sql` | 创建 `keyword_filter_logs` 表和基础索引 |
| 139 | `139_keyword_filter_logs_trgm_indexes.sql` | Best-effort 创建 pg_trgm 扩展 |
| 140 | `140_keyword_filter_logs_trgm_indexes_notx.sql` | CONCURRENTLY 创建 trigram 索引 |

---

## 10. 系统限制

| 限制项 | 值 |
|--------|-----|
| 单类型最大规则数 | 1000 |
| 关键词/白名单最大长度 | 256 rune |
| 正则表达式最大长度 | 512 rune |
| 日志摘要最大长度 | 240 rune |
| 日志最大保留天数 | 3650 天 |
| 拦截状态码范围 | 400-599 |
| WebSocket 窗口大小 | 512 rune |
| 运行时缓存 TTL | 5 秒 |
| 清理批次大小 | 5000 条 |

---

## 11. 安全设计

1. **无用户可控跳过逻辑**：不基于消息内容前缀/包含判断跳过检测
2. **全协议覆盖**：所有支持的 AI 协议均经过关键词扫描
3. **WebSocket 防拆分**：滑动窗口防止跨帧绕过
4. **正则无截断**：RE2 保证安全，全文扫描无盲区
5. **输入哈希去重**：相同输入不重复记录日志
6. **配置校验前置**：非法值在 normalize 前被拒绝
