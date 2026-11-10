const DEFAULT_CONFIG = {
  backendUrl: "http://127.0.0.1:8080",
  apiKey: ""
};

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

async function syncSession() {
  const tabs = await chrome.tabs.query({url: "https://www.noitu.fun/*"});
  for (const tab of tabs) {
    try {
      const session = await readSession(tab.id);
      if (!session?.accessToken) continue;
      const token = await refreshSession(tab.id, session);
      const response = await backendFetch("/collector/token", {
        method: "POST",
        body: JSON.stringify({token})
      });
      if (response.ok) return;
    } catch {
      // Retry on the next alarm when the browser tab or backend is available.
    }
  }
}

async function handleMessage(message) {
  if (message.type === "getConfig") return await getConfig();
  if (message.type === "syncSession") {
    await syncSession();
    return {ok: true};
  }
  if (message.type === "suggest") {
    return await backendJSON("/session/suggestions", {method: "POST", body: JSON.stringify(message.payload)});
  }
  if (message.type === "getSettings") return await backendJSON("/settings");
  if (message.type === "toggleCollector") {
    return await backendJSON("/collector/toggle", {method: "POST", body: JSON.stringify({enabled: !!message.enabled})});
  }
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
  if (alarm.name === "sync-session") syncSession();
});
chrome.tabs.onUpdated.addListener((id, info, tab) => {
  if (info.status === "complete" && tab.url?.startsWith("https://www.noitu.fun/")) syncSession();
});
chrome.runtime.onStartup.addListener(syncSession);
chrome.runtime.onInstalled.addListener(syncSession);
