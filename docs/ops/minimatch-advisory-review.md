# Build dependency advisory review / 构建依赖安全复核

Date: 2026-10-31. Status: **open; no audit exemption approved**.

Source: [GHSA-g3hv-75wg-m249](https://github.com/advisories/GHSA-g3hv-75wg-m249).
The advisory's affected range is `<11.0.1`; its technical description identifies
pattern amplification in the level-2 preprocessing implementation of 10.2.6.

Client and Server now resolve their modern minimatch consumers to 11.0.2.
Legacy consumers retain 3.1.5 and 5.1.9 because their CommonJS export is callable;
blindly replacing it with a newer namespace export would change their API.
This does not assert those legacy versions are universally safe.

## Observed legacy source / 已检查的旧版源码

The installed `minimatch.js` entrypoints do not contain `optimizationLevel`,
`firstPhasePreProcess`, or `secondPhasePreProcess`. Exact reviewed SHA-256:

| Version | SHA-256 of minimatch.js |
| --- | --- |
| 3.1.5 | 746849f267c4868a53729225bd85c7dccd387712136fb305fcc0a3eadd4a3d9d |
| 5.1.9 | 1339bca8b70e33847860daa8fc3ccfa62487e143670da01a3b718d40d5fd424f |

Source inspection of both legacy `make()` implementations shows brace
expansion followed by path splitting, per-component parsing and invalid-entry
filtering, without the advisory's iterative cloning/preprocessing pass. A
bounded probe using `a/` + eight repetitions of `**/../` + `z`, with
`optimizationLevel: 2`, produced one globParts entry and one compiled set in
both legacy versions. Their `firstPhasePreProcess` prototype member is absent.
This probe checks the specific transformation, not all possible DoS patterns.

The old code uses a different parsing path. The absent identifiers alone are
not a complete exploitability assessment. npm audit still reports this advisory
for the legacy dependencies and their dependent packages; the CI audit remains
unchanged and fails rather than hiding those results.

Next: establish whether the advisory mechanism exists in the legacy parsing
path, and either remove/upgrade the legacy consumers or document a narrowly
reviewed, time-limited exception. Do not use `npm audit fix --force`: its proposed
builder downgrade is not a verified compatibility or security fix.

新版接口依赖已经升级。旧版没有公告中所指的预处理函数，但不能仅凭这一点
宣称没有风险。当前不做全局替换、不关闭安全审计，也不发布受阻的安装包。
