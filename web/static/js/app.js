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

// Live "Right now" card: subscribes to a per-node Server-Sent Events
// stream and updates the on-air pill, the connected-node chips (with
// "talking" markers), and the "Signal on input" cell as state changes —
// no page reload. Progressive enhancement: the card is already rendered
// server-side, so if EventSource is unavailable or a proxy blocks the
// stream, the static snapshot simply stays put. DOM is built with
// textContent/createElement so callsign/description text from the node
// directory can never inject markup.
(function () {
  const cards = document.querySelectorAll("[data-live-node]");
  if (!cards.length || typeof EventSource === "undefined") return;

  function renderPill(container, receiving) {
    const pill = document.createElement("span");
    pill.className = "status-pill " + (receiving ? "down" : "up");
    const dot = document.createElement("span");
    dot.className = "dot";
    pill.appendChild(dot);
    pill.appendChild(
      document.createTextNode(
        receiving ? "On the air — signal on input" : "Idle — no signal on input"
      )
    );
    container.replaceChildren(pill);
  }

  function renderConnected(container, connected) {
    if (!connected || !connected.length) {
      const hint = document.createElement("div");
      hint.className = "hint";
      hint.textContent = "Nothing connected.";
      container.replaceChildren(hint);
      return;
    }
    const frag = document.createDocumentFragment();
    connected.forEach((n) => {
      const chip = document.createElement("span");
      chip.className = "node-chip" + (n.keyed ? " keyed" : "");
      if (n.detail) chip.title = n.detail;

      const tag = document.createElement("span");
      tag.className = "tag";
      tag.textContent = n.number;
      chip.appendChild(tag);

      if (n.callsign) {
        const call = document.createElement("span");
        call.className = "node-call";
        call.textContent = n.callsign;
        chip.appendChild(call);
      }
      if (n.keyed) {
        const badge = document.createElement("span");
        badge.className = "talking-badge";
        badge.textContent = "talking";
        chip.appendChild(badge);
      }
      frag.appendChild(chip);
      frag.appendChild(document.createTextNode(" "));
    });
    container.replaceChildren(frag);
  }

  cards.forEach((card) => {
    const node = card.getAttribute("data-live-node");
    const pillBox = card.querySelector("[data-live-pill]");
    const connBox = card.querySelector("[data-live-connected]");
    const signalCell = card.querySelector("[data-live-signal]");
    const indicator = card.querySelector("[data-live-indicator]");

    const es = new EventSource("/nodes/" + encodeURIComponent(node) + "/live", {
      withCredentials: true,
    });

    es.onmessage = (ev) => {
      let s;
      try {
        s = JSON.parse(ev.data);
      } catch (e) {
        return;
      }
      if (indicator) indicator.classList.add("on");
      if (pillBox) renderPill(pillBox, s.receiving);
      if (connBox) renderConnected(connBox, s.connected);
      if (signalCell && s.signalOnInput) signalCell.textContent = s.signalOnInput;
    };

    es.onerror = () => {
      // EventSource retries on its own; just dim the live indicator while
      // the connection is down so the card doesn't look falsely live.
      if (indicator) indicator.classList.remove("on");
    };
  });
})();

// Node page "radio hardware" toggle: shows either the existing-device
// picker or the create-a-new-device sub-form depending on which radio
// button is selected, so both stay in the same form (one POST covers
// node + optional new device) without cluttering the page with an
// always-visible device-creation form most edits don't need.
(function () {
  const radios = document.querySelectorAll("[data-radio-mode]");
  if (!radios.length) return;
  const sections = {
    existing: document.querySelector('[data-radio-mode-section="existing"]'),
    new: document.querySelector('[data-radio-mode-section="new"]'),
  };
  function apply() {
    const checked = document.querySelector("[data-radio-mode]:checked");
    const mode = checked ? checked.value : "existing";
    for (const key in sections) {
      if (sections[key]) sections[key].style.display = key === mode ? "" : "none";
    }
  }
  radios.forEach((r) => r.addEventListener("change", apply));
  apply();
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
