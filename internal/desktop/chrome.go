package desktop

import (
	"errors"
	"fmt"

	"github.com/egoist/mygo"
)

// SetWindowButtonsTheme draws the caption buttons of the hidden title bar for
// the page's own theme (goal 0022, Q-0022-02): Windows would draw them in the
// system's appearance, which may differ from the page's, while macOS and Linux
// keep the system's own buttons whatever this says.
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
	return nil
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
