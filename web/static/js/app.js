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

// Live node data: subscribes to a per-node Server-Sent Events stream and
// updates whichever pieces are present on the current page — Home's
// on-air pill and connected-node chips (with "talking" markers), and/or
// Stats's "Signal on input" cell and connection-history tables — as
// state changes, no page reload. Each element is looked up with
// querySelector and simply skipped if the page doesn't have it, so the
// same script serves both pages without knowing which one it's on.
// Progressive enhancement: every card is already rendered server-side,
// so if EventSource is unavailable or a proxy blocks the stream, the
// static snapshot simply stays put. DOM is built with
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

    // Scoped by node number, not just presence: with more than one node
    // configured, an unscoped querySelector would find whichever history
    // box happens to be first in the document and every node's stream
    // would write into it instead of its own.
    const historyBox = document.querySelector(
      '[data-live-history="' + CSS.escape(node) + '"]'
    );

    const es = new EventSource("/nodes/" + encodeURIComponent(node) + "/live", {
      withCredentials: true,
    });

    // "live" event: the right-now card (pill, connected chips, signal cell).
    es.addEventListener("live", (ev) => {
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
    });

    // "history" event: the whole history card, re-rendered server-side and
    // swapped in, so the connection-history tables update without a reload
    // and without duplicating their markup here. The payload is a
    // JSON-encoded HTML string produced by the same template as page load.
    es.addEventListener("history", (ev) => {
      if (!historyBox) return;
      let html;
      try {
        html = JSON.parse(ev.data);
      } catch (e) {
        return;
      }
      if (typeof html === "string" && html.length) historyBox.innerHTML = html;
    });

    es.onerror = () => {
      // EventSource retries on its own; just dim the live indicator while
      // the connection is down so the card doesn't look falsely live.
      if (indicator) indicator.classList.remove("on");
    };
  });
})();

// Link/unlink without a full-page reload: submit the quick-connect form
// in the background and show the result inline. The live stream above
// then reflects the connection appearing or dropping on its own. Falls
// back to a normal form POST if fetch is unavailable.
(function () {
  const forms = document.querySelectorAll("form[data-ajax-link]");
  if (!forms.length || typeof fetch === "undefined") return;

  forms.forEach((form) => {
    form.addEventListener("submit", async (e) => {
      e.preventDefault();

      const confirmMsg = form.getAttribute("data-confirm");
      if (confirmMsg && !window.confirm(confirmMsg)) return;

      const result = form.querySelector("[data-link-result]");
      const params = new URLSearchParams(new FormData(form));
      // FormData omits the submit button; carry which one was clicked.
      if (e.submitter && e.submitter.name) {
        params.set(e.submitter.name, e.submitter.value);
      }

      const buttons = form.querySelectorAll("button");
      buttons.forEach((b) => (b.disabled = true));

      function show(ok, msg) {
        if (!result) return;
        result.hidden = false;
        result.textContent = msg;
        result.classList.toggle("ok", ok);
        result.classList.toggle("error", !ok);
      }

      // getAttribute, not form.action: the submit buttons are named
      // "action", which DOM-clobbers the form's .action URL property into
      // returning the button collection instead of the endpoint.
      const endpoint = form.getAttribute("action");

      try {
        const res = await fetch(endpoint, {
          method: "POST",
          body: params,
          headers: { Accept: "application/json" },
          credentials: "same-origin",
        });
        const j = await res.json();
        show(!!j.ok, j.message || (j.ok ? "Done" : "Failed"));
      } catch (err) {
        show(false, "Request failed — check the connection and try again.");
      } finally {
        buttons.forEach((b) => (b.disabled = false));
      }
    });
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
