(() => {
  const seen = new Set();
  let lastSignature = "";
  let requestInFlight = false;

  const normalize = value => (value || "").normalize("NFC").toLowerCase().trim().replace(/\s+/g, " ");
  const twoSyllable = value => /^\p{L}[\p{L}\p{M}'’-]*\s+\p{L}[\p{L}\p{M}'’-]*$/u.test(value);

  function currentWord() {
    const preferred = document.querySelector('[class*="featuredWord"], [class*="wordWrap"], [class*="wordSection"]');
    const preferredText = normalize(preferred?.innerText);
    return twoSyllable(preferredText) ? preferredText : "";
  }

  function panel() {
    let node = document.getElementById("noitu-helper-panel");
    if (node) return node;
    node = document.createElement("aside");
    node.id = "noitu-helper-panel";
    node.style.cssText = "position:fixed;right:18px;bottom:18px;z-index:2147483647;width:280px;max-height:42vh;overflow:auto;background:#102f2b;color:#fff;border:2px solid #7cf5c5;border-radius:14px;padding:12px;font:14px/1.4 system-ui,sans-serif;box-shadow:0 8px 32px #0008";
    document.body.appendChild(node);
    return node;
  }

  function hidePanel() {
    const node = document.getElementById("noitu-helper-panel");
    if (node) node.remove();
  }

  function render(state, suggestions) {
    const node = panel();
    node.textContent = "";
    const heading = document.createElement("div");
    heading.style.cssText = "font-weight:700;margin-bottom:8px";
    heading.textContent = `Gợi ý cho ${state.key}`;
    node.appendChild(heading);
    const list = document.createElement("div");
    suggestions.slice(0, 12).forEach(item => {
      const row = document.createElement("div");
      row.style.cssText = "display:flex;justify-content:space-between;gap:8px;padding:5px 0;border-bottom:1px solid #ffffff20";
      row.textContent = item.word;
      const badge = document.createElement("small");
      badge.textContent = item.verified ? "✓" : "~";
      badge.title = item.verified ? "Đã được ghi nhận hợp lệ" : "Ứng viên từ từ điển nền";
      badge.style.color = item.verified ? "#7cf5c5" : "#ffd86b";
      row.appendChild(badge);
      list.appendChild(row);
    });
    node.appendChild(list);
    const controls = document.createElement("div");
    controls.style.cssText = "display:flex;align-items:center;justify-content:space-between;gap:8px;margin-top:10px";
    const collectButton = document.createElement("button");
    collectButton.type = "button";
    collectButton.textContent = "Cào ngay";
    collectButton.style.cssText = "border:1px solid #7cf5c5;border-radius:8px;background:#164b43;color:#fff;padding:5px 9px;cursor:pointer";
    collectButton.addEventListener("click", async () => {
      await chrome.storage.local.set({collectorEnabled: true, collectorLastError: ""});
      await collectBrowserGame();
    });
    controls.appendChild(collectButton);
    const collectorStatus = document.createElement("small");
    collectorStatus.id = "noitu-collector-status";
    collectorStatus.style.cssText = "display:block;margin-top:6px;color:#b7e8d6";
    controls.appendChild(collectorStatus);
    node.appendChild(controls);
    updateCollectorStatus();
  }

  async function updateCollectorStatus() {
    const statusNode = document.getElementById("noitu-collector-status");
    if (!statusNode) return;
    const status = await chrome.storage.local.get({collectorEnabled: false, collectorRunning: false, collectorLastError: ""});
    statusNode.textContent = status.collectorLastError
      ? `Cron lỗi: ${status.collectorLastError}`
      : status.collectorEnabled
        ? (status.collectorRunning ? "Cron browser đang cào..." : "Cron browser đã bật")
        : "Cron browser đã tắt";
    statusNode.style.color = status.collectorLastError ? "#ffb4a8" : "#b7e8d6";
  }

  async function sync() {
    const word = currentWord();
    if (!word) return;
    seen.add(word);
    const key = word.split(" ").at(-1);
    const signature = `${word}|${key}|${[...seen].sort().join(",")}`;
    if (signature === lastSignature || requestInFlight) return;
    lastSignature = signature;
    requestInFlight = true;
    try {
      const response = await chrome.runtime.sendMessage({
        type: "suggest",
        payload: {current_word: word, key, words: [word], used: [...seen]}
      });
      if (response?.status === 204) {
        hidePanel();
        return;
      }
      if (!response || response.status < 200 || response.status >= 300) {
        throw new Error(response?.data?.message || `backend ${response?.status || 0}`);
      }
      render(response.data, response.data?.suggestions || []);
    } catch (error) {
      const node = panel();
      node.textContent = `Backend gợi ý chưa sẵn sàng: ${error.message}`;
    } finally {
      requestInFlight = false;
    }
  }

  let browserCollectorPromise = null;
  const collectorDefaults = {backendUrl: "http://127.0.0.1:8080", apiKey: "", collectorEnabled: false};

  async function collectorConfig() {
    return await chrome.storage.local.get(collectorDefaults);
  }

  async function collectorBackendJSON(path, options = {}) {
    return await withTimeout(chrome.runtime.sendMessage({
      type: "collectorBackend",
      path,
      options: {method: options.method || "GET", body: options.body}
    }), 15000, `backend ${path} timeout`);
  }

  function withTimeout(promise, milliseconds, message) {
    let timer;
    const timeout = new Promise((_, reject) => {
      timer = setTimeout(() => reject(new Error(message)), milliseconds);
    });
    return Promise.race([promise, timeout]).finally(() => clearTimeout(timer));
  }

  function tokenExpiresSoon(token) {
    try {
      const payload = token.split(".")[1];
      const decoded = JSON.parse(atob(payload.replace(/-/g, "+").replace(/_/g, "/")));
      return decoded.exp * 1000 < Date.now() + 60000;
    } catch {
      return true;
    }
  }

  async function browserToken() {
    let store = JSON.parse(localStorage.getItem("user-store") || "{}");
    let state = store.state || {};
    let token = state.accessToken || "";
    if (token && state.refreshToken && tokenExpiresSoon(token)) {
      const response = await fetch("https://api.noitu.fun/api/v1/auth/refresh", {
        method: "POST",
        headers: {"Content-Type": "application/json"},
        body: JSON.stringify({refreshToken: state.refreshToken})
      });
      if (response.ok) {
        const tokens = await response.json();
        token = tokens.accessToken || token;
        store.state = {...state, accessToken: token, refreshToken: tokens.refreshToken || state.refreshToken};
        localStorage.setItem("user-store", JSON.stringify(store));
      }
    }
    return token;
  }

  async function browserGameRequest(token, request) {
    const controller = new AbortController();
    const response = await withTimeout(fetch(`https://api.noitu.fun/api/v1${request.path}`, {
      method: request.method || "GET",
      headers: {
        Accept: "application/json",
        Authorization: `Bearer ${token}`,
        ...(request.body ? {"Content-Type": "application/json"} : {})
      },
      body: request.body ? JSON.stringify(request.body) : undefined,
      signal: controller.signal
    }), 15000, `noitu.fun ${request.path} timeout`).finally(() => controller.abort());
    const text = await response.text();
    let data = null;
    try { data = text ? JSON.parse(text) : null; } catch { data = {message: text}; }
    return {status: response.status, data};
  }

  function browserResponseWord(data) {
    return normalize(data?.word || data?.wordDescription?.word);
  }

  async function saveBrowserObservation(phrase, accepted) {
    const result = await collectorBackendJSON("/observations", {
      method: "POST",
      body: JSON.stringify({phrase, accepted})
    });
    if (result.status !== 202) throw new Error(result.data?.message || `backend observation ${result.status}`);
  }

  function collectorWait(milliseconds) {
    return new Promise(resolve => setTimeout(resolve, milliseconds));
  }

  async function collectBrowserGame() {
    if (browserCollectorPromise) return browserCollectorPromise;
    browserCollectorPromise = (async () => {
      const config = await collectorConfig();
      if (!config.collectorEnabled) return;
      await chrome.storage.local.set({collectorRunning: true, collectorLastError: ""});
      try {
        const token = await browserToken();
        if (!token) throw new Error("Không tìm thấy access token trong tab noitu.fun");
        const sessionId = crypto.randomUUID();
        const init = await browserGameRequest(token, {path: `/word-link/init?sessionId=${encodeURIComponent(sessionId)}`});
        if (init.status !== 200) throw new Error(`noitu.fun init HTTP ${init.status}`);
        let current = browserResponseWord(init.data);
        if (!twoSyllable(current)) throw new Error("noitu.fun init không trả về từ hai tiếng");
        const used = [current];
        await saveBrowserObservation(current, true);

        for (let move = 0; move < 10; move += 1) {
          const latest = await collectorConfig();
          if (!latest.collectorEnabled) break;
          const key = current.split(" ").at(-1);
          const suggestions = await collectorBackendJSON("/session/suggestions", {
            method: "POST",
            body: JSON.stringify({current_word: current, key, words: [], used})
          });
          if (suggestions.status !== 200) throw new Error(suggestions.data?.message || `backend suggestions ${suggestions.status}`);
          const candidate = suggestions.data?.suggestions?.find(item => twoSyllable(item.word) && !used.includes(normalize(item.word)))?.word;
          if (!candidate) break;
          await collectorWait(3000);
          const answer = await browserGameRequest(token, {
            path: "/word-link/answer",
            method: "POST",
            body: {answer: normalize(candidate), sessionId}
          });
          if (answer.status !== 200) throw new Error(`noitu.fun answer HTTP ${answer.status}`);
          if (!answer.data?.isSuccessful) {
            await saveBrowserObservation(candidate, false);
            break;
          }
          await saveBrowserObservation(candidate, true);
          used.push(normalize(candidate));
          if (answer.data.isFinished) break;
          current = browserResponseWord(answer.data);
          if (!twoSyllable(current)) break;
          await saveBrowserObservation(current, true);
          used.push(current);
        }
        await chrome.storage.local.set({collectorRunning: false, collectorLastError: ""});
      } catch (error) {
        await chrome.storage.local.set({collectorRunning: false, collectorLastError: error?.message || "browser collector failed"});
      } finally {
        browserCollectorPromise = null;
      }
    })();
    return browserCollectorPromise;
  }

  chrome.storage.onChanged.addListener((changes, area) => {
    if (area === "local") {
      if (changes.collectorEnabled?.newValue) collectBrowserGame();
      updateCollectorStatus();
    }
  });
  setInterval(collectBrowserGame, 60000);
  setTimeout(collectBrowserGame, 1500);

  new MutationObserver(() => sync()).observe(document.documentElement, {subtree: true, childList: true, characterData: true});
  setInterval(sync, 1000);
  sync();
})();
