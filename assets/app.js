(function () {
  'use strict';

  var MAX_BATCH_ITEMS = 100;
  var REQUEST_TIMEOUT = 20000;
  var statusTimer = null;
  var lastFocusedElement = null;

  var singleForm = document.getElementById('shorten-form');
  var batchForm = document.getElementById('batch-form');
  var urlInput = document.getElementById('inputContent');
  var customInput = document.getElementById('customCode');
  var expireSelect = document.getElementById('expireDays');
  var singleButton = document.getElementById('shortify');
  var batchInput = document.getElementById('batchInput');
  var batchButton = document.getElementById('batchBtn');
  var batchCount = document.getElementById('batch-count');
  var batchResults = document.getElementById('batchResult');
  var batchList = document.getElementById('batch-list');
  var batchSummary = document.getElementById('batch-summary');
  var statusBox = document.getElementById('status');
  var domainSelect = document.getElementById('domainSelect');
  var domainField = document.getElementById('domain-field');
  var dialogBackdrop = document.getElementById('result-wrap');
  var dialog = dialogBackdrop.querySelector('[role="dialog"]');
  var dialogClose = document.getElementById('dialog-close');
  var resultInput = document.getElementById('gen_result_url');
  var copyButton = document.getElementById('copyBtn');
  var qrButton = document.getElementById('qrBtn');
  var qrBox = document.getElementById('qrcode');
  var previewButton = document.getElementById('preViewBtn');
  var againButton = document.getElementById('againBtn');

  // 会员 / 登录相关
  var memberBar = document.getElementById('memberBar');
  var memberLoginBtn = document.getElementById('memberLoginBtn');
  var memberUser = document.getElementById('memberUser');
  var memberLogoutBtn = document.getElementById('memberLogoutBtn');

  var memberState = { loggedIn: false, csrf: '' };

  function endpoint(file) {
    return new URL(file, document.baseURI).toString();
  }

  function setBusy(button, busy, busyText, idleText) {
    button.disabled = busy;
    button.setAttribute('aria-busy', busy ? 'true' : 'false');
    button.firstElementChild && button.firstElementChild !== button.lastElementChild
      ? button.firstElementChild.textContent = busy ? busyText : idleText
      : button.textContent = busy ? busyText : idleText;
  }

  function announce(message, isError) {
    window.clearTimeout(statusTimer);
    statusBox.textContent = message;
    statusBox.classList.toggle('is-error', Boolean(isError));
    statusBox.hidden = false;
    statusBox.setAttribute('role', isError ? 'alert' : 'status');
    if (!isError) {
      statusTimer = window.setTimeout(function () {
        statusBox.hidden = true;
      }, 3000);
    }
  }

  function normalizeUrl(value) {
    var trimmed = value.trim();
    if (!trimmed) return '';
    if (!/^https?:\/\//i.test(trimmed)) trimmed = 'https://' + trimmed;
    try {
      var parsed = new URL(trimmed);
      return /^https?:$/.test(parsed.protocol) && parsed.hostname ? parsed.toString() : '';
    } catch (error) {
      return '';
    }
  }

  function readError(payload, fallback) {
    if (payload && typeof payload === 'object') {
      return payload.msg || payload.message || (payload.error && (payload.error.message || payload.error)) || fallback;
    }
    return typeof payload === 'string' && payload.trim() ? payload.trim() : fallback;
  }

  function isSuccess(payload) {
    if (!payload || typeof payload !== 'object') return false;
    if (payload.success === true || payload.ok === true) return true;
    if (payload.result === 1 || payload.result === '1') return true;
    return Boolean(payload.code && payload.code !== 0 && payload.code !== '0' && !payload.error);
  }

  function extractShortUrl(payload) {
    if (!payload || typeof payload !== 'object') return '';
    var direct = payload.short_url || payload.shortUrl || payload.url_short || payload.short || (payload.data && (payload.data.short_url || payload.data.shortUrl || payload.data.url));
    if (typeof direct === 'string' && direct.trim()) {
      try { return new URL(direct, document.baseURI).toString(); } catch (error) { return direct; }
    }
    var code = payload.code || payload.short_code || payload.shortCode || (payload.data && (payload.data.code || payload.data.short_code));
    if (code && code !== '0') return new URL(encodeURIComponent(String(code)), document.baseURI).toString();
    return '';
  }

  async function postForm(file, params) {
    var controller = typeof AbortController === 'function' ? new AbortController() : null;
    var timeout = controller ? window.setTimeout(function () { controller.abort(); }, REQUEST_TIMEOUT) : null;
    var response;
    try {
      response = await fetch(endpoint(file), {
        method: 'POST',
        headers: { 'Content-Type': 'application/x-www-form-urlencoded;charset=UTF-8' },
        body: new URLSearchParams(params).toString(),
        credentials: 'same-origin',
        signal: controller ? controller.signal : undefined
      });
    } catch (error) {
      if (error && error.name === 'AbortError') throw new Error('请求超时，请稍后重试');
      throw new Error('网络连接失败，请检查网络后重试');
    } finally {
      if (timeout) window.clearTimeout(timeout);
    }

    var text = await response.text();
    var payload = null;
    try {
      payload = text ? JSON.parse(text) : null;
    } catch (error) {
      if (response.ok) throw new Error('服务返回了无法识别的数据');
    }

    if (!response.ok) {
      var fallback = response.status === 429 ? '请求过于频繁，请稍后再试' : '请求失败（' + response.status + '）';
      throw new Error(readError(payload || text, fallback));
    }
    return payload;
  }

  function validateSingleForm() {
    var normalized = normalizeUrl(urlInput.value);
    urlInput.setAttribute('aria-invalid', normalized ? 'false' : 'true');
    if (!normalized) {
      announce('请输入有效的 http 或 https 网址', true);
      urlInput.focus();
      return null;
    }

    var custom = customInput.value.trim();
    var customValid = custom === '' || /^[a-z0-5]{6,8}$/.test(custom);
    customInput.setAttribute('aria-invalid', customValid ? 'false' : 'true');
    if (!customValid) {
      announce('自定义短码需为 6–8 位小写字母或数字 0–5', true);
      customInput.focus();
      return null;
    }

    urlInput.value = normalized;
    var domain = domainSelect.value || '';
    return { url: normalized, custom: custom, expire: expireSelect.value, domain: domain };
  }

  function getFocusableElements() {
    return Array.prototype.slice.call(dialog.querySelectorAll('a[href], button:not([disabled]), input:not([disabled]), [tabindex]:not([tabindex="-1"])'));
  }

  function openDialog(shortUrl) {
    lastFocusedElement = document.activeElement;
    resultInput.value = shortUrl;
    previewButton.href = shortUrl;
    // 默认即渲染二维码（移动端扫码是核心诉求），省去一次点击
    qrBox.hidden = false;
    qrBox.replaceChildren();
    renderQr();
    qrButton.textContent = '隐藏二维码';
    qrButton.setAttribute('aria-expanded', 'true');
    dialogBackdrop.hidden = false;
    document.body.classList.add('dialog-open');
    dialogClose.focus();
  }

  function closeDialog() {
    if (dialogBackdrop.hidden) return;
    dialogBackdrop.hidden = true;
    document.body.classList.remove('dialog-open');
    qrBox.hidden = true;
    qrBox.replaceChildren();
    if (lastFocusedElement && typeof lastFocusedElement.focus === 'function') lastFocusedElement.focus();
  }

  async function copyText(text) {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text);
      return;
    }
    var helper = document.createElement('textarea');
    helper.value = text;
    helper.setAttribute('readonly', '');
    helper.style.position = 'fixed';
    helper.style.opacity = '0';
    document.body.appendChild(helper);
    helper.select();
    var copied = document.execCommand('copy');
    helper.remove();
    if (!copied) throw new Error('copy failed');
  }

  function showQrFallback(url) {
    var message = document.createElement('p');
    message.className = 'qr-fallback';
    message.textContent = '二维码暂时无法生成。你仍可复制或直接访问短链接：' + url;
    qrBox.appendChild(message);
  }

  function renderQr() {
    qrBox.replaceChildren();
    try {
      if (typeof window.QRCode !== 'function') throw new Error('QRCode unavailable');
      new window.QRCode(qrBox, { text: resultInput.value, width: 180, height: 180 });
    } catch (error) {
      showQrFallback(resultInput.value);
    }
  }

  function normalizeBatchPayload(payload) {
    if (Array.isArray(payload)) return payload;
    if (!payload || typeof payload !== 'object') return [];
    if (Array.isArray(payload.data)) return payload.data;
    if (Array.isArray(payload.results)) return payload.results;
    if (payload.data && Array.isArray(payload.data.results)) return payload.data.results;
    return [];
  }

  function createBatchItem(item) {
    var row = document.createElement('li');
    var source = document.createElement('div');
    var output = document.createElement('div');
    var shortUrl = extractShortUrl(item);
    var success = isSuccess(item) && Boolean(shortUrl);

    row.className = 'result-item' + (success ? '' : ' is-error');
    source.className = 'result-source';
    source.textContent = String(item.url || item.long_url || item.longUrl || '网址');
    source.title = source.textContent;
    output.className = 'result-output';

    if (success) {
      var link = document.createElement('a');
      var copy = document.createElement('button');
      link.className = 'result-link';
      link.href = shortUrl;
      link.target = '_blank';
      link.rel = 'noopener noreferrer';
      link.textContent = shortUrl;
      link.title = shortUrl;
      copy.className = 'mini-copy';
      copy.type = 'button';
      copy.textContent = '复制';
      copy.dataset.copy = shortUrl;
      copy.setAttribute('aria-label', '复制短链接 ' + shortUrl);
      output.append(link, copy);
    } else {
      var error = document.createElement('span');
      error.className = 'result-error';
      error.textContent = readError(item, '生成失败');
      output.appendChild(error);
    }

    row.append(source, output);
    return { element: row, success: success };
  }

  function renderBatch(items) {
    var bounded = items.slice(0, MAX_BATCH_ITEMS);
    var fragment = document.createDocumentFragment();
    var successCount = 0;
    batchList.replaceChildren();

    bounded.forEach(function (item) {
      var rendered = createBatchItem(item || {});
      if (rendered.success) successCount += 1;
      fragment.appendChild(rendered.element);
    });

    batchList.appendChild(fragment);
    batchSummary.textContent = successCount + ' 条成功，' + (bounded.length - successCount) + ' 条失败';
    batchResults.hidden = false;
    var reduceMotion = window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    batchResults.scrollIntoView({ behavior: reduceMotion ? 'auto' : 'smooth', block: 'nearest' });
  }

  function getBatchLines() {
    return batchInput.value.split(/\r?\n/).map(function (line) { return line.trim(); }).filter(Boolean);
  }

  function updateBatchCount() {
    var count = getBatchLines().length;
    batchCount.textContent = Math.min(count, MAX_BATCH_ITEMS) + ' / ' + MAX_BATCH_ITEMS;
    batchCount.style.color = count > MAX_BATCH_ITEMS ? 'var(--danger)' : '';
    batchInput.setAttribute('aria-invalid', count > MAX_BATCH_ITEMS ? 'true' : 'false');
  }

  singleForm.addEventListener('submit', async function (event) {
    event.preventDefault();
    var params = validateSingleForm();
    if (!params) return;

    setBusy(singleButton, true, '生成中…', '生成短链接');
    try {
      var payload = await postForm('api.php', params);
      if (!isSuccess(payload)) throw new Error(readError(payload, '短链接生成失败'));
      var shortUrl = extractShortUrl(payload);
      if (!shortUrl) throw new Error('服务未返回有效的短链接');
      openDialog(shortUrl);
    } catch (error) {
      announce(error.message || '短链接生成失败，请稍后重试', true);
    } finally {
      setBusy(singleButton, false, '生成中…', '生成短链接');
    }
  });

  batchForm.addEventListener('submit', async function (event) {
    event.preventDefault();
    if (!memberState.loggedIn) {
      announce('批量生成需要登录，正在跳转…', true);
      window.location.href = '/member/login?redirect=' + encodeURIComponent('/#batch');
      return;
    }
    var lines = getBatchLines();
    if (!lines.length) {
      batchInput.setAttribute('aria-invalid', 'true');
      announce('请至少输入一个网址', true);
      batchInput.focus();
      return;
    }
    if (lines.length > MAX_BATCH_ITEMS) {
      batchInput.setAttribute('aria-invalid', 'true');
      announce('一次最多处理 100 条网址，请删减后重试', true);
      batchInput.focus();
      return;
    }

    batchInput.setAttribute('aria-invalid', 'false');
    setBusy(batchButton, true, '生成中…', '批量生成');
    try {
      var payload = await postForm('batch.php', { urls: lines.join('\n'), domain: domainSelect.value || '', csrf: memberState.csrf || '' });
      var items = normalizeBatchPayload(payload);
      if (!items.length) throw new Error(readError(payload, '服务未返回批量结果'));
      renderBatch(items);
      announce('批量处理完成');
    } catch (error) {
      announce(error.message || '批量生成失败，请稍后重试', true);
    } finally {
      setBusy(batchButton, false, '生成中…', '批量生成');
    }
  });

  batchInput.addEventListener('input', updateBatchCount);
  urlInput.addEventListener('input', function () { urlInput.removeAttribute('aria-invalid'); });
  customInput.addEventListener('input', function () {
    customInput.value = customInput.value.toLowerCase().replace(/[^a-z0-5]/g, '').slice(0, 8);
    customInput.removeAttribute('aria-invalid');
  });

  dialogClose.addEventListener('click', closeDialog);
  dialogBackdrop.addEventListener('click', function (event) {
    if (event.target === dialogBackdrop) closeDialog();
  });

  dialogBackdrop.addEventListener('keydown', function (event) {
    if (event.key === 'Escape') {
      event.preventDefault();
      closeDialog();
      return;
    }
    if (event.key !== 'Tab') return;
    var focusable = getFocusableElements();
    if (!focusable.length) {
      event.preventDefault();
      dialog.focus();
      return;
    }
    var first = focusable[0];
    var last = focusable[focusable.length - 1];
    if (event.shiftKey && document.activeElement === first) {
      event.preventDefault();
      last.focus();
    } else if (!event.shiftKey && document.activeElement === last) {
      event.preventDefault();
      first.focus();
    }
  });

  copyButton.addEventListener('click', async function () {
    try {
      await copyText(resultInput.value);
      announce('短链接已复制');
      copyButton.textContent = '已复制';
      window.setTimeout(function () { copyButton.textContent = '复制'; }, 1600);
    } catch (error) {
      resultInput.focus();
      resultInput.select();
      announce('自动复制失败，已选中链接，请手动复制', true);
    }
  });

  qrButton.addEventListener('click', function () {
    var willShow = qrBox.hidden;
    if (willShow && !qrBox.childNodes.length) renderQr();
    qrBox.hidden = !willShow;
    qrButton.textContent = willShow ? '隐藏二维码' : '显示二维码';
    qrButton.setAttribute('aria-expanded', willShow ? 'true' : 'false');
  });

  // 「再生成一条」：关闭弹窗、清空并聚焦输入框，支持连续高频生成
  againButton.addEventListener('click', function () {
    closeDialog();
    urlInput.value = '';
    urlInput.removeAttribute('aria-invalid');
    urlInput.focus();
  });

  batchList.addEventListener('click', async function (event) {
    var button = event.target.closest('[data-copy]');
    if (!button || !batchList.contains(button)) return;
    try {
      await copyText(button.dataset.copy);
      announce('短链接已复制');
      button.textContent = '已复制';
      window.setTimeout(function () { button.textContent = '复制'; }, 1400);
    } catch (error) {
      announce('复制失败，请打开链接后手动复制', true);
    }
  });

  // ---------------- 会员登录 / 注册 ----------------

  function setMemberUI(member) {
    memberState.loggedIn = Boolean(member);
    if (member) {
      memberLoginBtn.hidden = true;
      memberLogoutBtn.hidden = false;
      memberUser.hidden = false;
      memberUser.textContent = (member.username || member.email || '') + ' ›';
    } else {
      memberLoginBtn.hidden = false;
      memberLogoutBtn.hidden = true;
      memberUser.hidden = true;
      memberUser.textContent = '';
    }
  }

  // 获取当前登录态 + CSRF token（用于批量接口）
  async function loadMemberState() {
    try {
      var response = await fetch(endpoint('member.php'), {
        method: 'POST',
        credentials: 'same-origin'
      });
      var payload = await response.json();
      if (payload && payload.result === 1 && payload.data) {
        memberState.csrf = payload.data.csrf || '';
        setMemberUI(payload.data.member);
      } else {
        setMemberUI(null);
      }
    } catch (e) {
      setMemberUI(null);
    }
  }

  // 登出后跳转到会员中心登录页（登录/注册统一收敛到 /member/login）
  async function handleLogout() {
    try {
      var body = new URLSearchParams();
      body.set('action', 'logout');
      body.set('csrf', memberState.csrf || '');
      await fetch(endpoint('member.php'), {
        method: 'POST',
        headers: { 'Content-Type': 'application/x-www-form-urlencoded;charset=UTF-8' },
        body: body.toString(),
        credentials: 'same-origin'
      });
    } catch (e) { /* 忽略 */ }
    window.location.href = '/member/login';
  }

  memberLogoutBtn.addEventListener('click', function (event) { event.preventDefault(); handleLogout(); });

  // 每个 fetch 的 POST 请求都带上会话 cookie（确保批量接口能识别登录态）
  updateBatchCount();

  // Load available domains for the domain selector
  async function loadDomains() {
    try {
      var response = await fetch(endpoint('admin/api/domains/active'), {
        method: 'GET',
        credentials: 'same-origin',
        signal: AbortSignal.timeout ? AbortSignal.timeout(5000) : undefined
      });
      if (!response.ok) return;
      var domains = await response.json();
      var list = Array.isArray(domains) ? domains : (domains && domains.data ? domains.data : []);
      if (!list.length) return;
      list.forEach(function (d) {
        var opt = document.createElement('option');
        opt.value = d.id;
        opt.textContent = (d.scheme || 'https') + '://' + d.domain;
        if (d.name) opt.textContent += ' (' + d.name + ')';
        domainSelect.appendChild(opt);
      });
      domainField.hidden = false;
      domainField.classList.add('fade-in');
    } catch (e) {
      // Domains API unavailable, keep selector hidden
    }
  }

  // 真实在线状态：请求 /health，失败时把头部状态点标记为「维护中」
  function checkHealth() {
    var note = document.querySelector('.header-note');
    if (!note) return;
    // 结构：<span class="status-dot"></span>在线 —— 文本节点是最后一个子节点
    var label = note.lastChild;
    fetch('./health', { method: 'GET', cache: 'no-store' })
      .then(function (r) { return r.ok; })
      .then(function (ok) {
        note.classList.toggle('is-down', !ok);
        if (label && label.nodeType === 3) label.textContent = ok ? '在线' : '维护中';
      })
      .catch(function () {
        note.classList.add('is-down');
        if (label && label.nodeType === 3) label.textContent = '维护中';
      });
  }

  loadDomains();
  loadMemberState();
  checkHealth();
  window.setInterval(checkHealth, 60000);
}());
