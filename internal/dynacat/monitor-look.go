package dynacat

import (
	"strings"
	"unicode"
)

// monogram is the letter shown in place of a monitor site's icon when it has
// none or the icon fails to load, so every row keeps the same shape.
func monogram(title string) string {
	for _, r := range title {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return strings.ToUpper(string(r))
		}
	}
	return "•"
}
