package desktop

import "fmt"

// missingWebViewMessage is the one message a launch shows when the system
// webview is absent: the runtime a package does not bundle was not found, so
// it names what to install and where to get it, per platform. The message is
// shown natively (TaskDialog, GTK, NSAlert) and the launch exits non-zero
// after it.
func missingWebViewMessage(goos string) (title, body string) {
	title = AppName + " cannot start"
	switch goos {
	case "windows":
		body = fmt.Sprintf("%s needs the Microsoft Edge WebView2 Runtime, which this PC does not have. Install it from https://developer.microsoft.com/microsoft-edge/webview2/ (the Evergreen Standalone Installer), then start %s again.", AppName, AppName)
	case "darwin":
		body = fmt.Sprintf("%s needs the macOS system web view (WKWebView), which this Mac does not provide. Update macOS to a supported version, then start %s again.", AppName, AppName)
	default:
		body = fmt.Sprintf("%s needs GTK 3 and WebKitGTK 4.1, which this system does not have. Install libgtk-3-0 and libwebkit2gtk-4.1-0 on Debian/Ubuntu (gtk3 and webkit2gtk4.1 on Fedora), then start %s again.", AppName, AppName)
	}
	return title, body
}
