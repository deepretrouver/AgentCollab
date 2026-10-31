const assert = require("node:assert/strict");
const test = require("node:test");
const { listLANAddresses, primaryLANURL, replaceURLPort, shouldUseAutomaticAddress } = require("./network.cjs");

test("prioritizes physical LAN interfaces and excludes loopback and link-local addresses", () => {
  const interfaces = {
    utun2: [{ family: "IPv4", address: "10.8.0.2", internal: false }],
    en7: [{ family: "IPv4", address: "192.168.30.28", internal: false }],
    lo0: [{ family: "IPv4", address: "127.0.0.1", internal: true }],
    en0: [{ family: "IPv4", address: "169.254.2.8", internal: false }],
  };
  assert.equal(primaryLANURL(interfaces, 8080), "http://192.168.30.28:8080");
  assert.deepEqual(listLANAddresses(interfaces, 8080).map((item) => item.url), ["http://192.168.30.28:8080", "http://10.8.0.2:8080"]);
});

test("recognizes generated IP URLs without converting named or HTTPS addresses to automatic mode", () => {
  assert.equal(shouldUseAutomaticAddress("http://192.168.1.20:8080"), true);
  assert.equal(shouldUseAutomaticAddress("http://localhost:8080"), true);
  assert.equal(shouldUseAutomaticAddress("http://agentcollab.office:8080"), false);
  assert.equal(shouldUseAutomaticAddress("https://collab.example.com"), false);
});

test("moves an automatic address to a healthy fallback port", () => {
  assert.equal(replaceURLPort("http://192.168.30.45:8080", 8083), "http://192.168.30.45:8083");
});
