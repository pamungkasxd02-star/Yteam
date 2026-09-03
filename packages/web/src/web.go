package web

import (
	"net/http"
)

// IndexHTML contains a lightweight standalone web frontend for Yteam.
const IndexHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>YTEAM — OpenCode Web</title>
  <style>
    :root { --bg: #0d1117; --fg: #c9d1d9; --accent: #58a6ff; --card: #161b22; --border: #30363d; }
    body { background: var(--bg); color: var(--fg); font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; margin: 0; padding: 20px; display: flex; flex-direction: column; height: calc(100vh - 40px); }
    header { border-bottom: 1px solid var(--border); padding-bottom: 10px; margin-bottom: 15px; display: flex; justify-content: space-between; align-items: center; }
    h1 { font-size: 1.25rem; margin: 0; color: var(--accent); }
    #chat { flex: 1; overflow-y: auto; display: flex; flex-direction: column; gap: 12px; }
    .msg { padding: 12px 16px; border-radius: 8px; border: 1px solid var(--border); background: var(--card); }
    .msg.user { border-color: var(--accent); }
    .role { font-weight: bold; font-size: 0.8rem; margin-bottom: 4px; color: var(--accent); }
    #prompt-form { display: flex; gap: 8px; margin-top: 15px; }
    input[type="text"] { flex: 1; padding: 10px 14px; border-radius: 6px; border: 1px solid var(--border); background: var(--card); color: var(--fg); outline: none; }
    button { padding: 10px 20px; background: var(--accent); color: #000; border: none; border-radius: 6px; font-weight: bold; cursor: pointer; }
  </style>
</head>
<body>
  <header>
    <h1>⚡ YTEAM Web Interface</h1>
    <div id="status">Connected</div>
  </header>
  <div id="chat">
    <div class="msg">
      <div class="role">System</div>
      <div>Yteam Go Runtime Active. Type a prompt below to interact.</div>
    </div>
  </div>
  <form id="prompt-form">
    <input type="text" id="prompt-input" placeholder="Type instructions or /help..." autofocus autocomplete="off" />
    <button type="submit">Send</button>
  </form>
</body>
</html>`

// Handler serves static web frontend resources.
func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(IndexHTML))
	})
}
