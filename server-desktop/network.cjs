function addressFamily(entry) {
  return entry?.family === 4 ? "IPv4" : entry?.family;
}

function interfaceRank(name) {
  const normalized = String(name || "").toLowerCase();
  if (/^(en0|ethernet|wi-?fi|wlan0)$/.test(normalized)) return 0;
  if (/^(en\d+|eth\d+|wlan\d+)$/.test(normalized)) return 1;
  if (/^(bridge|utun|awdl|llw|vmnet|vbox|docker)/.test(normalized)) return 9;
  return 4;
}

function usableIPv4(entry) {
  if (!entry || addressFamily(entry) !== "IPv4" || entry.internal) return false;
  const address = String(entry.address || "");
  return /^\d{1,3}(?:\.\d{1,3}){3}$/.test(address) && !address.startsWith("169.254.") && address !== "0.0.0.0";
}

function listLANAddresses(interfaces, port) {
  const candidates = [];
  for (const [name, entries] of Object.entries(interfaces || {})) {
    for (const entry of entries || []) {
      if (!usableIPv4(entry)) continue;
      candidates.push({ name, address: entry.address, rank: interfaceRank(name) });
    }
  }
  candidates.sort((left, right) => left.rank - right.rank || left.name.localeCompare(right.name) || left.address.localeCompare(right.address));
  const seen = new Set();
  return candidates.filter((candidate) => {
    if (seen.has(candidate.address)) return false;
    seen.add(candidate.address);
    return true;
  }).map((candidate) => ({ ...candidate, url: `http://${candidate.address}:${port}` }));
}

function primaryLANURL(interfaces, port) {
  return listLANAddresses(interfaces, port)[0]?.url || "";
}

function shouldUseAutomaticAddress(rawURL) {
  try {
    const parsed = new URL(String(rawURL || ""));
    return parsed.protocol === "http:" && (/^\d{1,3}(?:\.\d{1,3}){3}$/.test(parsed.hostname) || parsed.hostname === "localhost");
  } catch {
    return false;
  }
}

function replaceURLPort(rawURL, port) {
  const parsed = new URL(String(rawURL || ""));
  parsed.port = String(port);
  parsed.search = "";
  parsed.hash = "";
  return parsed.toString().replace(/\/$/, "");
}

module.exports = { listLANAddresses, primaryLANURL, replaceURLPort, shouldUseAutomaticAddress };
