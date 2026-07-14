// Polls /api/status and updates the dashboard status pill + stat grid.
// Plain short-polling rather than SSE/WebSocket: for a handful of scalar
// values refreshed every few seconds this is simpler and just as
// "realtime" as it needs to be. A push-based stream (WebSocket) is worth
// adding later for high-frequency data like live PTT/COS or log tailing.
(function () {
  const pill = document.querySelector("[data-status-pill]");
  const uptimeEl = document.querySelector("[data-uptime]");
  const hostnameEl = document.querySelector("[data-hostname]");
  if (!pill) return;

  async function poll() {
    try {
      const res = await fetch("/api/status", { credentials: "same-origin" });
      if (!res.ok) throw new Error("status " + res.status);
      const s = await res.json();

      pill.classList.toggle("up", s.asterisk_running);
      pill.classList.toggle("down", !s.asterisk_running);
      pill.querySelector(".label").textContent = s.asterisk_running
        ? "Asterisk running"
        : "Asterisk stopped";

      if (uptimeEl) uptimeEl.textContent = s.uptime || "—";
      if (hostnameEl) hostnameEl.textContent = s.hostname || "—";
    } catch (e) {
      pill.classList.remove("up");
      pill.classList.add("down");
      pill.querySelector(".label").textContent = "Status unavailable";
    }
  }

  poll();
  setInterval(poll, 4000);
})();

// Connections page "quick action" buttons: fills the DTMF sequence
// field with <prefix><target node>, so the operator can review it
// before sending rather than the click sending anything directly.
(function () {
  const target = document.querySelector("[data-target-node]");
  const digitsField = document.querySelector("[data-dtmf-field]");
  if (!digitsField) return;
  document.querySelectorAll("[data-fill-digits]").forEach((btn) => {
    btn.addEventListener("click", () => {
      const prefix = btn.getAttribute("data-fill-digits");
      const node = target ? target.value.trim() : "";
      digitsField.value = prefix + node;
      digitsField.focus();
    });
  });
})();
