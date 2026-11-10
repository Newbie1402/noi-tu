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

  new MutationObserver(() => sync()).observe(document.documentElement, {subtree: true, childList: true, characterData: true});
  setInterval(sync, 1000);
  sync();
})();
