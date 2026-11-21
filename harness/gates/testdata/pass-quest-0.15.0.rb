# The cask the release job renders and pushes to the tap `wxxb789/homebrew-tap`
# (goal 0017): `harness/lib/cask.ts` substitutes the two at-sign placeholders
# below with the release's values, and `harness/tools/cask-publish.ts` writes
# `Casks/pass-quest.rb`. The template is not a cask itself and Homebrew never
# reads it. Do not name the placeholders in a comment: the substitution is
# text-wide, and a rendered header that reads like a version is noise.
#
# The placeholders below are at-sign tokens with an uppercase name, deliberately
# not {{...}}: `harness/lib/cask.ts` holds the reason (Homebrew's install-step
# runner expands its own {{...}} tokens and leaves an unknown one silently in
# place) and the LEFTOVER check that fails the gate on a leftover at-sign token.
cask "pass-quest" do
  version "0.15.0"
  sha256 "477938dc1635971e23b2de952e04c684a87eda5c29fece7508d8b42c787b3e66"

  url "https://github.com/wxxb789/passquest/releases/download/v#{version}/pass-quest-#{version}-arm64.dmg"
  name "Pass Quest"
  desc "Local-first AI exam coach for one written exam"
  homepage "https://github.com/wxxb789/passquest"

  # A cask's livecheck url symbol is a string or :url/:homepage
  # (Cask-Cookbook, "livecheck"); :stable resolves only for a Formula
  # (livecheck/packages.rb). GithubLatest has priority 0 and Homebrew drops
  # priority-0 strategies unless the block names them
  # (livecheck/strategy/github_latest.rb, livecheck/strategy.rb), so both the
  # symbol and the strategy are named.
  livecheck do
    url :url
    strategy :github_latest
  end

  # The app is ad-hoc signed only and not notarized (goal 0013), and the dmg
  # carries one arm64 build; the same floor the package declares in mygo.json.
  # Cask-Cookbook orders depends_on by type, then by value
  # (rubocops/cask/stanza_order.rb), so arch: comes before macos:.
  depends_on arch: :arm64
  depends_on macos: :tahoe

  app "Pass Quest.app"

  # Homebrew quarantines the download, so without this the first launch meets
  # Gatekeeper. `xattr -rd` exits non-zero when a path carries no quarantine
  # attribute (the cask re-installed over itself), and that state is already the
  # one this step wants: the Cask-Cookbook documents `must_succeed: false` for
  # exactly this, a cleanup command that cannot run when its attribute is
  # absent. The steps sandbox already permits writes to the app directory —
  # `cask/artifact/install_steps.rb` allows the caskroom and the appdir before
  # any `writable_paths` — so this step declares none. The macOS install job
  # proves the attribute is gone; only that evidence, not caution, would add the
  # documented relative form
  # (`writable_paths: ["Pass Quest.app"], writable_base: :appdir`).
  postflight_steps do
    run "/usr/bin/xattr",
        args: ["-rd", "com.apple.quarantine", "{{appdir}}/Pass Quest.app"],
        must_succeed: false
  end

  uninstall quit: "com.github.wxxb789.passquest"

  # The data directory the install guides name, plus the WebKit data and cache
  # directories the app keeps for itself, which the macOS install job's
  # before/after `~/Library` listing measured (goal 0017, default 8). One string
  # per path, because Homebrew resolves each directive value as one literal path
  # (cask/artifact/abstract_uninstall.rb), in the array form `brew generate-zap`
  # emits (dev-cmd/generate-zap.rb); the elements are alphabetical because that
  # is what the Cask/ArrayAlphabetization cop requires of a `zap` array
  # (rubocops/cask/array_alphabetization.rb). The install job reads this list
  # back out of the installed cask file and requires every path to be gone after
  # `--zap`, so the stanza and that assertion cannot drift apart.
  zap trash: [
    "~/Library/Application Support/Pass Quest",
    "~/Library/Caches/com.github.wxxb789.passquest",
    "~/Library/WebKit/com.github.wxxb789.passquest",
  ]

  caveats <<~EOS
    Pass Quest is signed ad-hoc only: macOS 26 or macOS 27 on Apple silicon is required.
  EOS
end
