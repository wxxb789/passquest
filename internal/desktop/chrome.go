package desktop

import (
	"errors"
	"fmt"

	"github.com/egoist/mygo"
)

// SetWindowButtonsTheme draws the caption buttons of the hidden title bar for
// the page's own theme (goal 0022, Q-0022-02): Windows would draw them in the
// system's appearance, which may differ from the page's, and GTK's title
// buttons follow the desktop's GTK theme, which may differ from it just as
// well. macOS's traffic lights keep the system's own look.
func (s *Study) SetWindowButtonsTheme(theme string) error {
	want, err := buttonsTheme(theme)
	if err != nil {
		return err
	}
	win := s.window()
	if win == nil {
		return errors.New("the window does not exist yet")
	}
	win.SetWindowButtonsTheme(want)
	return mygo.Theme.SetGTKCSS(gtkTitleButtonsCSS(theme))
}

// buttonsTheme maps the page's theme name to MyGo's ThemeSource.
func buttonsTheme(theme string) (mygo.ThemeSource, error) {
	switch theme {
	case "light":
		return mygo.ThemeLight, nil
	case "dark":
		return mygo.ThemeDark, nil
	}
	return "", fmt.Errorf("unknown theme %q", theme)
}

// gtkTitleButtonsCSS draws Linux's GTK title buttons from the page's own ink: MyGo's
// Window.SetWindowButtonsTheme is a no-op there (internal/linux/window.go) and GTK's buttons
// otherwise follow the desktop's theme, which need not match the page's appearance (Q-0022-02);
// Theme.SetGTKCSS is the API for them. The hex values are desktop/web/styles/tokens.css --ink for
// [data-theme="light"] and [data-theme="dark"], in sRGB — GTK 3's CSS has no oklch().
func gtkTitleButtonsCSS(theme string) string {
	ink := "#151c28"
	if theme == "dark" {
		ink = "#ebeff4"
	}
	return `headerbar.mygo-window-controls button.titlebutton {
  color: ` + ink + `;
  -gtk-icon-effect: none;
  -gtk-icon-shadow: none;
  background-image: none;
  background-color: transparent;
  border-color: transparent;
  box-shadow: none;
}
headerbar.mygo-window-controls button.titlebutton:hover { background-color: alpha(currentColor, .10); }
headerbar.mygo-window-controls button.titlebutton:active { background-color: alpha(currentColor, .16); }
`
}
