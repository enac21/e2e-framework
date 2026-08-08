(function () {
  "use strict";

  const API = "";
  let allTests = [];
  let allResults = [];
  let selectedTestId = null;
  let selectedTestStatuses = {};
  let pollingIntervals = {};
  let serverReachable = false;

  // ========== API HELPERS ==========

  async function apiFetch(path, opts) {
    opts = opts || {};
    var token = localStorage.getItem("e2e_token") || "";
    var headers = {};
    if (opts.headers) {
      var k;
      for (k in opts.headers) {
        headers[k] = opts.headers[k];
      }
    }
    if (token) headers["Authorization"] = "Bearer " + token;
    if (opts.body && typeof opts.body === "object") {
      headers["Content-Type"] = "application/json";
      opts.body = JSON.stringify(opts.body);
    }
    var fetchOpts = { method: opts.method || "GET", headers: headers };
    if (opts.body) fetchOpts.body = opts.body;
    var res = await fetch(API + path, fetchOpts);
    if (!res.ok) {
      var text = "";
      try { text = await res.text(); } catch (_) { /* ignore */ }
      throw new Error(text || ("HTTP " + res.status));
    }
    var ct = res.headers.get("content-type") || "";
    if (ct.indexOf("application/json") !== -1) return res.json();
    return res.text();
  }

  // ========== TOAST ==========

  function showToast(message, type) {
    var container = document.getElementById("toast-container");
    if (!container) return;
    var toast = document.createElement("div");
    toast.className = "toast toast-" + (type || "info");
    toast.textContent = message;
    container.appendChild(toast);
    setTimeout(function () { toast.remove(); }, 4000);
  }

  // ========== HEALTH CHECK ==========

  function updateHealthUI(connected) {
    var dot = document.getElementById("health-dot");
    var label = document.getElementById("health-label");
    if (!dot || !label) return;
    if (connected) {
      dot.classList.remove("offline");
      label.textContent = "Connected";
    } else {
      dot.classList.add("offline");
      label.textContent = "Disconnected";
    }
  }

  async function checkHealth() {
    try {
      await apiFetch("/health");
      serverReachable = true;
      updateHealthUI(true);
    } catch (e) {
      serverReachable = false;
      updateHealthUI(false);
      console.warn("[e2e-ui] Health check failed:", e.message);
    }
  }

  // ========== TOKEN SETUP ==========

  function setupTokenInput() {
    var container = document.getElementById("token-area");
    if (!container) return;
    var current = localStorage.getItem("e2e_token") || "";
    container.innerHTML =
      '<input type="text" id="token-input" placeholder="JWT Token (optional)" ' +
      'value="' + esc(current) + '" style="' +
      'background:var(--bg-tertiary);border:1px solid var(--border-color);border-radius:4px;' +
      'color:var(--text-primary);font-size:12px;padding:4px 8px;width:180px;outline:none;">';

    var input = document.getElementById("token-input");
    input.addEventListener("change", function () {
      localStorage.setItem("e2e_token", input.value.trim());
      showToast("Token updated", "info");
      loadTests();
      loadResults();
    });
  }

  // ========== TEST LIST ==========

  async function loadTests() {
    try {
      allTests = await apiFetch("/tests");
      renderTestList();
    } catch (e) {
      console.warn("[e2e-ui] Failed to load tests:", e.message);
      var list = document.getElementById("test-list");
      if (list) {
        list.innerHTML =
          '<div style="padding:20px;color:var(--accent-red);text-align:center;">' +
          "Failed to load tests<br><small>" + esc(e.message) + "</small></div>";
      }
    }
  }

  function renderTestList() {
    var list = document.getElementById("test-list");
    if (!list) return;
    var searchEl = document.getElementById("search-input");
    var filter = searchEl ? searchEl.value.toLowerCase() : "";
    var filtered = allTests.filter(function (t) {
      return t.id.toLowerCase().indexOf(filter) !== -1 ||
        (t.description || "").toLowerCase().indexOf(filter) !== -1;
    });

    if (filtered.length === 0) {
      list.innerHTML =
        '<div style="padding:20px;color:var(--text-muted);text-align:center;">No tests found</div>';
      return;
    }

    list.innerHTML = filtered.map(function (t) {
      var status = selectedTestStatuses[t.id] || "idle";
      var active = t.id === selectedTestId ? " active" : "";
      return (
        '<div class="test-item' + active + '" data-id="' + esc(t.id) + '">' +
        '<div class="test-item-status ' + status + '"></div>' +
        '<div class="test-item-info">' +
        '<div class="test-item-name">' + esc(t.id) + "</div>" +
        '<div class="test-item-desc">' + esc(t.description || "No description") + "</div>" +
        "</div>" +
        '<div class="test-item-badges">' +
        (t.enabled ? "" : '<span class="badge badge-disabled">OFF</span>') +
        (t.async ? '<span class="badge badge-async">ASYNC</span>' : "") +
        "</div>" +
        "</div>"
      );
    }).join("");

    list.querySelectorAll(".test-item").forEach(function (el) {
      el.addEventListener("click", function () { selectTest(el.dataset.id); });
    });
  }

  function selectTest(id) {
    selectedTestId = id;
    renderTestList();
    renderTestDetail();
  }

  // ========== TEST DETAIL ==========

  function getTest(id) {
    return allTests.find(function (t) { return t.id === id; });
  }

  function renderTestDetail() {
    var main = document.getElementById("main-content");
    if (!main) return;
    if (!selectedTestId) {
      main.innerHTML =
        '<div class="main-empty">' +
        '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2"/></svg>' +
        "<p>Select a test from the sidebar</p>" +
        "</div>";
      return;
    }

    var t = getTest(selectedTestId);
    if (!t) return;

    var html =
      '<div class="test-detail">' +
      renderTestHeader(t) +
      renderTestConfig(t) +
      renderTriggers(t) +
      renderReceivers(t) +
      renderResultPanel(t) +
      renderHistoryPanel(t) +
      "</div>";

    main.innerHTML = html;
    attachDetailListeners();
  }

  function renderTestHeader(t) {
    return (
      '<div class="test-detail-header">' +
      '<div class="test-detail-title">' +
      esc(t.id) +
      (t.enabled
        ? '<span class="badge badge-enabled">Enabled</span>'
        : '<span class="badge badge-disabled">Disabled</span>') +
      (t.async ? '<span class="badge badge-async">Async</span>' : "") +
      "</div>" +
      '<div class="test-detail-meta">' +
      (t.description ? "<span>" + esc(t.description) + "</span>" : "") +
      (t.schedule ? "<span>Schedule: " + esc(t.schedule) + "</span>" : "") +
      "<span>Triggers: " + (t.triggers ? t.triggers.length : 0) + "</span>" +
      "</div>" +
      '<div class="test-detail-actions">' +
      '<button class="btn btn-primary" id="btn-run"><svg viewBox="0 0 24 24" fill="currentColor"><path d="M8 5v14l11-7z"/></svg> Run</button>' +
      '<button class="btn" id="btn-add-sequence">+ Add to Sequence</button>' +
      "</div>" +
      "</div>"
    );
  }

  function renderTestConfig(t) {
    return (
      '<div class="section" id="section-config">' +
      '<div class="section-header">' +
      '<svg class="section-chevron" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M9 5l7 7-7 7"/></svg>' +
      '<span class="section-label">Configuration</span>' +
      "</div>" +
      '<div class="section-body"><div class="code-block">' +
      esc(yamlPreview(t)) +
      "</div></div></div>"
    );
  }

  function yamlPreview(t) {
    var lines = ["id: " + t.id, "enabled: " + t.enabled, "async: " + t.async];
    if (t.schedule) lines.push("schedule: " + t.schedule);
    if (t.retry && t.retry.enabled) {
      lines.push("retry:");
      lines.push("  enabled: true");
      lines.push("  attempts: " + t.retry.attempts);
      lines.push("  delay: " + t.retry.delay);
    }
    if (t.triggers) {
      lines.push("triggers:");
      t.triggers.forEach(function (tr, i) {
        lines.push("  - [" + (i + 1) + "] " + (tr.method || "GET") + " " + tr.url);
      });
    }
    return lines.join("\n");
  }

  function renderTriggers(t) {
    if (!t.triggers || t.triggers.length === 0) return "";

    var html =
      '<div class="section open" id="section-triggers">' +
      '<div class="section-header">' +
      '<svg class="section-chevron" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M9 5l7 7-7 7"/></svg>' +
      '<span class="section-label">Triggers</span>' +
      '<span class="section-badge" style="background:var(--accent-blue);color:#fff;">' +
      t.triggers.length + "</span>" +
      "</div>" +
      '<div class="section-body">';

    t.triggers.forEach(function (tr, i) {
      var method = (tr.method || "GET").toUpperCase();
      html +=
        '<div class="trigger-card">' +
        '<div class="trigger-card-header">' +
        '<span class="trigger-index">Step ' + (i + 1) + "</span>" +
        '<span class="badge badge-method ' + method.toLowerCase() + '">' + method + "</span>" +
        '<span style="font-family:monospace;font-size:12px;color:var(--accent-cyan);">' +
        esc(tr.url || "") + "</span>" +
        "</div>";

      if (tr.expected_status) {
        html += '<div class="receiver-result-detail">Expected status: ' + tr.expected_status + "</div>";
      }

      if (tr.headers && Object.keys(tr.headers).length > 0) {
        html += '<div class="receiver-result-detail" style="margin-top:6px;">Headers: ' +
          esc(JSON.stringify(tr.headers)) + "</div>";
      }

      if (tr.body && Object.keys(tr.body).length > 0) {
        html += '<div class="code-block" style="margin-top:6px;">' +
          esc(JSON.stringify(tr.body, null, 2)) + "</div>";
      }

      if (tr.extract && Object.keys(tr.extract).length > 0) {
        html += '<div class="receiver-result-detail" style="margin-top:6px;">Extract: ' +
          esc(JSON.stringify(tr.extract)) + "</div>";
      }

      if (tr.response_assertions && tr.response_assertions.length > 0) {
        html += '<div class="receiver-result-detail" style="margin-top:6px;">Response assertions:</div>';
        tr.response_assertions.forEach(function (a) {
          html += '<div class="code-block" style="margin-top:4px;">' +
            esc(a.type) + " | " + esc(a.field) + " = " + esc(a.value) + "</div>";
        });
      }

      if (tr.receivers && tr.receivers.length > 0) {
        html += '<div class="receiver-result-detail" style="margin-top:6px;">Receivers:</div>';
        tr.receivers.forEach(function (rcv) {
          html += '<div class="code-block" style="margin-top:4px;">' +
            esc(rcv.type) + " | timeout: " + esc(rcv.timeout || "default") +
            (rcv.recipient ? " | to: " + esc(rcv.recipient) : "") + "</div>";
          if (rcv.assertions && rcv.assertions.length > 0) {
            rcv.assertions.forEach(function (a) {
              html += '<div style="padding-left:16px;font-size:12px;color:var(--text-muted);">- ' +
                esc(a.type) + " " + esc(a.field) + " " + esc(a.value) + "</div>";
            });
          }
        });
      }

      html += "</div>";
    });

    html += "</div></div>";
    return html;
  }

  function renderReceivers(t) {
    if (!t.triggers) return "";
    var hasReceivers = t.triggers.some(function (tr) {
      return tr.receivers && tr.receivers.length > 0;
    });
    if (!hasReceivers) return "";

    var html =
      '<div class="section" id="section-receivers">' +
      '<div class="section-header">' +
      '<svg class="section-chevron" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M9 5l7 7-7 7"/></svg>' +
      '<span class="section-label">Receivers</span>' +
      "</div>" +
      '<div class="section-body">';

    t.triggers.forEach(function (tr, i) {
      if (!tr.receivers || tr.receivers.length === 0) return;
      tr.receivers.forEach(function (rcv) {
        html +=
          '<div class="receiver-result">' +
          '<div class="receiver-result-header">' +
          '<span class="badge badge-method" style="background:var(--accent-orange);color:#000;">' +
          esc(rcv.type) + "</span>" +
          "<span>Trigger #" + (i + 1) + "</span>" +
          "</div>" +
          '<div class="receiver-result-detail">Timeout: ' + esc(rcv.timeout || "default") + "</div>" +
          (rcv.recipient ? '<div class="receiver-result-detail">Recipient: ' + esc(rcv.recipient) + "</div>" : "") +
          "</div>";
      });
    });

    html += "</div></div>";
    return html;
  }

  function renderResultPanel() {
    return (
      '<div class="result-panel" id="result-panel">' +
      '<div class="result-panel-header">' +
      '<span class="result-panel-title">Last Result</span>' +
      '<div id="result-status"></div>' +
      "</div>" +
      '<div id="result-content" style="color:var(--text-muted);font-size:13px;">No result yet. Click Run to execute.</div>' +
      "</div>"
    );
  }

  function renderHistoryPanel(t) {
    var history = allResults.filter(function (r) { return r.test_id === t.id; });
    if (history.length === 0) return "";

    var html =
      '<div class="section open" id="section-history">' +
      '<div class="section-header">' +
      '<svg class="section-chevron" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M9 5l7 7-7 7"/></svg>' +
      '<span class="section-label">History</span>' +
      '<span class="section-badge" style="background:var(--bg-tertiary);color:var(--text-secondary);">' +
      history.length + "</span>" +
      "</div>" +
      '<div class="section-body">' +
      '<table class="history-table">' +
      "<thead><tr>" +
      "<th>Status</th><th>Run ID</th><th>Duration</th><th>Started</th><th>Attempts</th>" +
      "</tr></thead><tbody>";

    history.forEach(function (r) {
      html +=
        '<tr data-run-id="' + esc(r.run_id) + '">' +
        "<td>" + statusBadge(r.status) + "</td>" +
        '<td class="run-id-cell">' + esc(r.run_id.substring(0, 12)) + "</td>" +
        "<td>" + formatDuration(r.duration_ms) + "</td>" +
        "<td>" + formatTime(r.started_at) + "</td>" +
        "<td>" + (r.attempts || 1) + "</td>" +
        "</tr>";
    });

    html += "</tbody></table></div></div>";
    return html;
  }

  // ========== RESULT DETAIL ==========

  async function showResultDetail(runId) {
    try {
      var result = await apiFetch("/results/" + runId);
      renderResultContent(result);
    } catch (e) {
      showToast("Failed to load result: " + e.message, "error");
    }
  }

  function renderResultContent(r) {
    var content = document.getElementById("result-content");
    if (!content) return;

    var html =
      '<div style="margin-top:8px;">' +
      '<div class="result-stats">' +
      '<div class="result-stat"><div class="result-stat-value" style="color:var(--status-' + r.status + ');">' +
      r.status.toUpperCase() +
      "</div><div class="result-stat-label">Status</div></div>" +
      '<div class="result-stat"><div class="result-stat-value">' +
      formatDuration(r.duration_ms) +
      "</div><div class="result-stat-label">Duration</div></div>" +
      '<div class="result-stat"><div class="result-stat-value">' +
      (r.attempts || 1) +
      "</div><div class="result-stat-label">Attempts</div></div>" +
      "</div>";

    if (r.error) {
      html +=
        '<div class="code-block" style="margin-top:12px;color:var(--accent-red);">' +
        esc(r.error) + "</div>";
    }

    if (r.receivers && r.receivers.length > 0) {
      html += '<div style="margin-top:12px;font-size:13px;font-weight:600;">Receivers</div>';
      r.receivers.forEach(function (rcv) {
        html +=
          '<div class="receiver-result">' +
          '<div class="receiver-result-header">' +
          '<span class="badge badge-method" style="background:var(--accent-orange);color:#000;">' +
          esc(rcv.type) + "</span>" +
          statusBadge(rcv.status) +
          "<span>" + formatDuration(rcv.duration_ms) + "</span>" +
          "</div>" +
          (rcv.error
            ? '<div class="receiver-result-detail" style="color:var(--accent-red);">' + esc(rcv.error) + "</div>"
            : "") +
          (rcv.message
            ? '<div class="code-block" style="margin-top:6px;">' + esc(JSON.stringify(rcv.message, null, 2)) + "</div>"
            : "") +
          "</div>";
      });
    }

    if (r.trigger_vars && Object.keys(r.trigger_vars).length > 0) {
      html +=
        '<div style="margin-top:12px;font-size:13px;font-weight:600;">Extracted Variables</div>' +
        '<div class="code-block">' + esc(JSON.stringify(r.trigger_vars, null, 2)) + "</div>";
    }

    html += "</div>";
    content.innerHTML = html;

    var statusEl = document.getElementById("result-status");
    if (statusEl) statusEl.innerHTML = statusBadge(r.status);
  }

  // ========== RUN TEST ==========

  async function runTest(testId) {
    selectedTestStatuses[testId] = "running";
    renderTestList();
    showResultPanelRunning();

    try {
      var result = await apiFetch("/run?id=" + encodeURIComponent(testId), {
        method: "POST",
      });

      if (result.status === "running") {
        showToast("Test started (async). Polling for result...", "info");
        startPolling(result.run_id, testId);
      } else {
        selectedTestStatuses[testId] = result.status;
        renderTestList();
        renderResultContent(result);
        showToast("Test " + result.status, result.status === "passed" ? "success" : "error");
        loadResults();
      }
    } catch (e) {
      selectedTestStatuses[testId] = "error";
      renderTestList();
      showToast("Run failed: " + e.message, "error");
    }
  }

  function showResultPanelRunning() {
    var content = document.getElementById("result-content");
    if (content) {
      content.innerHTML =
        '<div style="display:flex;align-items:center;gap:10px;padding:16px 0;">' +
        '<div class="spinner spinner-lg"></div>' +
        "<span>Running test...</span>" +
        "</div>";
    }
    var statusEl = document.getElementById("result-status");
    if (statusEl) statusEl.innerHTML = statusBadge("running");
  }

  // ========== POLLING ==========

  function startPolling(runId, testId) {
    stopPolling(runId);
    pollingIntervals[runId] = setInterval(async function () {
      try {
        var result = await apiFetch("/results/" + runId);
        if (result.status !== "running") {
          stopPolling(runId);
          selectedTestStatuses[testId] = result.status;
          renderTestList();
          renderResultContent(result);
          showToast(
            "Test " + result.status,
            result.status === "passed" ? "success" : "error"
          );
          loadResults();
        }
      } catch (_) {
        stopPolling(runId);
      }
    }, 2000);
  }

  function stopPolling(runId) {
    if (pollingIntervals[runId]) {
      clearInterval(pollingIntervals[runId]);
      delete pollingIntervals[runId];
    }
  }

  // ========== HISTORY ==========

  async function loadResults() {
    try {
      allResults = await apiFetch("/results");
      if (selectedTestId) renderTestDetail();
    } catch (_) {
      // silent - results may not be available
    }
  }

  // ========== SEQUENCE BUILDER ==========

  var sequenceItems = [];

  function openSequenceModal() {
    sequenceItems = [];
    if (selectedTestId) sequenceItems.push(selectedTestId);
    renderSequenceModal();
  }

  function renderSequenceModal() {
    var existing = document.getElementById("sequence-modal");
    if (existing) existing.remove();

    var overlay = document.createElement("div");
    overlay.className = "modal-overlay";
    overlay.id = "sequence-modal";

    var inSequence = {};
    sequenceItems.forEach(function (id) { inSequence[id] = true; });

    var html =
      '<div class="modal">' +
      '<div class="modal-title">Build Test Sequence</div>' +
      '<div style="font-size:13px;color:var(--text-muted);margin-bottom:16px;">' +
      "Click tests below to add them. Drag to reorder." +
      "</div>" +
      '<div class="sequence-list" id="sequence-list">';

    if (sequenceItems.length === 0) {
      html += '<div style="padding:20px;text-align:center;color:var(--text-muted);">No tests added yet</div>';
    }

    sequenceItems.forEach(function (id, i) {
      html +=
        '<div class="sequence-item" draggable="true" data-index="' + i + '">' +
        '<div class="sequence-item-number">' + (i + 1) + "</div>" +
        '<span class="sequence-item-name">' + esc(id) + "</span>" +
        '<button class="sequence-item-remove" data-index="' + i + '">' +
        '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M18 6L6 18M6 6l12 12"/></svg>' +
        "</button>" +
        "</div>";
    });

    html += "</div>";

    html += '<div class="sequence-available"><div class="sequence-available-title">Available Tests</div><div class="sequence-available-list">';
    allTests.forEach(function (t) {
      var cls = inSequence[t.id] ? " in-sequence" : "";
      html += '<div class="sequence-available-item' + cls + '" data-id="' + t.id + '">' + esc(t.id) + "</div>";
    });
    html += "</div></div>";

    html +=
      '<div class="sequence-options">' +
      '<label class="sequence-option">' +
      "Delay between tests: " +
      '<input type="text" id="seq-delay" value="1s" placeholder="e.g. 2s">' +
      "</label>" +
      '<label class="sequence-option">' +
      '<input type="checkbox" id="seq-skip-fail"> Stop on first failure' +
      "</label>" +
      "</div>";

    html +=
      '<div class="modal-actions">' +
      '<button class="btn" id="seq-cancel">Cancel</button>' +
      '<button class="btn btn-primary" id="seq-run"' +
      (sequenceItems.length === 0 ? " disabled" : "") +
      ">Run Sequence (" + sequenceItems.length + " tests)</button>" +
      "</div>" +
      "</div>";

    overlay.innerHTML = html;
    document.body.appendChild(overlay);
    attachSequenceListeners();
  }

  function attachSequenceListeners() {
    var overlay = document.getElementById("sequence-modal");
    if (!overlay) return;

    overlay.addEventListener("click", function (e) {
      if (e.target === overlay) overlay.remove();
    });

    document.getElementById("seq-cancel").addEventListener("click", function () { overlay.remove(); });

    document.getElementById("seq-run").addEventListener("click", function () {
      overlay.remove();
      runSequence();
    });

    document.querySelectorAll(".sequence-item-remove").forEach(function (btn) {
      btn.addEventListener("click", function (e) {
        e.stopPropagation();
        sequenceItems.splice(parseInt(btn.dataset.index), 1);
        renderSequenceModal();
      });
    });

    document.querySelectorAll(".sequence-available-item:not(.in-sequence)").forEach(function (el) {
      el.addEventListener("click", function () {
        sequenceItems.push(el.dataset.id);
        renderSequenceModal();
      });
    });

    var dragIdx = null;
    document.querySelectorAll(".sequence-item").forEach(function (el) {
      el.addEventListener("dragstart", function () {
        dragIdx = parseInt(el.dataset.index);
        el.style.opacity = "0.5";
      });
      el.addEventListener("dragend", function () {
        el.style.opacity = "1";
        dragIdx = null;
      });
      el.addEventListener("dragover", function (e) { e.preventDefault(); });
      el.addEventListener("drop", function (e) {
        e.preventDefault();
        var dropIdx = parseInt(el.dataset.index);
        if (dragIdx !== null && dragIdx !== dropIdx) {
          var item = sequenceItems.splice(dragIdx, 1)[0];
          sequenceItems.splice(dropIdx, 0, item);
          renderSequenceModal();
        }
      });
    });
  }

  async function runSequence() {
    if (sequenceItems.length === 0) return;

    var delayEl = document.getElementById("seq-delay");
    var delay = delayEl ? delayEl.value : "1s";
    var skipFailEl = document.getElementById("seq-skip-fail");
    var skipFail = skipFailEl ? skipFailEl.checked : false;

    sequenceItems.forEach(function (id) {
      selectedTestStatuses[id] = "running";
    });
    renderTestList();

    var url = "/run-sequence?test_delay=" + encodeURIComponent(delay);
    if (skipFail) url += "&skip_fail_test=true";

    try {
      var results = await apiFetch(url, {
        method: "POST",
        body: sequenceItems,
      });

      if (Array.isArray(results)) {
        results.forEach(function (r) {
          selectedTestStatuses[r.test_id] = r.status;
        });
        var passed = results.filter(function (r) { return r.status === "passed"; }).length;
        var failed = results.filter(function (r) { return r.status !== "passed"; }).length;
        showToast(
          "Sequence completed: " + passed + " passed, " + failed + " failed",
          failed > 0 ? "error" : "success"
        );
      }

      renderTestList();
      loadResults();
      if (selectedTestId) renderTestDetail();
    } catch (e) {
      sequenceItems.forEach(function (id) {
        selectedTestStatuses[id] = "error";
      });
      renderTestList();
      showToast("Sequence failed: " + e.message, "error");
    }
  }

  // ========== EVENT LISTENERS ==========

  function attachDetailListeners() {
    var btnRun = document.getElementById("btn-run");
    if (btnRun) {
      btnRun.addEventListener("click", function () { runTest(selectedTestId); });
    }

    var btnSeq = document.getElementById("btn-add-sequence");
    if (btnSeq) {
      btnSeq.addEventListener("click", openSequenceModal);
    }

    document.querySelectorAll(".section-header").forEach(function (header) {
      header.addEventListener("click", function () {
        header.parentElement.classList.toggle("open");
      });
    });

    document.querySelectorAll(".history-table tr[data-run-id]").forEach(function (row) {
      row.addEventListener("click", function () { showResultDetail(row.dataset.runId); });
    });
  }

  function initListeners() {
    var searchInput = document.getElementById("search-input");
    if (searchInput) searchInput.addEventListener("input", renderTestList);

    var btnRunAll = document.getElementById("btn-run-all");
    if (btnRunAll) btnRunAll.addEventListener("click", openSequenceModal);

    var btnRefresh = document.getElementById("btn-refresh");
    if (btnRefresh) {
      btnRefresh.addEventListener("click", function () {
        checkHealth().then(function () {
          loadTests();
          loadResults();
          showToast("Refreshed", "info");
        });
      });
    }
  }

  // ========== UTILS ==========

  function esc(s) {
    if (s == null) return "";
    var div = document.createElement("div");
    div.textContent = String(s);
    return div.innerHTML;
  }

  function statusBadge(status) {
    return '<span class="badge badge-status ' + esc(status) + '">' + esc(status) + "</span>";
  }

  function formatDuration(ms) {
    if (!ms || ms === 0) return "-";
    if (ms < 1000) return ms + "ms";
    return (ms / 1000).toFixed(1) + "s";
  }

  function formatTime(iso) {
    if (!iso) return "-";
    try {
      var d = new Date(iso);
      return d.toLocaleTimeString();
    } catch (_) {
      return iso;
    }
  }

  // ========== INIT ==========

  async function init() {
    initListeners();
    setupTokenInput();
    await checkHealth();
    await loadTests();
    await loadResults();
    setInterval(checkHealth, 30000);
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})();
