const assert = require("node:assert/strict");
const test = require("node:test");
const { broadcastAddresses, expandSameHostPorts, hubProxyBypassRules, mergeHubURLs, normalizeHubURL, parseDiscoveryResponse, parseTeamInput } = require("./network.cjs");

test("normalizes and deduplicates team addresses", () => {
  assert.equal(normalizeHubURL("192.168.30.45:8080/"), "http://192.168.30.45:8080");
  assert.deepEqual(mergeHubURLs(["http://192.168.30.45:8080/"], ["192.168.30.45:8080", "bad://host"]), ["http://192.168.30.45:8080"]);
});

test("accepts one human invitation link as the complete Client input", () => {
  assert.deepEqual(parseTeamInput("http://192.168.30.45:8080/?join=invite-123"), {
    hubUrl: "http://192.168.30.45:8080",
    inviteCode: "invite-123",
  });
});

test("parses an AgentCollab deep link", () => {
  assert.deepEqual(parseTeamInput("agentcollab://join?hub=http%3A%2F%2F192.168.30.45%3A8080&invite=invite-123"), {
    hubUrl: "http://192.168.30.45:8080",
    inviteCode: "invite-123",
  });
});

test("adds bounded same-host fallback ports without changing https endpoints", () => {
  assert.deepEqual(expandSameHostPorts(["http://192.168.30.45:8082"], 8080, 8082), [
    "http://192.168.30.45:8082",
    "http://192.168.30.45:8080",
    "http://192.168.30.45:8081",
  ]);
  assert.deepEqual(expandSameHostPorts(["https://collab.office"], 8080, 8082), ["https://collab.office"]);
});

test("bypasses the system proxy for verified team endpoints", () => {
  assert.equal(
    hubProxyBypassRules(["http://192.168.30.45:8080", "https://collab.office", "http://192.168.30.45:8081"]),
    "<local>,localhost,127.0.0.1,[::1],192.168.30.45,collab.office",
  );
});

test("derives interface broadcast addresses for LAN discovery", () => {
  assert.deepEqual(broadcastAddresses({
    en0: [{ family: "IPv4", address: "192.168.30.45", netmask: "255.255.254.0", internal: false }],
    lo0: [{ family: "IPv4", address: "127.0.0.1", netmask: "255.0.0.0", internal: true }],
  }), ["192.168.31.255", "255.255.255.255"]);
});

test("accepts only AgentCollab discovery packets", () => {
  assert.equal(parseDiscoveryResponse('{"service":"other","urls":["http://10.0.0.1:8080"]}'), undefined);
  assert.deepEqual(parseDiscoveryResponse('{"service":"agentcollab-hub","hub_id":"hub-one","name":"Research","urls":["http://10.0.0.1:8080/"]}'), {
    hubId: "hub-one", name: "Research", urls: ["http://10.0.0.1:8080"],
  });
});
