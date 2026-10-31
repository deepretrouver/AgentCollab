(() => {
  let locale = 'zh-CN';
  try { locale = localStorage.getItem('agentcollab.locale') === 'en' ? 'en' : 'zh-CN'; } catch {}
  const messages = {
    '正在检查': 'Checking status', '团队地址': 'Team address',
    '进入团队': 'Open team', '复制团队入口': 'Copy team address',
    '日常只需保持 Server 运行': 'Keep the Server running',
    '进入团队后，在目标群聊邀请协作者并发送一个链接。成员可以直接用浏览器参与讨论；仅在接入本机 Agent 时需要安装 Client，不需要安装 Server。关闭本窗口后 Server 会继续在后台运行。': 'Invite people from the target room with one link. Members can use a browser; install the Client only to connect local agents. Other members do not need the Server. Closing this window keeps the Server running in the background.',
    '高级管理与连接诊断': 'Administration and connection diagnostics',
    '本机管理地址': 'Local management address', '当前可用入口': 'Available addresses',
    '数据目录': 'Data directory', '监听端口': 'Listening port', '最近健康检查': 'Last health check',
    '账号与权限': 'Accounts and permissions', '客户端下载': 'Client downloads',
    '网页版备用': 'Open in browser', '重新启动': 'Restart', '运行日志': 'Logs',
    'Server 设置': 'Server settings', '退出并停止 Server': 'Quit and stop Server',
    '团队入口 · 自动适应当前网络': 'Team address · Automatic', '团队入口 · 固定': 'Team address · Fixed',
    '正在检测': 'Detecting', '等待首次检查': 'Waiting for the first check',
    'Server 正常运行': 'Server is running', 'Server 正在自动恢复': 'Server is recovering',
    '正在处理…': 'Working…', '操作失败，请打开运行日志查看原因': 'Action failed. Open the logs for details.',
    '无法读取 Server 状态': 'Unable to read Server status',
    '已打开 AgentCollab Client': 'AgentCollab Client opened',
    '已在浏览器打开网页版备用入口': 'Team opened in your browser',
    '已打开账号与权限页面': 'Accounts and permissions opened',
    '已打开客户端下载页面': 'Client downloads opened', '已打开数据目录': 'Data directory opened',
    '已打开运行日志': 'Logs opened', '已打开 Server 设置': 'Server settings opened',
    'Server 已重新启动': 'Server restarted', '正在停止 Server…': 'Stopping Server…',
    '无法停止 Server': 'Unable to stop Server', '团队地址已复制：': 'Team address copied: ',
  };
  const t = text => locale === 'en' ? messages[text] || text : text;
  const leaves = [...document.querySelectorAll('main *')].filter(element => !element.children.length && messages[element.textContent]);
  const original = leaves.map(element => [element, element.textContent]);
  const select = document.createElement('select');
  select.id = 'language'; select.setAttribute('aria-label', 'Language / 语言');
  for (const [value, label] of [['zh-CN', '简体中文'], ['en', 'English']]) {
    const option = document.createElement('option'); option.value = value; option.textContent = label; select.append(option);
  }
  select.value = locale;
  select.style.cssText = 'padding:8px;border:1px solid #d7dfeb;border-radius:9px;background:white;color:#34445c';
  document.querySelector('.top').append(select);
  function apply() {
    document.documentElement.lang = locale;
    for (const [element, text] of original) element.textContent = t(text);
  }
  select.addEventListener('change', () => {
    locale = select.value;
    try { localStorage.setItem('agentcollab.locale', locale); } catch {}
    apply();
    window.dispatchEvent(new Event('dashboard-language-change'));
  });
  window.dashboardLocale = { t, get locale() { return locale; } };
  apply();
})();
