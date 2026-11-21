Pass Quest {{VERSION}}, a local-first desktop exam coach.

## Downloads

| Platform | File |
|---|---|
| Windows x64 (Windows 11+, per-user installer, no admin) | `pass-quest-setup-{{VERSION}}-amd64.exe` |
| macOS arm64 (macOS 26 and 27) | `pass-quest-{{VERSION}}-arm64.dmg` |
| Linux x86_64 (second-tier; needs GTK 3 and WebKitGTK 4.1) | `Pass_Quest-{{VERSION}}-x86_64.AppImage` |

`SHA256SUMS.txt` lists each file's SHA-256.

**The installers are unsigned.**
- On Windows, SmartScreen may show "Windows protected your PC"; choose *More info → Run anyway*.
- On macOS 26 and 27, open the app once and macOS refuses it; then choose **Open Anyway** under *System Settings → Privacy & Security* and confirm with your login password. With Homebrew, that step never comes:

  ```bash
  brew install --cask wxxb789/tap/pass-quest
  ```

The [install guide](https://github.com/wxxb789/passquest/blob/{{TAG}}/docs/install.md) ([简体中文](https://github.com/wxxb789/passquest/blob/{{TAG}}/docs/install.zh-CN.md)) has the steps for each platform, where your data lives, how to upgrade and how to remove it. An upgrade keeps your data. The app refuses to open data written by a newer version, and leaves that data untouched.

CI built and checked every file in [run {{RUN}}](https://github.com/wxxb789/passquest/actions/runs/{{RUN}}), from commit `{{SHA}}`. On its own runner, each package was installed and launched with no developer tools present, and the run completed a study loop, a relaunch, a reinstall and a forward upgrade. The package contents were scanned too.
