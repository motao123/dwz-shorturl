# click_logs 月度分区维护

`click_logs` 是 DWZ 的点击明细表，按 `TO_DAYS(created_at)` 做 RANGE 分区，每月一个
分区（`pYYYYMM`），外加一个兜底分区 `p_future (VALUES LESS THAN MAXVALUE)`。

分区不是"可选优化"：统计查询与过期清理都依赖分区裁剪。一旦某个月的分区缺失，
新数据会全部落进 `p_future`，`click_logs` 就静默退化成一张单一大表 —— 没有报错，
只是查询和清理越来越慢。因此分区维护本身就是可靠性功能，需要可观测、可告警、可自愈。

## 覆盖目标

始终保证"当前月 + 2 个月"已有独立分区，即覆盖目标为 `当前月 + partitionAheadMonths`。

```
当前月 = 2026-09  →  目标月 2026-11  →  必须存在 p202609 / p202610 / p202611
```

判断口径是"最远分区月 ≥ 目标月"，落后月数（`behind_months`）为 0 才算达标。

## 定时任务

| 项 | 值 |
|---|---|
| cron 任务名 | `ensure_partitions` |
| 调度 | 每日 03:15（`15 3 * * *`） |
| 行为 | 幂等补齐缺失月份，已存在月份不重复创建 |
| 失败重试 | 下个自然日的窗口自动重试 |

## 可观测性

### 监控接口

| 接口 | 权限 | 说明 |
|---|---|---|
| `GET /admin/api/monitor/partitions` | `stats:read` | 只读分区覆盖状态，可安全轮询（不执行 DDL） |
| `GET /admin/api/monitor` | `stats:read` | 状态内联在 `partitions` / `partition_health` 字段 |
| `POST /admin/api/monitor/ensure-partitions` | `stats:update` | 幂等补齐；`?months=N` 覆盖目标月，`?dry_run=true` 只预览 |
| `POST /admin/api/monitor/run-task` | `stats:update` | `{"name":"ensure_partitions"}` 走 cron 同一入口 |

`GET /monitor/partitions` 返回的核心字段：

| 字段 | 含义 |
|---|---|
| `available` | 表存在且按 RANGE 分区 |
| `months` | 已存在的月度分区（升序） |
| `earliest_month` / `latest_month` | 覆盖区间的首末月份 |
| `required_month` | 目标月（当前月 + 2） |
| `behind_months` | 落后目标月的月数，0 为达标 |
| `pending_months` | 下次补齐将创建的月份 |
| `future_rows` / `total_rows` / `future_ratio` | `p_future` 行数及其占全表比例 |
| `has_future` | 兜底分区是否存在（REORGANIZE 依赖它） |
| `mismatch` | 结构异常：`table_not_partitioned` / `future_partition_missing` |
| `consecutive_failures` | 连续失败次数 |
| `alert_level` | `ok` / `warning` / `critical` |
| `alerts` | 每条告警的可读原因 |

### 监控页

`系统监控 → 点击日志分区` 卡片展示覆盖区间、目标月、落后月数、`p_future` 行数占比、
最近维护时间与失败原因；覆盖不足时逐条列出告警，并提供「补齐分区」按钮（幂等）。

### 告警判定

| 级别 | 触发条件 |
|---|---|
| `critical` | 连续失败 ≥ 2 次；表未按 RANGE 分区；缺少 `p_future`；`p_future` 行数占比 ≥ 10%；状态查询失败；表不存在 |
| `warning` | 覆盖落后目标月；最近一次维护失败（尚未达连续阈值） |

日志约定：

- 成功且创建了分区 → `Info`（`count` / `months` / `coverage_until`）
- 成功且无需创建 → `Info`（"already up to date"）
- 失败 → `Error`，带月份、失败次数与原始错误
- 补齐结束后仍有告警 → `Warn`/`Error`（`ALERT`），并逐条打印原因

告警同时通过 webhook 外发（事件 `system.partition_alert`，恢复为 `system.partition_ok`），
可在「Webhook」页订阅。

## 为什么设置 `future_rows` / `future_ratio`

`p_future` 有行属于正常现象（月切换瞬间写入、回填历史数据）。判断健康与否看"比例是否
持续增长"：`future_ratio ≥ 10%` 说明月度分区已经明显跟不上写入了，此时指标会转红。

## 幂等语义与历史回补

补齐是严格幂等的：先读现有覆盖，只创建缺失月份，重复执行不产生任何 DDL，也不会出现
重名分区。

**不自动回补历史月份**，原因有两个：

1. `REORGANIZE PARTITION p_future` 只能拆分 `p_future` 里实际存在的行。为"创建第一个
   分区之前的月份"建分区，结果只会是一个空分区，并让 `information_schema` 谎报覆盖区间
   （看起来覆盖了，实际老数据还在别处）。
2. 若历史数据已经落在一个较老的分区里，要把它拆到更细的月度分区必须重建数据
   （导出 → 重建表 → 导入），不可能在 cron 里安全地做。

因此当"不存在月度分区"或"最远分区月早于当前月"时，补齐从**当前月**开始，向后创建到目标月
为止，保证从当前月起不存在空洞。老库若确有历史分区缺失，请按
`backend/migrations/schema.sql` 的分区定义走一次数据重建，不在本任务职责内。

## 生产注意事项

- **DDL 锁**：`ALTER TABLE ... REORGANIZE PARTITION` 需要在旧分区上持锁。`p_future`
  通常很小（它只承接未按月分区的行），所以锁窗口一般很短；但仍建议在低峰执行，
  并支持分步进行 —— 每个月的 ALTER 单独提交，中途失败不影响已创建的月份。
- **先预览再执行**：大表上建议先 `?dry_run=true` 看清将创建哪些月份，再实际执行。
- **MySQL 版本差异**：`REORGANIZE PARTITION` 语法与 `TO_DAYS()` 行为建议在测试库
  对 5.7 / 8.0 各验证一次。本项目使用 `TO_DAYS(?)` 绑定参数（分区名必须内联，值可参数化）。
- **索引不受影响**：维护只改分区，不动 `short_url_id` 索引与 `created_at` 查询路径。
- **删旧数据兜底分区增长**：`cleanup_click_logs` 依赖 `created_at < ?` 裁剪分区；
  若分区缺失导致数据堆积在 `p_future`，清理会退化为全表扫描。

## 排查步骤

```sql
-- 1. 当前分区与行数
SELECT PARTITION_NAME, PARTITION_METHOD, PARTITION_DESCRIPTION, TABLE_ROWS
  FROM information_schema.PARTITIONS
 WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'click_logs'
 ORDER BY PARTITION_ORDINAL_POSITION;

-- 2. 是否已有数据落进兜底分区
SELECT COUNT(*) FROM click_logs PARTITION (p_future);
```

```bash
# 3. 通过接口确认状态并补齐（stats:update 权限）
curl -s -H "Authorization: Bearer $TOKEN" .../monitor/partitions
curl -s -X POST -H "Authorization: Bearer $TOKEN" .../monitor/ensure-partitions?dry_run=true
curl -s -X POST -H "Authorization: Bearer $TOKEN" .../monitor/ensure-partitions
```

告警转红时的处置顺序：确认磁盘空间与 MySQL 负载 → 确认执行账号仍有 `ALTER` 权限 →
预览待建月份 → 补齐 → 复查 `p_future` 行数占比是否停止增长。

## 测试覆盖

`backend/internal/service/partition_test.go` 通过脚本化 MySQL driver 覆盖：

- 正常补建（含边界必须是次月 1 号、`p_future` 必须保留）
- `ALTER` 失败（部分成功保留、失败月份上报、连续失败进入 `critical`）
- 无月度分区（初始建立，从当前月起无空洞）
- 已覆盖时 no-op（不产生任何 DDL）
- 结构异常（未分区 / 缺 `p_future`）
- 表不存在 / 查询失败 / 未配置数据库均不 panic
- `partition_scenario_test.go` 端到端串起"健康 → 失败告警 → 补齐 → 幂等"

```bash
cd backend && go build ./... && go vet ./... && go test ./...
```
