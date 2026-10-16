# 飞书监控告警

飞书与邮件独立启用。飞书不依赖 SMTP，现有邮件配置和 `email_sent` 状态保持不变。

## 配置

1. 在飞书群添加自定义机器人，在安全设置中**开启签名校验**，获取 Webhook 和签名密钥。
2. 在管理员「运维监控 → 设置 → 飞书告警」填写两项凭据，设置最低级别、每小时上限和可选的自动恢复通知。
3. 点击该区块的「保存飞书配置」（独立于弹窗底部的保存），再点击「发送测试消息」。测试使用已保存配置，可在通道尚未启用时执行。
4. 启用飞书告警，并在需要的告警规则中开启「发送飞书通知」。默认规则开关开启，全局飞书开关关闭，因此升级不会自动发送。

签名密钥必填。保存有效 Webhook 时必须同时具备密钥，发送入口也会再次校验，所有真实通知、恢复通知、测试和重试请求均携带新生成的时间戳与签名。

配置界面不回显凭据；留空表示保留，替换 Webhook 必须同时填写对应密钥。显式勾选清除并保存会同时删除两项凭据和关闭飞书。签名密钥可单独轮换。系统复用现有 `SecretEncryptor`（AES-256-GCM，使用 TOTP 加密密钥）加密存储两项凭据；部署迁移和备份时需保留原加密密钥。应用不能读取飞书机器人的安全设置，管理员仍须在飞书侧开启签名校验。

只接受 `https://open.feishu.cn/open-apis/bot/v2/hook/...`。发送客户端禁止重定向及私网地址，使用直连，不继承环境代理。服务器需可直连飞书并保持系统时钟准确。若同时启用机器人关键词校验，建议使用消息固定前缀 `Sub2API`。

## 投递行为

- 告警事件与飞书任务在同一事务中持久化；新表 `ops_feishu_deliveries` 在迁移 `246_ops_feishu_notifications.sql` 中创建。
- 工作进程独立于指标评估循环。多实例通过数据库行锁和 60 秒租约领取任务；过期任务可恢复，旧租约不能覆盖新结果。
- 发送前重新检查监控开关、通道和规则开关、级别及全局/范围静默。邮件开关不参与这些判断。
- 每小时上限默认 60 次（1–3600，可配置），统计发送尝试，包含失败、重试及测试。所有实例共享限额，每秒最多启动一次发送；测试还受 10 秒冷却限制。超限任务延后，不直接标记失败。
- 单次 HTTP 超时为 10 秒。网络错误、HTTP 429/5xx 和飞书限流码 `11232` 最多尝试 3 次，重试间隔 10 秒、60 秒。签名错误、请求格式错误等永久失败不重试。HTTP 200 仍需业务 `code=0` 才算成功。
- 同一事件的触发和恢复各最多入队一次。尚未发送的告警已恢复时取消触发任务；只有触发已成功投递，才发送自动恢复通知。人工解除不发恢复消息。恢复消息中的“触发值”是原始触发时的指标值。
- 替换 Webhook 会取消旧目标的待发送任务，避免旧消息被发到新的群。关闭通道后工作进程取消尚未发送的任务；已经发出的网络请求不能撤回。
- 网络超时或发送成功后进程崩溃可能导致消息重复；Webhook 不提供本系统可使用的幂等确认，不承诺严格 exactly-once。消息包含事件 ID，便于识别重复。
- 告警事件列表分别展示飞书触发/恢复的待发送、发送中、已发送、失败、取消状态；错误仅保存安全的 HTTP 状态或业务错误码，不保存上游原始响应和凭据。事件清理时投递记录通过外键级联删除。

## 渠道降智检测完成通知

- 在渠道监控中配置测试 Prompt 和预期答案，并开启「降智检测完成后通知飞书群」。同时须启用运维监控与飞书通知，并保存有效的 Webhook 和签名密钥。
- 手动和定时检测共用同一通知流程：每轮检测结束后，将主模型和附加模型的结果合并为一条消息；全部通过也会发送，不再仅在答案不符时通知。
- 答案匹配忽略首尾空白和外层加粗、行内代码格式。预期答案为纯数字时，也支持明确的数字结论（如 `最少取出 **21个**：` 后跟解释、`最终答案：21`、`The answer is 21.`）；正文仅提到数字不算通过，多个明确结论冲突也不通过。这是有限规则匹配而非语义评分，复杂回复建议在 Prompt 中要求只输出最终答案；上游输出截断仍判为检测未完成。
- 消息包含通过、答案不符、未完成的数量，以及最多 5 个模型的结果、回答、检测时间和错误详情。回答正确但响应较慢仍计为通过；请求失败、超时或输出截断表示无法判断是否降智，不计为答案不符。
- 复用运维飞书签名、凭据脱敏、安全传输和共享发送限额。每轮直接尝试发送一次，不进入告警事件重试队列；发送失败或限流不会改变检测结果，手动检测响应的 `notification_error` 返回安全提示，下轮检测完成后再次尝试。启用完成通知会增加发送量，请按检测频率与渠道数量配置限额。

## 管理 API

- `GET /api/v1/admin/ops/feishu-notification/config`：仅返回非敏感设置及 `webhook_configured`、`secret_configured`。
- `PUT /api/v1/admin/ops/feishu-notification/config`：保存非敏感设置；可选 `webhook_url`、`signing_secret`（省略保留）；`clear_credentials=true` 要求同时 `enabled=false` 且不传凭据字段。
- `POST /api/v1/admin/ops/feishu-notification/test`：测试已保存配置；不创建真实告警事件。

规则 API 新增 `notify_feishu`。老客户端更新时不传此字段会保留现值；创建时不传默认开启。

## 验证

```sh
cd backend
go test -tags=unit ./internal/service ./internal/repository ./internal/handler/admin ./cmd/server -run 'TestOps|TestValidateOps' -count=1
go test -tags=integration ./internal/repository -run '^TestOpsFeishuPostgresLifecycle$' -count=1
```

也可使用仅依赖 PostgreSQL 的窄范围测试入口：设置 `SUB2API_FEISHU_TEST_DSN` 指向**可丢弃的测试数据库**，运行 `go test -tags=feishu_integration ./internal/repository -run '^TestOpsFeishuPostgresLifecycle$' -count=1`。该入口会应用全量数据库迁移，不得指向生产数据库。

```sh
pnpm --dir frontend test:run src/views/admin/ops/components/__tests__ src/i18n/__tests__/localeKeyCompleteness.spec.ts
pnpm --dir frontend build
```

协议依据：[飞书自定义机器人官方指南](https://open.feishu.cn/document/client-docs/bot-v3/add-custom-bot)。签名使用 `timestamp + "\n" + secret` 作为 HMAC-SHA256 的 key，对空消息计算摘要，再做 Base64 编码。
