const DEFAULT_CONFIG = {
  backendUrl: "http://127.0.0.1:8080",
  apiKey: "",
  collectorEnabled: false,
  collectorRunning: false,
  collectorLastError: ""
};

const COLLECTOR_ALARM = "browser-collector";
let collectorPromise = null;

async function getConfig() {
  return await chrome.storage.local.get(DEFAULT_CONFIG);
}

function backendURL(base, path) {
  return new URL(path, `${base.replace(/\/+$/, "")}/`).toString();
}

async function backendFetch(path, options = {}) {
  const config = await getConfig();
  const headers = new Headers(options.headers || {});
  headers.set("Content-Type", "application/json");
  if (config.apiKey) headers.set("X-API-Key", config.apiKey);
  return await fetch(backendURL(config.backendUrl, path), {...options, headers});
}

async function backendJSON(path, options = {}) {
  const response = await backendFetch(path, options);
  const text = await response.text();
  let data = null;
  if (text) {
    try { data = JSON.parse(text); } catch { data = {message: text}; }
  }
  return {status: response.status, data};
}

function readSession(tabId) {
  return chrome.scripting.executeScript({target: {tabId}, world: "MAIN", func: () => {
    try {
      const state = JSON.parse(localStorage.getItem("user-store") || "{}").state || {};
      return {accessToken: state.accessToken || null, refreshToken: state.refreshToken || null};
    } catch { return null; }
  }}).then(result => result[0]?.result);
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

async function refreshSession(tabId, session) {
  if (!session?.refreshToken || !tokenExpiresSoon(session.accessToken)) return session?.accessToken;
  const response = await fetch("https://api.noitu.fun/api/v1/auth/refresh", {
    method: "POST",
    headers: {"Content-Type": "application/json"},
    body: JSON.stringify({refreshToken: session.refreshToken})
  });
  if (!response.ok) return session.accessToken;
  const tokens = await response.json();
  const token = tokens.accessToken || session.accessToken;
  await chrome.scripting.executeScript({target: {tabId}, world: "MAIN", args: [tokens], func: next => {
    try {
      const store = JSON.parse(localStorage.getItem("user-store") || "{}");
      store.state = store.state || {};
      store.state.accessToken = next.accessToken;
      if (next.refreshToken) store.state.refreshToken = next.refreshToken;
      localStorage.setItem("user-store", JSON.stringify(store));
    } catch {}
  }});
  return token;
}

async function findBrowserSession() {
  const tabs = await chrome.tabs.query({url: "https://www.noitu.fun/*"});
  for (const tab of tabs) {
    try {
      const session = await readSession(tab.id);
      if (!session?.accessToken) continue;
      const token = await refreshSession(tab.id, session);
      if (token) return {tabId: tab.id, token};
    } catch {
      // Try the next open game tab.
    }
  }
  return null;
}

async function syncSession() {
  const session = await findBrowserSession();
  if (!session) return null;
  try {
    const response = await backendFetch("/collector/token", {
      method: "POST",
      body: JSON.stringify({token: session.token})
    });
    if (!response.ok) return null;
  } catch {
    return null;
  }
  return session;
}

async function pageGameRequest(tabId, token, request) {
  const result = await chrome.scripting.executeScript({
    target: {tabId},
    world: "MAIN",
    args: [token, request],
    func: async (accessToken, nextRequest) => {
      try {
        const response = await fetch(`https://api.noitu.fun/api/v1${nextRequest.path}`, {
          method: nextRequest.method || "GET",
          headers: {
            Accept: "application/json",
            Authorization: `Bearer ${accessToken}`,
            ...(nextRequest.body ? {"Content-Type": "application/json"} : {})
          },
          body: nextRequest.body ? JSON.stringify(nextRequest.body) : undefined
        });
        const text = await response.text();
        let data = null;
        try { data = text ? JSON.parse(text) : null; } catch { data = {message: text}; }
        return {status: response.status, data};
      } catch (error) {
        return {status: 0, data: {message: error.message}};
      }
    }
  });
  return result[0]?.result || {status: 0, data: {message: "browser request returned no result"}};
}

function normalize(value) {
  return (value || "").normalize("NFC").toLowerCase().trim().replace(/\s+/g, " ");
}

function isPhrase(value) {
  return /^\p{L}[\p{L}\p{M}'’-]*\s+\p{L}[\p{L}\p{M}'’-]*$/u.test(normalize(value));
}

function responseWord(data) {
  return normalize(data?.word || data?.wordDescription?.word);
}

async function saveObservation(phrase, accepted) {
  const result = await backendJSON("/observations", {
    method: "POST",
    body: JSON.stringify({phrase, accepted})
  });
  if (result.status !== 202) {
    throw new Error(result.data?.message || `backend observation ${result.status}`);
  }
}

function wait(milliseconds) {
  return new Promise(resolve => setTimeout(resolve, milliseconds));
}

async function collectBrowserGame() {
  if (collectorPromise) return collectorPromise;
  collectorPromise = (async () => {
    const config = await getConfig();
    if (!config.collectorEnabled) return {enabled: false, running: false};
    await chrome.storage.local.set({collectorRunning: true, collectorLastError: ""});
    try {
      const session = await findBrowserSession();
      if (!session) throw new Error("Không tìm thấy session đăng nhập trong tab noitu.fun");
      const sessionId = crypto.randomUUID();
      const start = await pageGameRequest(session.tabId, session.token, {
        path: `/word-link/start?sessionId=${encodeURIComponent(sessionId)}`
      });
      if (start.status !== 200) throw new Error(start.data?.message || `noitu.fun start HTTP ${start.status}`);
      let current = responseWord(start.data);
      if (!isPhrase(current)) throw new Error("noitu.fun start không trả về từ hai tiếng");
      const used = [current];
      await saveObservation(current, true);

      for (let move = 0; move < 10; move += 1) {
        const fields = current.split(" ");
        const key = fields[fields.length - 1];
        const suggestions = await backendJSON("/session/suggestions", {
          method: "POST",
          body: JSON.stringify({current_word: current, key, words: [], used})
        });
        if (suggestions.status !== 200) throw new Error(suggestions.data?.message || `backend suggestions ${suggestions.status}`);
        const candidate = suggestions.data?.suggestions?.find(item => isPhrase(item.word) && !used.includes(normalize(item.word)))?.word;
        if (!candidate) break;
        await wait(3000);
        const answer = await pageGameRequest(session.tabId, session.token, {
          path: "/word-link/answer",
          method: "POST",
          body: {answer: normalize(candidate), sessionId}
        });
        if (answer.status !== 200) throw new Error(`noitu.fun answer HTTP ${answer.status}`);
        if (!answer.data?.isSuccessful) {
          await saveObservation(candidate, false);
          break;
        }
        await saveObservation(candidate, true);
        used.push(normalize(candidate));
        if (answer.data.isFinished) break;
        current = responseWord(answer.data);
        if (!isPhrase(current)) break;
        await saveObservation(current, true);
        used.push(current);
      }
      await chrome.storage.local.set({collectorRunning: false, collectorLastError: ""});
      return {enabled: true, running: false};
    } catch (error) {
      const message = error?.message || "browser collector failed";
      await chrome.storage.local.set({collectorRunning: false, collectorLastError: message});
      return {enabled: true, running: false, last_error: message};
    } finally {
      collectorPromise = null;
    }
  })();
  return collectorPromise;
}

async function collectorStatus() {
  const config = await getConfig();
  return {
    enabled: !!config.collectorEnabled,
    running: !!config.collectorRunning,
    last_error: config.collectorLastError || ""
  };
}

async function setCollector(enabled) {
  await chrome.storage.local.set({collectorEnabled: enabled, collectorLastError: ""});
  if (!enabled) {
    await chrome.alarms.clear(COLLECTOR_ALARM);
    await chrome.storage.local.set({collectorRunning: false});
    return collectorStatus();
  }
  return collectorStatus();
}

async function handleMessage(message) {
  if (message.type === "getConfig") return await getConfig();
  if (message.type === "syncSession") return {ok: !!(await syncSession())};
  if (message.type === "suggest") {
    return await backendJSON("/session/suggestions", {method: "POST", body: JSON.stringify(message.payload)});
  }
  if (message.type === "collectorBackend") {
    const options = {method: message.options?.method || "GET"};
    if (message.options?.body !== undefined && message.options?.body !== null) {
      options.body = message.options.body;
    }
    return await backendJSON(message.path, options);
  }
  if (message.type === "getSettings") {
    const result = await backendJSON("/settings");
    if (result.status >= 200 && result.status < 300) result.data.collector = await collectorStatus();
    return result;
  }
  if (message.type === "toggleCollector") return {status: 200, data: await setCollector(!!message.enabled)};
  if (message.type === "toggleSuggestions") {
    return await backendJSON("/settings/suggestions", {method: "POST", body: JSON.stringify({enabled: !!message.enabled})});
  }
  throw new Error("unknown message");
}

chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
  if (!message?.type) return false;
  handleMessage(message)
    .then(sendResponse)
    .catch(error => sendResponse({status: 0, data: {message: error.message}}));
  return true;
});

chrome.alarms.create("sync-session", {periodInMinutes: 1});
chrome.alarms.onAlarm.addListener(alarm => {
  if (alarm.name === "sync-session") syncSession().catch(() => {});
});
chrome.tabs.onUpdated.addListener((id, info, tab) => {
  if (info.status === "complete" && tab.url?.startsWith("https://www.noitu.fun/")) syncSession().catch(() => {});
});
chrome.runtime.onStartup.addListener(async () => {
  await syncSession().catch(() => {});
});
chrome.runtime.onInstalled.addListener(() => syncSession().catch(() => {}));
