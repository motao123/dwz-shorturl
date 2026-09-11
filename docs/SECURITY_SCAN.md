# 依赖风险与安全扫描

本文说明仓库的安全扫描体系：**哪些告警会在 PR 阶段阻断、哪些只是记录、存量噪音如何降级**。

## 一、为什么需要分层

仓库早期依赖长期停留旧版本（如 `golang.org/x/crypto 0.25.0`、`echarts 5.6.0`），
平台安全页因此累积了大量存量告警（曾出现漏洞 31 条 / 代码问题 10 条 / 敏感信息 7 条）。

这里有一个关键取舍：

- 若在 PR 阶段做**全量**门禁 → 任何一个 PR 都会被历史告警卡死，CI 变成摆设；
- 若完全不做门禁 → 新引入的漏洞会静默合入，安全页数字只增不减。

因此采用**分层**设计：

| 层 | 位置 | 职责 | 是否阻断 |
|---|---|---|---|
| 增量门禁 | `.cnb.yml`（SCA 插件） | 只拦本次 PR **新引入**的漏洞 / License 风险 | ✅ 阻断 |
| 仓库级扫描 | `.cnb/security/code_scan_config.yml` + `.scanignore` | 范围收敛，压低存量噪音 | ❌ 只记录 |
| 自动升级 | `.github/dependabot.yml` | 每周自动拉取 minor / patch 升级 | ❌ 产 PR |

## 二、PR 阶段：增量门禁

配置见仓库根目录 `.cnb.yml`，在 `pull_request` 事件下运行三个 SCA 插件：

```yaml
"**":
  pull_request:
    - name: dependency-risk-check
      stages:
        - name: 新增依赖漏洞扫描
          image: cnbcool/sca-vulnerability:latest
          settings:
            failOnSeverity: high
        - name: 新增 License 风险扫描
          image: cnbcool/sca-license:latest
          settings:
            failOnSeverity: high
        - name: 生成 SBOM Delta
          image: cnbcool/sca-sbom:latest
          settings:
            output: ${CNB_BUILD_WORKSPACE}/.cnb-sca/sbom-delta.json
```

### 增量语义（重点）

插件会分别扫描 PR 的 base 与 head，**只报告两者之间的差集**：

- base 已存在的同一组件 + 同一 CVE → **不阻断**（存量告警不会卡住历史 PR）；
- head 新增的组件 / 新命中的 CVE → **阻断**；
- 删除依赖、无内容变化的文件重命名 → 不产生新增告警。

也就是说：**只有当前这次改动真正引入的风险才会让 CI 变红。**

### 门禁等级

| 等级 | 是否阻断 |
|---|---|
| `critical` | ✅ |
| `high` | ✅ |
| `medium` | ❌ 仅输出日志 |
| `low` | ❌ 仅输出日志 |

`failOnSeverity: high` 表示 High 及以上阻断。若希望更宽松，改为 `critical`；
若只想观察不阻断，删除 `failOnSeverity` 即可（插件仍输出结果，退出码始终为 0）。

### 退出码

| 退出码 | 含义 |
|---:|---|
| `0` | 扫描完成，新增风险未命中门禁 |
| `1` | 新增风险命中门禁（CI 失败） |
| `2` | 配置错误或扫描失败 |

## 三、仓库级：范围收敛

### `.scanignore`

gitignore 风格的路径规则，默认对 `secrets` 与 `software-composition-analysis` 两类能力生效。
当前排除：`vendor/`、`node_modules/`、构建产物、测试装置、示例配置、文档与静态站。

其中「示例配置」一类是**明确已知的误报**：`config.sample.php`、`config.example.yaml`
里的 `your_password` / `change-me-...` 是占位符，不是真实密钥（真实配置由 `.gitignore` 排除）。
把它们排除掉，安全页才能反映真实风险。

### `.cnb/security/code_scan_config.yml`

仅在公共规则之外做能力级补充，**不关闭任何能力**。

## 四、存量告警的处理原则

安全页的历史数字不会因为修了代码立刻刷新 —— 平台扫描基于 commit 的定时任务，
需等下一次扫描。判断某条告警是否已修复，请看它的 `revision` 是否早于修复提交。

对当前存量项的分类结论：

| 类型 | 数量 | 结论 | 处置 |
|---|---|---|---|
| 间接依赖版本区间命中（`x/net`、`x/text` 等 `// indirect`） | 多数 | 非直接调用，实际可达路径极少 | 由 Dependabot 逐步升掉 |
| 示例配置占位符 | 7 | 误报，无真实密钥 | 已加入 `.scanignore` |
| `GO-SQLI-001` 分区名拼接 | 1 | 无法参数化标识符，已加白名单校验 `^p\d{6}$` | 防御性兜底已就位 |

## 五、本地自查

提交依赖变更前，可先本地跑一遍，避免 CI 往返：

```bash
# Go
cd backend && go mod tidy && go build ./... && go vet ./...

# 前端
cd frontend && npm audit && npm run build && npm test
```

`npm audit` 应为 0 vulnerabilities；若非 0，说明引入了带已知漏洞的版本，
CI 的增量门禁同样会拦住。

## 参考

- [CNB 安全扫描插件总览](https://docs.cnb.cool/zh/security/sca-plugins.md)
- [开源组件漏洞扫描](https://docs.cnb.cool/zh/security/sca-vulnerability.md)
- [License 扫描](https://docs.cnb.cool/zh/security/sca-license.md)
- [SBOM 生成](https://docs.cnb.cool/zh/security/sca-sbom.md)
- [扫描配置文件语法](https://docs.cnb.cool/zh/security/syntax-reference.md)
