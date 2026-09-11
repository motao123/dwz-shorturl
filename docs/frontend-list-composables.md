# 列表页通用 composable（useListQuery / useListData / useBatchAction）

> 来源：Issue #17（由 Issue #1 审计遗留项拆出）。属前端架构重构，**不引入功能变更**。

## 目标

管理台与会员端此前每个列表页各自实现「加载态 / 分页 / 筛选 / 搜索 / 错误处理 / 刷新 / 导出 / 批量操作」，
行为不一致且改一次契约要改多处。本目录把逻辑层抽成组合式函数，页面只保留「派生字段组装」与业务动作。

## 目录

| 文件 | 职责 |
| --- | --- |
| `frontend/src/composables/listResponses.ts` | 列表响应归一化（裸数组 / `{list,total}` 包裹） |
| `frontend/src/composables/useListQuery.ts` | 分页 + 排序 + 筛选 ↔ 请求参数、URL query 双向同步 |
| `frontend/src/composables/useListData.ts` | 加载 / 刷新 / 错误 / 空态 / 并发竞态防护 |
| `frontend/src/composables/useBatchAction.ts` | 批量选择 + 执行 + 结果提示（含公共库同步失败契约） |
| `frontend/src/composables/useRowAction.ts` | 单行操作（确认 / 执行 / 结果 / 刷新） |
| `frontend/src/composables/useExportCsv.ts` | CSV 导出（blob 下载 + URL 释放 + 提示） |
| `frontend/src/composables/useListPage.ts` | 列表页骨架：组合上述能力 + 统一的响应式分页器布局 |

单元测试位于 `frontend/src/composables/__tests__/`，`npm test` 运行。

## 使用方式

```ts
const page = useListPage<User>({
  perPage: 20,
  perPageOptions: [10, 20, 50],
  // 键名即接口 query 参数名；默认值同时决定 URL 回填时的类型（数字保持数字）
  filters: { keyword: '', status: '' },
  fetcher: (params) => listUsers(params),
  errorMessage: '加载用户列表失败'
})

const { rows, total, loading } = page
const keyword = page.filterRef<string>('keyword') // 可 v-model
```

模板侧：

```vue
<el-input v-model="keyword" @keyup.enter="page.search" @clear="page.search" />
<el-table v-loading="loading" :data="rows" @selection-change="page.setSelected" />
<el-pagination
  :current-page="page.page.value"
  :page-size="page.perPage.value"
  :total="total"
  :page-sizes="page.perPageOptions"
  :layout="page.pagerLayout.value"
  @current-change="page.handlePageChange"
  @size-change="page.handleSizeChange"
/>
```

## URL 持久化约定

所有接入页面统一使用下列参数名，**空值不写入 URL**：

| 参数 | 说明 |
| --- | --- |
| `page` | 页码，从 1 开始（非法值回退 1） |
| `per_page` | 每页条数（兼容读取历史别名 `page_size`；不在候选列表内则回退默认值） |
| 筛选字段 | 字段名与接口 query 参数名一致，例如 `keyword`、`status`、`user_id` |
| 日期范围 | 统一使用 `date_start` / `date_end`（`YYYY-MM-DD`），页面在 `buildParams` 中转成接口既有字段（如 `date_from`/`date_to`） |

行为：

- 变更筛选 → `page` 重置为 1；变更 `per_page` → `page` 重置为 1；翻页只改 `page`。
- 写回地址栏使用 `history.replaceState`（不堆历史记录）；`hydrateFromUrl()` 在页面初始化时把 URL 还原回状态。
- 类型还原：当筛选默认值为数字时，URL 中的值会还原为数字——否则 `el-select` 的 `:value="1"` 与字符串 `'1'` 不相等，回填后下拉框会显示空白。

## 并发竞态防护

快速翻页或连续切换筛选时，先发出的请求可能后返回并覆盖新数据（现网隐性缺陷）。`useListData`
用自增请求序号丢弃过期响应：过期响应不写数据、不弹错误提示、不改动 loading 状态。
组件卸载后仍在途的请求同样通过 `isStale()` 暴露给调用方判断。

> 该防护在「仅重构」范围内顺带修复，但在 PR 描述中单独标注，避免与行为变更混淆。

## 公共跳转库同步失败契约

后台本地操作成功、但公共库（`wjoy_log`）同步失败时，链接可能仍可通过 PHP 路径访问。
契约字段：`public_sync_failed` / `sync_failed_uids`，由 `useBatchAction` 统一提示：

- 单条操作（`notifySyncResult`）：Warning，6s，附 `warning` 文案；
- 批量操作（`notifyBatchSyncResult`）：Warning，8s，列出失败 UID（去重前最多展示接口返回值）。

后续修改该契约只需改 `useBatchAction`，各页面无需同步。

## 迁移进度

| 页面 | 状态 |
| --- | --- |
| `views/audit/AuditLogList.vue` | ✅ 已迁移（含日期范围持久化） |
| `views/users/UserList.vue` | ✅ 已迁移 |
| `views/short-urls/ShortUrlList.vue` | ✅ 已迁移（批量删除 / 导出 / 回收站 / 排序） |
| `member/views/DashboardView.vue` | ✅ 列表部分已迁移（查看模式：登录态就绪后手动加载） |
| 其余列表页（角色 / 域名 / API 密钥 / Webhook / 违规） | ⏳ 按批次跟进 |

新增列表页请直接使用 `useListPage`，不要新写分页 / 筛选 / 加载样板。
