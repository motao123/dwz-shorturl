# 备份与恢复策略

适用两套部署形态：**Docker 一键部署**（`docker-compose.yml`，单库）与
**裸机/宝塔部署**（`deploy.sh`，双库，使用 `deploy/backup.sh`）。

## 一、备份对象清单

| 对象 | 位置（Docker 部署） | 丢失后果 |
|------|--------------------|----------|
| 业务数据库 | 卷 `dwz-shorturl_mysql_data`（容器 `dwz-mysql`） | **全部短链/会员/统计丢失**（P0） |
| 运行时配置 | 卷 `dwz-shorturl_app_config`（config.yaml/config.php，含 JWT 密钥） | 会话全失效、需重新初始化 |
| Redis 数据 | 卷 `dwz-shorturl_redis_data`（限流/黑名单/缓存） | 可再生，损失小 |
| 限流与 RUM 日志 | 容器内 `/var/www/dwz/logs/`（不持久化） | 仅统计数据损失 |
| 代码 | Git 仓库（CNB/GitHub/Gitee 三镜像） | 可从仓库恢复 |
| `.env` | 服务器 `/data/dwz/dwz-shorturl/.env`（**不在任何卷内**） | 数据库/管理员密码丢失=无法恢复数据库 |

> `.env` 与配置卷是恢复链的根：**二者必须异地备份**，只备数据库不够。

## 二、Docker 部署：备份操作

### 1. 数据库（每日，宝塔计划任务或 cron）

```bash
# 在宿主机执行；单库模式库名为 .env 的 DB_NAME（默认 dwz）
docker exec dwz-mysql sh -c 'exec mysqldump -udwz -p"$MYSQL_PASSWORD" \
  --single-transaction --quick dwz' | gzip > /data/backup/dwz-$(date +%F).sql.gz
```

建议 crontab（每日 03:00，保留 14 天）：

```cron
0 3 * * * docker exec dwz-mysql sh -c 'exec mysqldump -udwz -p"$MYSQL_PASSWORD" --single-transaction --quick dwz' | gzip > /data/backup/dwz-$(date +\%F).sql.gz && find /data/backup -name 'dwz-*.sql.gz' -mtime +14 -delete
```

### 2. 配置卷与 .env（每周 + 每次变更后立即）

```bash
tar czf /data/backup/app_config-$(date +%F).tar.gz \
  -C /var/lib/docker/volumes dwz-shorturl_app_config
cp /data/dwz/dwz-shorturl/.env /data/backup/.env-$(date +%F)
```

### 3. 异地

将 `/data/backup/` 同步到对象存储或另一台机器（rsync/rclone/宝塔计划任务均可）。
**本地备份与服务器同盘=没有备份。**

## 三、恢复演练（每季度一次）

1. 新环境装 Docker，克隆仓库，放置 `.env`（用备份件）；
2. `docker compose up -d mysql`，导入 dump：
   `gunzip -c dwz-YYYY-MM-DD.sql.gz | docker exec -i dwz-mysql mysql -udwz -p"$MYSQL_PASSWORD" dwz`
3. 恢复配置卷：`tar xzf app_config-*.tar.gz -C /var/lib/docker/volumes`
   （或让 init 容器重新生成——但 **JWT 密钥会变，所有会话与密码保护短链的解锁 cookie 失效**，故优先用备份卷）；
4. `docker compose up -d`，验证：`/health` 全绿、抽 3 条短链跳转正常、管理台可登录。

## 四、裸机/宝塔双库部署

使用 `deploy/backup.sh`（环境变量见脚本头注释），cron 建议：

```cron
0 3 * * * /www/server/dwz-admin/backup.sh >> /var/log/dwz-backup.log 2>&1
```

恢复：建库 → 导入两份 dump → 恢复 `config.php`/`config.yaml` → 重启 php-fpm 与 dwz-admin 服务。

## 五、RUM 与点击日志

`logs/rum.jsonl`（CWV 指标）与 click_logs 分区表属可再生/可丢弃数据：
RUM 文件随容器重建清空（接受）；click_logs 由 `cleanup_click_logs` 任务按保留期自动清理，
如需留存分析请在清理前导出：`docker exec dwz-mysql ... SELECT ... INTO OUTFILE` 或 mysqldump 单表。
