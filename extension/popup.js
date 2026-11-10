const backend = document.getElementById("backend");
const apiKey = document.getElementById("api-key");
const suggestions = document.getElementById("suggestions");
const collector = document.getElementById("collector");
const status = document.getElementById("status");

function show(message, error = false) {
  status.textContent = message;
  status.style.color = error ? "#b42318" : "#286d5d";
}

async function message(payload) {
  return await chrome.runtime.sendMessage(payload);
}

async function refresh() {
  const result = await message({type: "getSettings"});
  if (!result || result.status < 200 || result.status >= 300) {
    show(result?.data?.message || "Không kết nối được backend", true);
    return;
  }
  suggestions.checked = !!result.data.suggestions_enabled;
  collector.checked = !!result.data.collector?.running;
  show(`Gợi ý: ${suggestions.checked ? "bật" : "tắt"}\nCron: ${collector.checked ? "đang chạy" : "đã tắt"}`);
}

async function requestBackendPermission(value) {
  const url = new URL(value);
  const origin = `${url.protocol}//${url.host}/*`;
  if (origin === "http://127.0.0.1:8080/*" || origin === "http://localhost:8080/*") return true;
  return await chrome.permissions.request({origins: [origin]});
}

document.getElementById("save").addEventListener("click", async () => {
  try {
    const value = backend.value.trim().replace(/\/+$/, "");
    const parsed = new URL(value);
    if (!/^https?:$/.test(parsed.protocol)) throw new Error("Backend phải dùng http hoặc https");
    if (!await requestBackendPermission(value)) throw new Error("Chưa cấp quyền gọi backend");
    await chrome.storage.local.set({backendUrl: value, apiKey: apiKey.value.trim()});
    show("Đã lưu cấu hình");
    await refresh();
  } catch (error) {
    show(error.message, true);
  }
});

suggestions.addEventListener("change", async () => {
  const result = await message({type: "toggleSuggestions", enabled: suggestions.checked});
  if (result?.status !== 200) show(result?.data?.message || "Không đổi được trạng thái gợi ý", true);
  else await refresh();
});

collector.addEventListener("change", async () => {
  const result = await message({type: "toggleCollector", enabled: collector.checked});
  if (result?.status !== 200) {
    collector.checked = !collector.checked;
    show(result?.data?.last_error || result?.data?.message || "Không đổi được trạng thái cron", true);
  } else await refresh();
});

(async () => {
  const config = await message({type: "getConfig"});
  backend.value = config?.backendUrl || "http://127.0.0.1:8080";
  apiKey.value = config?.apiKey || "";
  await refresh();
})();
