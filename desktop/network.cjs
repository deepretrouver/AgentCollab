function normalizeHubURL(value) {
  const raw = String(value ?? "").trim().replace(/\/+$/, "");
  const withScheme = /^[a-z][a-z0-9+.-]*:\/\//i.test(raw) ? raw : `http://${raw}`;
  const parsed = new URL(withScheme);
  if (!["http:", "https:"].includes(parsed.protocol) || !parsed.host || parsed.username || parsed.password) {
    throw new Error("团队地址必须是可访问的 HTTP 或 HTTPS 地址");
  }
  parsed.pathname = parsed.pathname.replace(/\/+$/, "");
  parsed.search = "";
  parsed.hash = "";
  return parsed.toString().replace(/\/$/, "");
}

function mergeHubURLs(...groups) {
  const seen = new Set();
  const result = [];
  for (const group of groups) {
    for (const raw of group || []) {
      try {
        const value = normalizeHubURL(raw);
        if (seen.has(value)) continue;
        seen.add(value);
        result.push(value);
      } catch {
        // Ignore malformed addresses learned from stale settings or discovery.
      }
    }
  }
  return result;
}

function parseTeamInput(value) {
  const raw = String(value ?? "").trim();
  const withScheme = /^[a-z][a-z0-9+.-]*:\/\//i.test(raw) ? raw : `http://${raw}`;
  const parsed = new URL(withScheme);
  if (parsed.protocol === "agentcollab:" && parsed.hostname === "join") {
    return {
      hubUrl: normalizeHubURL(parsed.searchParams.get("hub")),
      inviteCode: String(parsed.searchParams.get("invite") || "").trim(),
    };
  }
  return {
    hubUrl: normalizeHubURL(parsed.toString()),
    inviteCode: String(parsed.searchParams.get("join") || "").trim(),
  };
}

function expandSameHostPorts(urls, firstPort = 8080, lastPort = 8090) {
  const expanded = [...mergeHubURLs(urls)];
  for (const raw of urls || []) {
    let parsed;
    try { parsed = new URL(normalizeHubURL(raw)); } catch { continue; }
    if (parsed.protocol !== "http:") continue;
    for (let port = firstPort; port <= lastPort; port += 1) {
      const candidate = new URL(parsed.toString());
      candidate.port = String(port);
      expanded.push(candidate.toString().replace(/\/$/, ""));
    }
  }
  return mergeHubURLs(expanded);
}

function hubProxyBypassRules(urls) {
  const hosts = [];
  for (const raw of mergeHubURLs(urls)) {
    try { hosts.push(new URL(raw).hostname); } catch {}
  }
  // Electron follows Chromium's comma-separated proxy bypass syntax. Proxy
  // rules use semicolons, but bypass rules do not.
  return Array.from(new Set(["<local>", "localhost", "127.0.0.1", "[::1]", ...hosts])).join(",");
}

function ipv4ToInteger(value) {
  const parts = String(value || "").split(".").map(Number);
  if (parts.length !== 4 || parts.some((part) => !Number.isInteger(part) || part < 0 || part > 255)) return undefined;
  return parts.reduce((total, part) => ((total << 8) | part) >>> 0, 0) >>> 0;
}

function integerToIPv4(value) {
  return [24, 16, 8, 0].map((shift) => (value >>> shift) & 255).join(".");
}

function broadcastAddresses(interfaces) {
  const values = [];
  for (const entries of Object.values(interfaces || {})) {
    for (const entry of entries || []) {
      const family = entry?.family === 4 ? "IPv4" : entry?.family;
      if (family !== "IPv4" || entry.internal) continue;
      const address = ipv4ToInteger(entry.address);
      const netmask = ipv4ToInteger(entry.netmask);
      if (address === undefined || netmask === undefined || String(entry.address).startsWith("169.254.")) continue;
      values.push(integerToIPv4(((address & netmask) | (~netmask >>> 0)) >>> 0));
    }
  }
  return Array.from(new Set([...values, "255.255.255.255"]));
}

function parseDiscoveryResponse(value) {
  try {
    const parsed = JSON.parse(Buffer.isBuffer(value) ? value.toString("utf8") : String(value));
    if (parsed?.service !== "agentcollab-hub") return undefined;
    return { hubId: String(parsed.hub_id || ""), name: String(parsed.name || ""), urls: mergeHubURLs(parsed.urls || []) };
  } catch {
    return undefined;
  }
}

module.exports = { broadcastAddresses, expandSameHostPorts, hubProxyBypassRules, mergeHubURLs, normalizeHubURL, parseDiscoveryResponse, parseTeamInput };
