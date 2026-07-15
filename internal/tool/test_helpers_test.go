package tool

import "strconv"

func quoteJSON(value string) string {
	return strconv.Quote(value)
}
