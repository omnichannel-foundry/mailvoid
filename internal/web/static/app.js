(function () {
  "use strict";

  var THEME_KEY = "mailvoid-theme";

  function applyTheme(theme) {
    document.documentElement.setAttribute("data-theme", theme);
    try {
      localStorage.setItem(THEME_KEY, theme);
    } catch (e) {
      /* private mode */
    }
    var button = document.getElementById("theme-toggle");
    if (button) {
      button.textContent = theme === "dark" ? "☀" : "☾";
      button.setAttribute("title", theme === "dark" ? "Switch to light theme" : "Switch to dark theme");
    }
  }

  function currentTheme() {
    return document.documentElement.getAttribute("data-theme") || "dark";
  }

  function escapeHTML(value) {
    return String(value == null ? "" : value).replace(/[&<>"']/g, function (ch) {
      switch (ch) {
        case "&": return "&amp;";
        case "<": return "&lt;";
        case ">": return "&gt;";
        case '"': return "&quot;";
        default: return "&#39;";
      }
    });
  }

  function severityClass(severity) {
    switch (severity) {
      case "error": return "badge-error";
      case "warn": return "badge-warning";
      case "debug": return "badge-ghost";
      default: return "badge-info";
    }
  }

  function eventRow(ev) {
    var row = document.createElement("tr");
    row.className = "event-row hover";
    row.setAttribute("data-event-id", ev.id || "");
    var payload;
    try {
      payload = JSON.stringify(ev.payload || {}, null, 2);
    } catch (e) {
      payload = "{}";
    }
    var time = ev.timestamp ? new Date(ev.timestamp).toLocaleTimeString() : "";
    row.innerHTML =
      '<td class="whitespace-nowrap align-top"><span class="text-xs opacity-60">' + escapeHTML(time) + "</span></td>" +
      '<td class="align-top"><span class="badge badge-sm ' + severityClass(ev.severity) + '">' + escapeHTML(ev.severity || "info") + "</span></td>" +
      '<td class="align-top"><code class="text-xs">' + escapeHTML(ev.type || "") + "</code></td>" +
      '<td class="align-top"><details><summary class="cursor-pointer text-xs opacity-70">payload</summary>' +
      '<pre class="mv-pre mt-2 rounded-lg bg-base-300/60 p-2">' + escapeHTML(payload) + "</pre></details></td>";
    return row;
  }

  function setupLiveEvents() {
    var body = document.getElementById("event-stream-body");
    if (!body || !window.WebSocket) {
      return;
    }
    var statusDot = document.getElementById("event-status");
    var pauseButton = document.getElementById("event-pause");
    var clearButton = document.getElementById("event-clear");
    var paused = false;
    var queue = [];

    function setPaused(value) {
      paused = value;
      if (statusDot) {
        statusDot.classList.toggle("paused", paused);
      }
      if (pauseButton) {
        pauseButton.textContent = paused ? "Resume" : "Pause";
      }
      if (!paused) {
        while (queue.length) {
          body.insertBefore(queue.shift(), body.firstChild);
        }
        trimRows();
      }
    }

    function trimRows() {
      while (body.children.length > 500) {
        body.removeChild(body.lastChild);
      }
    }

    function addEvent(ev) {
      if (paused) {
        queue.push(eventRow(ev));
        if (queue.length > 200) {
          queue.shift();
        }
        return;
      }
      body.insertBefore(eventRow(ev), body.firstChild);
      trimRows();
    }

    if (pauseButton) {
      pauseButton.addEventListener("click", function () {
        setPaused(!paused);
      });
    }
    if (clearButton) {
      clearButton.addEventListener("click", function () {
        queue = [];
        body.innerHTML = "";
      });
    }

    var retry = 1000;

    function connect() {
      var protocol = location.protocol === "https:" ? "wss" : "ws";
      var socket = new WebSocket(protocol + "://" + location.host + "/ws/events");
      socket.onopen = function () {
        retry = 1000;
        if (statusDot) {
          statusDot.classList.remove("paused");
        }
      };
      socket.onmessage = function (message) {
        try {
          addEvent(JSON.parse(message.data));
        } catch (e) {
          /* ignore malformed frame */
        }
      };
      socket.onclose = function () {
        if (statusDot) {
          statusDot.classList.add("paused");
        }
        setTimeout(connect, retry);
        retry = Math.min(retry * 2, 15000);
      };
      socket.onerror = function () {
        socket.close();
      };
    }

    connect();
  }

  function setupCountdowns() {
    var nodes = document.querySelectorAll("[data-expires]");
    if (!nodes.length) {
      return;
    }
    function tick() {
      var now = Date.now();
      for (var i = 0; i < nodes.length; i++) {
        var node = nodes[i];
        var target = Date.parse(node.getAttribute("data-expires"));
        if (isNaN(target)) {
          continue;
        }
        var remaining = target - now;
        if (remaining <= 0) {
          node.textContent = "expired";
          continue;
        }
        var seconds = Math.floor(remaining / 1000);
        var days = Math.floor(seconds / 86400);
        var hours = Math.floor((seconds % 86400) / 3600);
        var minutes = Math.floor((seconds % 3600) / 60);
        var value;
        if (days > 0) {
          value = days + "d " + hours + "h";
        } else if (hours > 0) {
          value = hours + "h " + minutes + "m";
        } else {
          value = minutes + "m " + (seconds % 60) + "s";
        }
        node.textContent = "in " + value;
      }
    }
    tick();
    setInterval(tick, 1000);
  }

  function setupCopy() {
    $(document).on("click", "[data-copy-target]", function (event) {
      event.preventDefault();
      var selector = this.getAttribute("data-copy-target");
      var target = document.querySelector(selector);
      if (!target) {
        return;
      }
      var text = target.textContent || "";
      var original = this.textContent;
      var done = function () {
        this.textContent = "Copied";
        var button = this;
        setTimeout(function () { button.textContent = original; }, 1200);
      }.bind(this);

      if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(text).then(done, done);
        return;
      }
      var area = document.createElement("textarea");
      area.value = text;
      document.body.appendChild(area);
      area.select();
      try {
        document.execCommand("copy");
      } catch (e) {
        /* ignore */
      }
      document.body.removeChild(area);
      done();
    });
  }

  function setupTheme() {
    $(document).on("click", "#theme-toggle", function (event) {
      event.preventDefault();
      applyTheme(currentTheme() === "dark" ? "light" : "dark");
    });
    applyTheme(currentTheme());
  }

  function setupDrawerClose() {
    $(document).on("click", ".mv-nav-link", function () {
      var toggle = document.getElementById("main-drawer");
      if (toggle && window.innerWidth < 1024) {
        toggle.checked = false;
      }
    });
  }

  document.addEventListener("DOMContentLoaded", function () {
    setupTheme();
    setupCopy();
    setupCountdowns();
    setupLiveEvents();
    setupDrawerClose();
  });
})();
