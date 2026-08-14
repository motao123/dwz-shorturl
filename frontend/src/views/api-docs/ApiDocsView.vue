<script setup lang="ts">
import { ref } from 'vue'
import { DocumentCopy, CopyDocument } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import { copyText } from '@/utils/clipboard'

const baseUrl = 'https://1.xk7.cn'
const activeTab = ref<'create' | 'errors'>('create')

const curlExample = `# 创建短链（需 API 密钥）
curl -X POST '${baseUrl}/public/api/short-urls' \\
  -H 'Content-Type: application/json' \\
  -H 'X-API-Key: dwz_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx' \\
  -d '{
    "url": "https://example.com/very/long/path",
    "custom": "",
    "expire_days": 0
  }'`

const responseExample = `{
  "code": 0,
  "msg": "success",
  "data": {
    "uid": "aB3x9",
    "short_url": "${baseUrl}/aB3x9",
    "long_url": "https://example.com/very/long/path",
    "expire_at": null,
    "created_at": "2026-08-03T14:00:00+08:00"
  }
}`

const curlBatchExample = `# 批量创建短链（每次最多 100 条）
curl -X POST '${baseUrl}/public/api/short-urls/batch' \\
  -H 'Content-Type: application/json' \\
  -H 'X-API-Key: dwz_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx' \\
  -d '{
    "urls": [
      "https://example.com/a",
      "https://example.com/b"
    ]
  }'`

const batchResponseExample = `{
  "code": 0,
  "msg": "success",
  "data": {
    "results": [
      { "uid": "aB3x9", "short_url": "${baseUrl}/aB3x9", "long_url": "https://example.com/a" },
      { "uid": "cD7k2", "short_url": "${baseUrl}/cD7k2", "long_url": "https://example.com/b" }
    ],
    "errors": [],
    "total": 2
  }
}`

const javaExample = `// Java (OkHttp)
OkHttpClient client = new OkHttpClient();
RequestBody body = new RequestBody.Builder()
    .setBody("{\\"url\\":\\"https://example.com/a\\"}")
    .build();
Request req = new Request.Builder()
    .url("${baseUrl}/public/api/short-urls")
    .header("X-API-Key", "dwz_xxx")
    .post(body)
    .build();`

const jsExample = `// JavaScript (fetch)
const res = await fetch('${baseUrl}/public/api/short-urls', {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json',
    'X-API-Key': 'dwz_xxx'
  },
  body: JSON.stringify({ url: 'https://example.com/a' })
});
const data = await res.json();
console.log(data.short_url); // 短链`

const pythonExample = `# Python (requests)
import requests
r = requests.post('${baseUrl}/public/api/short-urls',
    headers={'X-API-Key': 'dwz_xxx'},
    json={'url': 'https://example.com/a'})
print(r.json()['data']['short_url'])`

const errorCodes = [
  ['40100', '未提供或无效的 API 密钥'],
  ['40100', 'API 密钥已吊销或过期'],
  ['40000', '请求参数错误（如 url 缺失）'],
  ['42200', 'URL 格式非法或包含违规内容'],
  ['42900', '触发限流（每分钟配额用尽）']
]

async function copy(text: string) {
  try {
    await copyText(text)
    ElMessage.success('已复制')
  } catch {
    ElMessage.error('复制失败')
  }
}
</script>

<template>
  <div class="app-page">
    <div class="app-page__head">
      <div>
        <h1 class="app-page__title">
          API 文档
          <small>API DOCS · 开放平台接口</small>
        </h1>
        <p class="app-page__desc">通过 API 密钥调用公开接口，程序化创建短链</p>
      </div>
      <el-button :icon="DocumentCopy" @click="copy(curlExample)">复制示例</el-button>
    </div>

    <div class="docs-grid">
      <!-- 快速开始 -->
      <section class="app-card">
        <h3 class="card-title">快速开始</h3>
        <ol class="steps">
          <li>在「API 密钥」页创建密钥，复制一次性明文（仅显示一次）</li>
          <li>请求时在 <code>X-API-Key</code> 请求头携带密钥</li>
          <li>调用 <code>POST /public/api/short-urls</code> 创建短链</li>
        </ol>
      </section>

      <!-- 认证 -->
      <section class="app-card">
        <h3 class="card-title">认证方式</h3>
        <p class="desc">公开接口使用 API 密钥鉴权，通过请求头传递：</p>
        <pre class="code-block"><code>X-API-Key: dwz_xxxxxxxx...      # 推荐
Authorization: Bearer dwz_xxx    # 兼容</code></pre>
        <p class="desc hint">密钥在「API 密钥」页创建。密钥明文仅创建时展示一次，请妥善保管。</p>
      </section>

      <!-- 接口 -->
      <section class="app-card wide">
        <h3 class="card-title">接口：创建短链</h3>
        <pre class="code-block"><code>POST {{ baseUrl }}/public/api/short-urls</code></pre>

        <el-tabs v-model="activeTab">
          <el-tab-pane label="请求示例" name="create">
            <h4>cURL</h4>
            <div class="code-wrap">
              <pre class="code-block"><code>{{ curlExample }}</code></pre>
              <el-button class="copy-btn" :icon="CopyDocument" circle size="small" @click="copy(curlExample)" />
            </div>
            <h4>响应</h4>
            <pre class="code-block"><code>{{ responseExample }}</code></pre>
          </el-tab-pane>

          <el-tab-pane label="多语言示例" name="languages">
            <h4>JavaScript</h4>
            <pre class="code-block"><code>{{ jsExample }}</code></pre>
            <h4>Python</h4>
            <pre class="code-block"><code>{{ pythonExample }}</code></pre>
            <h4>Java</h4>
            <pre class="code-block"><code>{{ javaExample }}</code></pre>
          </el-tab-pane>

          <el-tab-pane label="请求参数" name="params">
            <el-table :data="[
              { name: 'url', type: 'string', required: '是', desc: '目标长网址，http/https，≤2048 字符' },
              { name: 'custom', type: 'string', required: '否', desc: '自定义短码，6-8 位 a-z0-5' },
              { name: 'expire_days', type: 'int', required: '否', desc: '有效期天数：0 / 1 / 7 / 30 / 365' },
              { name: 'domain_id', type: 'int', required: '否', desc: '指定域名 ID（可选）' }
            ]" size="small" stripe>
              <el-table-column prop="name" label="参数" width="120" />
              <el-table-column prop="type" label="类型" width="90" />
              <el-table-column prop="required" label="必填" width="70" />
              <el-table-column prop="desc" label="说明" />
            </el-table>
          </el-tab-pane>

          <el-tab-pane label="错误码" name="errors">
            <el-table :data="errorCodes" size="small" stripe>
              <el-table-column prop="0" label="Code" width="100" />
              <el-table-column prop="1" label="说明" />
            </el-table>
          </el-tab-pane>
        </el-tabs>
      </section>

      <!-- 批量创建 -->
      <section class="app-card wide">
        <h3 class="card-title">接口：批量创建短链</h3>
        <pre class="code-block"><code>POST {{ baseUrl }}/public/api/short-urls/batch</code></pre>
        <div class="code-wrap">
          <pre class="code-block"><code>{{ curlBatchExample }}</code></pre>
          <el-button class="copy-btn" :icon="CopyDocument" circle size="small" @click="copy(curlBatchExample)" />
        </div>
        <h4>响应</h4>
        <pre class="code-block"><code>{{ batchResponseExample }}</code></pre>
        <h4>请求参数</h4>
        <el-table :data="[
          { name: 'urls', type: 'string[]', required: '是', desc: '目标网址数组，每项 http/https，≤100 条' }
        ]" size="small" stripe>
          <el-table-column prop="name" label="参数" width="120" />
          <el-table-column prop="type" label="类型" width="110" />
          <el-table-column prop="required" label="必填" width="70" />
          <el-table-column prop="desc" label="说明" />
        </el-table>
        <p class="desc hint">响应中 <code>results</code> 为成功项，<code>errors</code> 为失败原因，逐条对应。</p>
      </section>
    </div>
  </div>
</template>

<style scoped>
.docs-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
  gap: 16px;
}

.docs-grid .wide {
  grid-column: 1 / -1;
}

.card-title {
  margin: 0 0 12px;
  font-size: 15px;
  font-weight: 700;
  color: var(--dwz-ink);
}

.steps {
  margin: 0;
  padding-left: 20px;
}

.steps li {
  margin-bottom: 8px;
  font-size: 13px;
  color: var(--dwz-text);
}

.desc {
  margin: 0 0 10px;
  font-size: 13px;
  color: var(--dwz-text);
}

.desc.hint {
  color: var(--dwz-text-dim);
  font-size: 12px;
}

.code-block {
  margin: 0 0 12px;
  padding: 12px 14px;
  background: #0f2328;
  color: #d7ecea;
  border-radius: 8px;
  font-family: "JetBrains Mono", "SF Mono", Consolas, monospace;
  font-size: 12.5px;
  line-height: 1.6;
  overflow-x: auto;
  white-space: pre;
}

.code-block code {
  font-family: inherit;
}

.code-wrap {
  position: relative;
}

.copy-btn {
  position: absolute;
  top: 8px;
  right: 8px;
}

h4 {
  margin: 14px 0 8px;
  font-size: 13px;
  color: var(--dwz-text-dim);
}
</style>