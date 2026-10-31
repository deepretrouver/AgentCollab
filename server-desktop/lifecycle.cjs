function launchedHidden(argv = []) {
  return argv.includes("--hidden");
}

function shouldRevealInitialWindow(argv = [], configured = false) {
  // A first launch must still show onboarding. The optional --hidden flag is
  // reserved for explicit administrative launches; normal app launches are visible.
  return !configured || !launchedHidden(argv);
}

module.exports = { launchedHidden, shouldRevealInitialWindow };
