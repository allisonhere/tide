package ui

import (
	"github.com/allisonhere/tideui"
)

// Theme is an alias for tideui.Theme. Tide shares its theme collection with the
// other Tide apps through tideui; aliasing the type (rather than redeclaring an
// identical struct) keeps them from silently drifting and lets Tide hand its
// themes straight to any tideui API. The field set is the twelve core colors,
// the Family and Variant labels, and the optional Success, Warning, Link,
// Quote, Match, Tag and Cursor roles.
type Theme = tideui.Theme

// The palettes come from tideui, which owns the collection (families, variants,
// upstream sources and licenses; see its docs/themes.md). These are the themes
// Tide shipped before the collection was shared, kept under their old names.
var (
	CatppuccinMocha       = tideui.CatppuccinMocha
	CatppuccinLatte       = tideui.CatppuccinLatte
	CatppuccinFrappe      = tideui.CatppuccinFrappe
	CatppuccinMacchiato   = tideui.CatppuccinMacchiato
	Nord                  = tideui.Nord
	Dracula               = tideui.Dracula
	GruvboxDark           = tideui.GruvboxDark
	GruvboxLight          = tideui.GruvboxLight
	TokyoNight            = tideui.TokyoNight
	TokyoNightDay         = tideui.TokyoNightDay
	RosePine              = tideui.RosePine
	RosePineMoon          = tideui.RosePineMoon
	RosePineDawn          = tideui.RosePineDawn
	OneDark               = tideui.OneDark
	MagentaGeode          = tideui.MagentaGeode
	CoralSunset           = tideui.CoralSunset
	LavenderFieldsForever = tideui.LavenderFieldsForever
	VT100                 = tideui.VT100
	VT52                  = tideui.VT52
)

// BuiltinThemes is tideui's collection, grouped by family.
var BuiltinThemes = tideui.BuiltinThemes

func ThemeByName(name string) (Theme, int) {
	if isMatchOmarchy(name) {
		return omarchyPlaceholderTheme, omarchyIndex
	}
	for i, t := range BuiltinThemes {
		if t.Name == name {
			return t, i
		}
	}
	return BuiltinThemes[0], 0
}

// PickableThemes is BuiltinThemes plus the runtime-resolved "match-omarchy"
// pseudo-theme. The theme pickers (Settings screen and the T overlay) iterate
// this; contrast tests keep iterating BuiltinThemes only.
func PickableThemes() []Theme {
	out := make([]Theme, 0, len(BuiltinThemes)+1)
	out = append(out, BuiltinThemes...)
	out = append(out, omarchyPlaceholderTheme)
	return out
}

// pickableThemeNameAt returns the theme name at idx into PickableThemes(),
// clamping an out-of-range index to the first theme.
func pickableThemeNameAt(idx int) string {
	pt := PickableThemes()
	if idx < 0 || idx >= len(pt) {
		idx = 0
	}
	return pt[idx].Name
}

// pickableThemeLabelAt is the display label of the theme at idx into
// PickableThemes(), clamped the same way as pickableThemeNameAt. Config keeps
// the Name; the label is only for showing to people.
func pickableThemeLabelAt(idx int) string {
	pt := PickableThemes()
	if idx < 0 || idx >= len(pt) {
		idx = 0
	}
	return pt[idx].Label()
}
