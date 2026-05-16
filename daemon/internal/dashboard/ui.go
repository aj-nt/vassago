// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

package dashboard

// dashboardHTML is the single-page dashboard UI. It fetches data from the
// JSON API endpoints and renders a live view of the daemon state.
const dashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Vassago Dashboard</title>
<style>
  :root {
    --bg: #0d1117;
    --surface: #161b22;
    --border: #30363d;
    --text: #c9d1d9;
    --muted: #8b949e;
    --accent: #58a6ff;
    --green: #3fb950;
    --orange: #d2991d;
    --red: #f85149;
  }
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body {
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Helvetica, Arial, sans-serif;
    background: var(--bg);
    color: var(--text);
    line-height: 1.5;
  }
  header {
    background: var(--surface);
    border-bottom: 1px solid var(--border);
    padding: 16px 24px;
    display: flex;
    justify-content: space-between;
    align-items: center;
  }
  header h1 { font-size: 20px; font-weight: 600; }
  header h1 span { color: var(--muted); font-weight: 400; }
  .stats-bar {
    display: flex; gap: 24px; padding: 16px 24px;
    background: var(--surface); border-bottom: 1px solid var(--border);
  }
  .stat {
    text-align: center;
  }
  .stat .num { font-size: 28px; font-weight: 700; color: var(--accent); }
  .stat .label { font-size: 12px; color: var(--muted); text-transform: uppercase; letter-spacing: 0.5px; }
  main { padding: 24px; max-width: 1200px; margin: 0 auto; }
  .section { margin-bottom: 32px; }
  .section h2 {
    font-size: 16px; font-weight: 600; color: var(--muted);
    margin-bottom: 12px; border-bottom: 1px solid var(--border); padding-bottom: 8px;
  }
  .search-box {
    display: flex; gap: 8px; margin-bottom: 16px;
  }
  .search-box input {
    flex: 1; padding: 8px 12px; background: var(--surface); border: 1px solid var(--border);
    border-radius: 6px; color: var(--text); font-size: 14px;
  }
  .search-box input:focus { outline: none; border-color: var(--accent); }
  .search-box button {
    padding: 8px 16px; background: var(--accent); border: none; border-radius: 6px;
    color: #fff; font-size: 14px; cursor: pointer; font-weight: 600;
  }
  .category-bar {
    display: flex; flex-wrap: wrap; gap: 12px; margin-bottom: 16px;
  }
  .category-item {
    background: var(--surface); border: 1px solid var(--border); border-radius: 6px;
    padding: 12px 16px; display: flex; flex-direction: column; align-items: center; min-width: 100px;
  }
  .category-item .cat-name { font-size: 13px; color: var(--muted); text-transform: uppercase; letter-spacing: 0.5px; }
  .category-item .cat-count { font-size: 24px; font-weight: 700; color: var(--accent); }
  table { width: 100%; border-collapse: collapse; }
  th, td { padding: 10px 12px; text-align: left; border-bottom: 1px solid var(--border); }
  th { color: var(--muted); font-size: 12px; text-transform: uppercase; letter-spacing: 0.5px; font-weight: 600; }
  td { font-size: 14px; }
  .badge {
    display: inline-block; padding: 2px 8px; border-radius: 12px; font-size: 11px; font-weight: 600;
  }
  .badge-memory { background: rgba(88,166,255,0.15); color: var(--accent); }
  .badge-user { background: rgba(63,185,80,0.15); color: var(--green); }
  .badge-priority { background: rgba(210,153,29,0.15); color: var(--orange); }
  .hot-block {
    background: var(--surface); border: 1px solid var(--border); border-radius: 8px;
    padding: 16px; max-height: 320px; overflow-y: auto; font-size: 13px;
    font-family: 'SF Mono', 'Fira Code', monospace; white-space: pre-wrap; line-height: 1.6;
  }
  .empty { color: var(--muted); font-style: italic; padding: 24px; text-align: center; }
  .footer {
    text-align: center; padding: 24px; color: var(--muted); font-size: 12px;
    border-top: 1px solid var(--border); margin-top: 32px;
  }
  .tab-bar { display: flex; gap: 0; margin-bottom: 0; }
  .tab {
    padding: 8px 16px; background: var(--surface); border: 1px solid var(--border);
    border-bottom: none; border-radius: 6px 6px 0 0; cursor: pointer; font-size: 13px; color: var(--muted);
  }
  .tab.active { background: var(--bg); color: var(--text); border-bottom: 1px solid var(--bg); margin-bottom: -1px; }
  .table-wrap { border: 1px solid var(--border); border-radius: 0 0 8px 8px; overflow: hidden; }
</style>
</head>
<body>
<header>
  <h1>⚕ Vassago <span>Dashboard</span></h1>
  <div id="uptime" style="color:var(--muted);font-size:13px;"></div>
</header>
<div class="stats-bar">
  <div class="stat"><div class="num" id="totalMem">—</div><div class="label">Total</div></div>
  <div class="stat"><div class="num" id="memCount">—</div><div class="label">Memory</div></div>
  <div class="stat"><div class="num" id="userCount">—</div><div class="label">User</div></div>
  <div class="stat"><div class="num" id="agentCount">—</div><div class="label">Agents</div></div>
  <div class="stat"><div class="num" id="sessionCount">—</div><div class="label">Sessions</div></div>
  <div class="stat"><div class="num" style="color:var(--green)" id="schemaVer">v35</div><div class="label">Schema</div></div>
</div>

<main>
  <!-- Search -->
  <div class="section">
    <h2>🔍 Search</h2>
    <div class="search-box">
      <input id="searchInput" placeholder="Search memories (FTS5)..." onkeydown="if(event.key==='Enter')doSearch()">
      <button onclick="doSearch()">Search</button>
    </div>
    <div id="searchResults"></div>
  </div>

  <!-- Memory Browser -->
  <div class="section">
    <h2>📋 Memories</h2>
    <div class="tab-bar">
      <div class="tab active" onclick="switchTab('memory',this)">Memory</div>
      <div class="tab" onclick="switchTab('user',this)">User</div>
    </div>
    <div class="table-wrap" id="memList"></div>
  </div>

  <!-- Categories -->
  <div class="section">
    <h2>📊 By Category</h2>
    <div class="category-bar" id="categories"></div>
  </div>

  <!-- Hot Block -->
  <div class="section">
    <h2>🔥 Hot Block <span style="font-weight:400;font-size:12px;color:var(--muted)">(what agents see)</span></h2>
    <div class="hot-block" id="hotBlock">Loading...</div>
  </div>

  <div class="footer">
    Vassago Dashboard · Auto-refreshes every 10s · <span id="dbPath"></span>
  </div>
</main>

<script>
let currentTab = 'memory';

async function fetchStats() {
  const r = await fetch('/api/stats');
  const d = await r.json();
  document.getElementById('totalMem').textContent = d.total_memories;
  document.getElementById('memCount').textContent = d.memory_count;
  document.getElementById('userCount').textContent = d.user_count;
  document.getElementById('agentCount').textContent = d.agent_count;
  document.getElementById('sessionCount').textContent = d.session_count;
  document.getElementById('schemaVer').textContent = 'v' + d.schema_version;
  document.getElementById('uptime').textContent = 'up ' + d.server_uptime;
  document.getElementById('dbPath').textContent = d.db_path;

  // Category breakdown
  const cats = document.getElementById('categories');
  cats.innerHTML = Object.entries(d.by_category).map(([k,v]) =>
    '<div class="category-item"><span class="cat-name">'+k+'</span><span class="cat-count">'+v+'</span></div>'
  ).join('');

  // Hot block
  const hb = await fetch('/api/hot-block?target=' + currentTab);
  const hd = await hb.json();
  document.getElementById('hotBlock').textContent = hd.content || '(empty)';
}

async function doSearch() {
  const q = document.getElementById('searchInput').value.trim();
  if (!q) return;
  const r = await fetch('/api/search?q=' + encodeURIComponent(q));
  const d = await r.json();
  const el = document.getElementById('searchResults');
  if (!d.length) { el.innerHTML = '<div class="empty">No results for "'+q+'"</div>'; return; }
  el.innerHTML = '<table><tr><th>Key</th><th>Content</th><th>Priority</th></tr>' +
    d.map(e =>
      '<tr><td><span class="badge badge-'+e.target+'">'+e.target+'/'+e.key+'</span></td>'+
      '<td>'+escapeHTML(e.content).substring(0,120)+'</td>'+
      '<td><span class="badge badge-priority">P'+e.priority+'</span></td></tr>'
    ).join('') + '</table>';
}

async function fetchMemories() {
  const r = await fetch('/api/memories?target=' + currentTab);
  const d = await r.json();
  const el = document.getElementById('memList');
  if (!d.length) { el.innerHTML = '<div class="empty">No '+currentTab+' memories yet.</div>'; return; }
  el.innerHTML = '<table><tr><th>Category</th><th>Key</th><th>Content</th><th>Priority</th></tr>' +
    d.map(e =>
      '<tr><td style="color:var(--muted);font-size:12px">'+e.category+'</td>'+
      '<td style="font-weight:600">'+e.key+'</td>'+
      '<td>'+escapeHTML(e.content).substring(0,100)+'</td>'+
      '<td><span class="badge badge-priority">P'+e.priority+'</span></td></tr>'
    ).join('') + '</table>';
}

function switchTab(tab, el) {
  currentTab = tab;
  document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
  el.classList.add('active');
  fetchMemories();
  fetchStats();
}

function escapeHTML(s) {
  const d = document.createElement('div');
  d.textContent = s;
  return d.innerHTML;
}

fetchStats();
fetchMemories();
setInterval(fetchStats, 10000);
</script>
</body>
</html>`
